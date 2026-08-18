package service

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"github.com/louissoe/niaga-autoparts/internal/utils"
	"go.uber.org/zap"
)

const (
	// Warehouse Coordinates (Niaga Autoparts Sunter)
	WarehouseLat = -7.990571
	WarehouseLng = 112.678819

	// Shipping Cost calculation constants
	BaseShippingRatePerKm = 750.0 // Rp 750 / km
	MinShippingCost       = 2000.0 // Minimum Rp 2.000
)

// RequestDeliveryInput represents input for requesting delivery for an order.
type RequestDeliveryInput struct {
	OrderID      int64    `json:"order_id"`
	CustomerID   int64    `json:"customer_id"`
	ScheduleID   int64    `json:"schedule_id"`
	DeliveryDate string   `json:"delivery_date"` // YYYY-MM-DD
	Address      string   `json:"address"`
	Latitude     *float64 `json:"latitude"`
	Longitude    *float64 `json:"longitude"`
	Notes        string   `json:"notes"`
}

type DeliveryService struct {
	deliveryRepo *repository.DeliveryRepository
	scheduleRepo *repository.DeliveryScheduleRepository
	customerRepo *repository.CustomerRepository
	orderRepo    *repository.OrderRepository
	sessionRepo  *repository.SessionRepository
	courierBot   *CourierBotService
	msgSender    model.MessageSender
	logger       *zap.Logger
	mapsSvc      *GoogleMapsService
}

func NewDeliveryService(
	deliveryRepo *repository.DeliveryRepository,
	scheduleRepo *repository.DeliveryScheduleRepository,
	customerRepo *repository.CustomerRepository,
	orderRepo *repository.OrderRepository,
	logger *zap.Logger,
) *DeliveryService {
	return &DeliveryService{
		deliveryRepo: deliveryRepo,
		scheduleRepo: scheduleRepo,
		customerRepo: customerRepo,
		orderRepo:    orderRepo,
		logger:       logger,
	}
}

func (s *DeliveryService) SetSessionRepository(repo *repository.SessionRepository) {
	s.sessionRepo = repo
}

// SetGoogleMapsService injects the GoogleMapsService for distance calculations.
func (s *DeliveryService) SetGoogleMapsService(svc *GoogleMapsService) {
	s.mapsSvc = svc
}

func (s *DeliveryService) SetCourierBot(courierBot *CourierBotService) {
	s.courierBot = courierBot
}

func (s *DeliveryService) SetMessageSender(sender model.MessageSender) {
	s.msgSender = sender
}

// GetAllSchedules returns all master delivery schedules with optional filters.
func (s *DeliveryService) GetAllSchedules(ctx context.Context, dayOfWeek string, isActive *bool) ([]model.DeliverySchedule, error) {
	return s.scheduleRepo.GetAll(ctx, dayOfWeek, isActive)
}

// GetScheduleByID returns a master delivery schedule by ID.
func (s *DeliveryService) GetScheduleByID(ctx context.Context, id int64) (*model.DeliverySchedule, error) {
	return s.scheduleRepo.GetByID(ctx, id)
}

// CreateSchedule validates and creates a new delivery schedule.
func (s *DeliveryService) CreateSchedule(ctx context.Context, schedule *model.DeliverySchedule) error {
	if schedule.DayOfWeek == "" {
		return fmt.Errorf("hari (day_of_week) wajib diisi")
	}
	if schedule.SlotName == "" {
		return fmt.Errorf("nama slot (slot_name) wajib diisi")
	}
	if schedule.StartTime == "" || schedule.EndTime == "" {
		return fmt.Errorf("jam mulai dan jam selesai wajib diisi")
	}
	if schedule.MaxCapacity <= 0 {
		schedule.MaxCapacity = 5 // default
	}
	return s.scheduleRepo.Create(ctx, schedule)
}

// UpdateSchedule validates and updates an existing delivery schedule.
func (s *DeliveryService) UpdateSchedule(ctx context.Context, schedule *model.DeliverySchedule) error {
	if schedule.ID <= 0 {
		return fmt.Errorf("ID jadwal tidak valid")
	}
	if schedule.DayOfWeek == "" {
		return fmt.Errorf("hari (day_of_week) wajib diisi")
	}
	if schedule.SlotName == "" {
		return fmt.Errorf("nama slot (slot_name) wajib diisi")
	}
	if schedule.StartTime == "" || schedule.EndTime == "" {
		return fmt.Errorf("jam mulai dan jam selesai wajib diisi")
	}
	if schedule.MaxCapacity <= 0 {
		schedule.MaxCapacity = 5
	}
	return s.scheduleRepo.Update(ctx, schedule)
}

// DeleteSchedule removes a delivery schedule by ID.
func (s *DeliveryService) DeleteSchedule(ctx context.Context, id int64) error {
    return s.scheduleRepo.Delete(ctx, id)
}


func (s *DeliveryService) GetDeliveriesForDate(ctx context.Context, date time.Time, status string) ([]model.Delivery, error) {
    return s.deliveryRepo.GetDeliveriesForDate(ctx, date, status)
}

// GetAvailableSchedules returns available delivery slots for the specified date.
func (s *DeliveryService) GetAvailableSchedules(ctx context.Context, targetDate time.Time) ([]model.DeliverySchedule, error) {
	return s.scheduleRepo.GetAvailableSchedulesByDate(ctx, targetDate)
}

// ShippingEstimateResult contains estimation calculations.
type ShippingEstimateResult struct {
	DistanceKm   float64 `json:"distance_km"`
	ShippingCost float64 `json:"shipping_cost"`
}

// CalculateShippingCost calculates distance and rounds cost up to nearest 1,000.
func (s *DeliveryService) CalculateShippingCost(lat, lng float64) ShippingEstimateResult {
	var distanceKm float64
	if s.mapsSvc != nil {
		if d, err := s.mapsSvc.GetDrivingDistance(context.Background(), WarehouseLat, WarehouseLng, lat, lng); err == nil {
			distanceKm = d
		} else {
			// Fallback to Haversine if Google Maps fails
			s.logger.Warn("Google Maps distance error, fallback to Haversine", zap.Error(err))
			distanceKm = utils.CalculateHaversineDistanceKm(WarehouseLat, WarehouseLng, lat, lng)
		}
	} else {
		distanceKm = utils.CalculateHaversineDistanceKm(WarehouseLat, WarehouseLng, lat, lng)
	}

	rawCost := distanceKm * BaseShippingRatePerKm

	// Bulatkan ke atas ke kelipatan 1.000 (contoh: 16.459 -> 17.000)
	roundedCost := math.Ceil(rawCost/1000.0) * 1000.0
	finalCost := math.Max(MinShippingCost, roundedCost)

	return ShippingEstimateResult{
		DistanceKm:   math.Round(distanceKm*100) / 100, // 2 desimal
		ShippingCost: finalCost,
	}
}

// RequestDelivery creates a delivery request from the customer checkout.
func (s *DeliveryService) RequestDelivery(ctx context.Context, input RequestDeliveryInput) (*model.Delivery, error) {
	targetDate, err := time.Parse("2006-01-02", input.DeliveryDate)
	if err != nil {
		return nil, fmt.Errorf("format tanggal tidak valid, gunakan YYYY-MM-DD: %w", err)
	}

	now := time.Now()
	todayZero := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if targetDate.Before(todayZero) {
		return nil, fmt.Errorf("tanggal pengantaran tidak boleh di masa lampau (backdate)")
	}

	// 1. Validasi slot ketersediaan jadwal
	schedules, err := s.scheduleRepo.GetAvailableSchedulesByDate(ctx, targetDate)
	if err != nil {
		return nil, fmt.Errorf("gagal mengecek jadwal: %w", err)
	}

	var selectedSchedule *model.DeliverySchedule
	for _, sch := range schedules {
		if sch.ID == input.ScheduleID {
			selectedSchedule = &sch
			break
		}
	}

	if selectedSchedule == nil {
		return nil, fmt.Errorf("jadwal pengantaran tidak ditemukan untuk hari yang dipilih")
	}

	if selectedSchedule.IsFull {
		return nil, fmt.Errorf("jadwal pengantaran slot '%s' sudah penuh untuk tanggal %s", selectedSchedule.SlotName, input.DeliveryDate)
	}

	// 2. Update/Patch koordinat, alamat, dan notes customer jika ada perubahan
	var custLat, custLng float64
	if input.Latitude != nil && input.Longitude != nil {
		custLat = *input.Latitude
		custLng = *input.Longitude
	}

	// Auto-resolve CustomerID from Order if not explicitly passed
	if input.CustomerID <= 0 && input.OrderID > 0 && s.orderRepo != nil && s.customerRepo != nil {
		if ord, errO := s.orderRepo.GetByID(ctx, input.OrderID); errO == nil && ord != nil && ord.UserID.Valid {
			if cust, errC := s.customerRepo.GetByUserID(ctx, ord.UserID.Int64); errC == nil && cust != nil {
				input.CustomerID = cust.ID
			}
		}
	}

	if input.CustomerID > 0 {
		cust, err := s.customerRepo.GetByID(ctx, input.CustomerID)
		if err == nil && cust != nil {
			updateNeeded := false

			// Patch Address jika dikirim dan berbeda
			cleanAddr := strings.TrimSpace(input.Address)
			if cleanAddr != "" && (!cust.Address.Valid || cust.Address.String != cleanAddr) {
				cust.Address = sql.NullString{String: cleanAddr, Valid: true}
				updateNeeded = true
			}

			// Patch Notes jika dikirim dan berbeda
			cleanNotes := strings.TrimSpace(input.Notes)
			if cleanNotes != "" && (!cust.Notes.Valid || cust.Notes.String != cleanNotes) {
				cust.Notes = sql.NullString{String: cleanNotes, Valid: true}
				updateNeeded = true
			}

			// Patch Latitude & Longitude jika dikirim dan berbeda
			if custLat != 0 && custLng != 0 {
				if !cust.Latitude.Valid || !cust.Longitude.Valid ||
					cust.Latitude.Float64 != custLat || cust.Longitude.Float64 != custLng {
					cust.Latitude = sql.NullFloat64{Float64: custLat, Valid: true}
					cust.Longitude = sql.NullFloat64{Float64: custLng, Valid: true}
					updateNeeded = true
				}
			} else if cust.Latitude.Valid && cust.Longitude.Valid {
				custLat = cust.Latitude.Float64
				custLng = cust.Longitude.Float64
			}

			// Hanya lakukan update database jika ada field yang benar-benar berubah
			if updateNeeded {
				if errUpd := s.customerRepo.Update(ctx, cust); errUpd != nil {
					s.logger.Warn("gagal update data customer saat delivery request", zap.Error(errUpd), zap.Int64("customer_id", cust.ID))
				}
			}
		}
	}

	// 3. Hitung estimasi jarak & ongkir
	var distanceKm float64
	var shippingCost float64

	if custLat != 0 && custLng != 0 {
		est := s.CalculateShippingCost(custLat, custLng)
		distanceKm = est.DistanceKm
		shippingCost = est.ShippingCost
	}

	delivery := &model.Delivery{
		OrderID:      input.OrderID,
		CustomerID:   sql.NullInt64{Int64: input.CustomerID, Valid: input.CustomerID > 0},
		ScheduleID:   input.ScheduleID,
		DeliveryDate: targetDate,
		Status:       model.DeliveryStatusWaitingCourier,
		ShippingCost: shippingCost,
		DistanceKm:   distanceKm,
		Notes:        input.Notes,
	}

	if err := s.deliveryRepo.Create(ctx, delivery); err != nil {
		return nil, fmt.Errorf("gagal membuat data pengantaran: %w", err)
	}

	// Fetch full data for notifications
	fullDelivery, err := s.deliveryRepo.GetByID(ctx, delivery.ID)
	if err == nil && fullDelivery != nil {
		// Notifikasi ke Kurir via Bot Kurir
		if s.courierBot != nil {
			s.courierBot.NotifyNewDeliveryRequest(ctx, fullDelivery)
		}
	}

	return delivery, nil
}

// CourierApprove accepts the customer delivery schedule.
func (s *DeliveryService) CourierApprove(ctx context.Context, deliveryID int64, courierID int64) error {
	var cID *int64
	if courierID > 0 {
		cID = &courierID
	}

	if err := s.deliveryRepo.UpdateStatus(ctx, deliveryID, model.DeliveryStatusConfirmed, cID); err != nil {
		return fmt.Errorf("gagal mengonfirmasi pengantaran: %w", err)
	}

	d, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err == nil && d != nil && d.TelegramChatID != "" && s.msgSender != nil {
		msg := fmt.Sprintf(
			"✅ *Jadwal Pengantaran Dikonfirmasi!*\n\n"+
				"Pesanan: *%s*\n"+
				"Tanggal: *%s*\n"+
				"Waktu: *%s*\n"+
				"Alamat: %s\n\n"+
				"Kurir kami siap mengantarkan pesanan Anda sesuai jadwal. Terima kasih! 🚚",
			d.OrderNumber, d.DeliveryDate.Format("02 Jan 2006"), d.SlotName, d.CustomerAddress,
		)
		_ = s.msgSender.SendText(ctx, model.PlatformTelegram, d.TelegramChatID, msg)
	}

	return nil
}

// CourierSuggestReschedule rejects the requested schedule and proposes an alternate date/slot.
func (s *DeliveryService) CourierSuggestReschedule(ctx context.Context, deliveryID int64, suggestedDate time.Time, suggestedScheduleID int64, reason string) error {
	if err := s.deliveryRepo.UpdateRescheduleSuggestion(ctx, deliveryID, suggestedDate, suggestedScheduleID, reason); err != nil {
		return fmt.Errorf("gagal mengajukan saran jadwal: %w", err)
	}

	d, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err == nil && d != nil && d.TelegramChatID != "" && s.msgSender != nil {
		sugSched, _ := s.scheduleRepo.GetByID(ctx, suggestedScheduleID)
		sugSlotName := ""
		if sugSched != nil {
			sugSlotName = fmt.Sprintf("%s (%s - %s)", sugSched.SlotName, sugSched.StartTime, sugSched.EndTime)
		}

		// Update customer session to StateAwaitingRescheduleDecision
		if s.sessionRepo != nil {
			if sess, errSess := s.sessionRepo.GetOrCreate(ctx, d.TelegramChatID); errSess == nil && sess != nil {
				sess.LoadContext()
				sess.State = model.StateAwaitingRescheduleDecision
				sess.PendingDeliveryID = &deliveryID
				sess.SaveContext()
				_ = s.sessionRepo.Save(ctx, sess)
			}
		}

		msg := fmt.Sprintf(
			"⚠️ *Pemberitahuan Perubahan Jadwal Pengantaran*\n\n"+
				"Pesanan: *%s*\n"+
				"📝 *Alasan Kurir:* %s\n\n"+
				"💡 *Saran Jadwal Baru dari Kurir:*\n"+
				"📅 Tanggal : *%s*\n"+
				"⏰ Slot Jam : *%s*\n\n"+
				"━━━━━━━━━━━━━━━━━━━━\n"+
				"👉 *Silakan pilih tindakan:*\n"+
				"1️⃣ Balas *1* atau *SETUJU* → Menerima jadwal saran dari kurir\n"+
				"2️⃣ Balas *2* atau *GANTI*  → Pilih tanggal / slot pengantaran lain\n"+
				"3️⃣ Balas *3* atau *TOLAK*  → Batalkan pengantaran (ambil sendiri di toko)\n"+
				"━━━━━━━━━━━━━━━━━━━━",
			d.OrderNumber, reason, suggestedDate.Format("02 Jan 2006"), sugSlotName,
		)

		_ = s.msgSender.SendText(ctx, model.PlatformTelegram, d.TelegramChatID, msg)
	}

	return nil
}

// CustomerAcceptReschedule accepts the courier's proposed reschedule.
func (s *DeliveryService) CustomerAcceptReschedule(ctx context.Context, deliveryID int64) error {
	d, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil || d == nil {
		return fmt.Errorf("data pengantaran tidak ditemukan")
	}

	if d.SuggestedDate == nil || !d.SuggestedScheduleID.Valid {
		return fmt.Errorf("tidak ada saran jadwal baru yang diajukan")
	}

	if err := s.deliveryRepo.AcceptRescheduledSchedule(ctx, deliveryID, *d.SuggestedDate, d.SuggestedScheduleID.Int64); err != nil {
		return fmt.Errorf("gagal menerima jadwal baru: %w", err)
	}

	if d.TelegramChatID != "" && s.msgSender != nil {
		sugSched, _ := s.scheduleRepo.GetByID(ctx, d.SuggestedScheduleID.Int64)
		sugSlotName := ""
		if sugSched != nil {
			sugSlotName = sugSched.SlotName
		}
		msg := fmt.Sprintf(
			"✅ *Jadwal Pengantaran Telah Dikonfirmasi!*\n\n"+
				"Pesanan: *%s*\n"+
				"📅 Tanggal : *%s*\n"+
				"⏰ Slot Jam : *%s*\n\n"+
				"Kurir kami akan mengantar pesanan Anda sesuai jadwal baru ini. Terima kasih! 🚚",
			d.OrderNumber, d.SuggestedDate.Format("02 Jan 2006"), sugSlotName,
		)
		_ = s.msgSender.SendText(ctx, model.PlatformTelegram, d.TelegramChatID, msg)
	}

	return nil
}

// CustomerRejectReschedule cancels the delivery request (e.g. customer chooses to pick up in store instead).
func (s *DeliveryService) CustomerRejectReschedule(ctx context.Context, deliveryID int64) error {
	d, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil || d == nil {
		return fmt.Errorf("data pengantaran tidak ditemukan")
	}

	if err := s.deliveryRepo.UpdateStatus(ctx, deliveryID, model.DeliveryStatusCancelled, nil); err != nil {
		return fmt.Errorf("gagal membatalkan pengantaran: %w", err)
	}

	if d.TelegramChatID != "" && s.msgSender != nil {
		msg := fmt.Sprintf(
			"❌ *Pengantaran Kurir Dibatalkan*\n\n"+
				"Pesanan: *%s*\n\n"+
				"Pengantaran kurir telah dibatalkan. Anda dapat mengambil pesanan secara langsung di toko Niaga AutoParts. Terima kasih! 🏬",
			d.OrderNumber,
		)
		_ = s.msgSender.SendText(ctx, model.PlatformTelegram, d.TelegramChatID, msg)
	}

	return nil
}

// CustomerChangeSchedule allows customer to choose their own new date & slot.
func (s *DeliveryService) CustomerChangeSchedule(ctx context.Context, deliveryID int64, newDate time.Time, newScheduleID int64) error {
	d, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil || d == nil {
		return fmt.Errorf("data pengantaran tidak ditemukan")
	}

	if err := s.deliveryRepo.ChangeSchedule(ctx, deliveryID, newDate, newScheduleID); err != nil {
		return fmt.Errorf("gagal mengubah jadwal pengantaran: %w", err)
	}

	// Notify courier of customer's updated schedule
	if s.courierBot != nil {
		if updatedDelivery, errG := s.deliveryRepo.GetByID(ctx, deliveryID); errG == nil && updatedDelivery != nil {
			s.courierBot.NotifyNewDeliveryRequest(ctx, updatedDelivery)
		}
	}

	return nil
}
