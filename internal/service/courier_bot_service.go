package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"github.com/louissoe/niaga-autoparts/internal/utils"
	"go.uber.org/zap"
)

func getRoutables(points []model.DeliveryPoint) []utils.DeliveryRoutable {
	res := make([]utils.DeliveryRoutable, len(points))
	for i, pt := range points {
		res[i] = pt
	}
	return res
}

func getSeqIndices(n int) []int {
	res := make([]int, n)
	for i := range res {
		res[i] = i
	}
	return res
}

// CourierBotService manages interactions for the Courier Assistant Telegram Bot.
type CourierBotService struct {
	bot          *tgbotapi.BotAPI
	logger       *zap.Logger
	webAppBase   string
	deliverySvc  *DeliveryService
	orderSvc     *OrderService
	userRepo     *repository.UserRepository
	courierChats map[int64]bool
}

// NewCourierBotService initializes a new courier Telegram bot instance.
func NewCourierBotService(token string, webAppBase string, logger *zap.Logger) (*CourierBotService, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("courier bot init failed: %w", err)
	}

	logger.Info("courier telegram bot authenticated", zap.String("username", "@"+bot.Self.UserName))

	return &CourierBotService{
		bot:          bot,
		logger:       logger,
		webAppBase:   strings.TrimRight(webAppBase, "/"),
		courierChats: make(map[int64]bool),
	}, nil
}

func (s *CourierBotService) SetUserRepository(repo *repository.UserRepository) {
	s.userRepo = repo
}

func (s *CourierBotService) SetOrderService(orderSvc *OrderService) {
	s.orderSvc = orderSvc
}

// StartPolling starts polling for updates from the courier bot (in a background goroutine).
func (s *CourierBotService) StartPolling(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := s.bot.GetUpdatesChan(u)

	go func() {
		s.logger.Info("courier bot polling started")
		for {
			select {
			case <-ctx.Done():
				s.logger.Info("stopping courier bot polling")
				s.bot.StopReceivingUpdates()
				return
			case update, ok := <-updates:
				if !ok {
					return
				}
				s.handleUpdate(update)
			}
		}
	}()
}

// handleUpdate handles incoming commands, callback queries, and messages.
func (s *CourierBotService) handleUpdate(update tgbotapi.Update) {
	if update.CallbackQuery != nil {
		if update.CallbackQuery.Message != nil {
			s.courierChats[update.CallbackQuery.Message.Chat.ID] = true
		}
		s.handleCallbackQuery(update.CallbackQuery)
		return
	}

	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	s.courierChats[chatID] = true
	text := strings.TrimSpace(update.Message.Text)

	switch {
	case strings.HasPrefix(text, "/start") || strings.HasPrefix(text, "/tugas") || strings.HasPrefix(text, "/menu"):
		s.sendManifestSummary(chatID)
	case strings.HasPrefix(text, "/pins"):
		s.sendLocationPins(chatID)
	case strings.HasPrefix(text, "/rute"):
		s.sendRouteMap(chatID)
	case strings.HasPrefix(text, "/help"):
		s.sendHelp(chatID)
	default:
		s.sendManifestSummary(chatID)
	}
}

// SetDeliveryService injects DeliveryService to handle approval/reschedule callbacks.
func (s *CourierBotService) SetDeliveryService(svc *DeliveryService) {
	s.deliverySvc = svc
}

// getTargetCourierChatIDs retrieves all chat IDs from database (role = courier) and recent active chat IDs.
func (s *CourierBotService) getTargetCourierChatIDs(ctx context.Context) []int64 {
	targetSet := make(map[int64]bool)

	// 1. From recent memory cache
	for chatID := range s.courierChats {
		targetSet[chatID] = true
	}

	// 2. From database (users with role = 'courier')
	if s.userRepo != nil {
		users, _, err := s.userRepo.FindFiltered(ctx, repository.UserFilter{
			Role: "courier",
		})
		if err == nil {
			for _, u := range users {
				if u.TelegramChatID.Valid && u.TelegramChatID.String != "" {
					if cID, errParse := strconv.ParseInt(u.TelegramChatID.String, 10, 64); errParse == nil {
						targetSet[cID] = true
					}
				}
			}
		}
	}

	res := make([]int64, 0, len(targetSet))
	for chatID := range targetSet {
		res = append(res, chatID)
	}
	return res
}

// NotifyNewDeliveryRequest sends a notification to courier(s) when a customer requests delivery.
func (s *CourierBotService) NotifyNewDeliveryRequest(ctx context.Context, d *model.Delivery) {
	if s.bot == nil {
		return
	}

	text := fmt.Sprintf(
		"🚚 *PERMINTAAN PENGANTARAN BARU!*\n\n"+
			"📦 *No. Order:* `%s`\n"+
			"👤 *Penerima:* %s\n"+
			"📞 *Telepon:* `%s`\n"+
			"🏢 *Alamat:* %s\n"+
			"📅 *Tanggal Antar:* %s\n"+
			"⏰ *Slot Waktu:* %s\n"+
			"📏 *Jarak Estimasi:* `%.1f km`\n"+
			"💰 *Ongkir:* `Rp %.0f`\n\n"+
			"Apakah jadwal ini dapat diterima?",
		d.OrderNumber, d.CustomerName, d.CustomerPhone, d.CustomerAddress,
		d.DeliveryDate.Format("02 Jan 2006"), d.SlotName, d.DistanceKm, d.ShippingCost,
	)

	// Action buttons
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Terima Jadwal", fmt.Sprintf("approve_delivery_%d", d.ID)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Sarankan Jadwal Lain", fmt.Sprintf("reschedule_delivery_%d", d.ID)),
		),
	)

	// Broadcast to all courier chat IDs
	targetChats := s.getTargetCourierChatIDs(ctx)
	if len(targetChats) == 0 {
		s.logger.Warn("no registered courier chat IDs found to send delivery alert", zap.Int64("delivery_id", d.ID))
		return
	}

	for _, chatID := range targetChats {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = tgbotapi.ModeMarkdown
		msg.ReplyMarkup = markup
		if _, err := s.bot.Send(msg); err != nil {
			s.logger.Warn("failed to send delivery notification to courier chat", zap.Int64("chat_id", chatID), zap.Error(err))
		}
	}

	s.logger.Info("delivery notification sent to active couriers", zap.Int64("delivery_id", d.ID), zap.Int("recipients", len(targetChats)))
}

// CalculateNextRunTime calculates the next execution time for a target hour and minute in a given timezone location.
func CalculateNextRunTime(now time.Time, targetHour, targetMinute int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	nowInLoc := now.In(loc)
	nextRun := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), targetHour, targetMinute, 0, 0, loc)
	if nowInLoc.After(nextRun) || nowInLoc.Equal(nextRun) {
		nextRun = nextRun.Add(24 * time.Hour)
	}
	return nextRun
}

// FormatDailyMorningDigest formats the delivery digest text for couriers.
func FormatDailyMorningDigest(today time.Time, deliveries []model.Delivery) string {
	if len(deliveries) == 0 {
		return fmt.Sprintf("🌅 *Selamat Pagi Tim Kurir!* (05:00 WIB)\n📅 *Tanggal:* %s\n\n✅ _Tidak ada jadwal pengantaran barang untuk hari ini._ Tetap semangat!", today.Format("02 Jan 2006"))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🌅 *PENGINGAT PENGANTARAN HARI INI (05:00 WIB)*\n"))
	sb.WriteString(fmt.Sprintf("📅 *Tanggal:* %s\n", today.Format("02 Jan 2006")))
	sb.WriteString(fmt.Sprintf("📦 *Total Paket Siap Antar:* %d Pengiriman\n", len(deliveries)))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n\n")

	for i, d := range deliveries {
		sb.WriteString(fmt.Sprintf("%d️⃣ *Slot: %s*\n", i+1, d.SlotName))
		sb.WriteString(fmt.Sprintf("📦 No: `%s`\n", d.OrderNumber))
		sb.WriteString(fmt.Sprintf("👤 Penerima: %s (Telp: `%s`)\n", d.CustomerName, d.CustomerPhone))
		sb.WriteString(fmt.Sprintf("🏢 Alamat: %s\n", d.CustomerAddress))
		if d.DistanceKm > 0 {
			sb.WriteString(fmt.Sprintf("📏 Jarak: `%.1f km`\n", d.DistanceKm))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("🗺️ _Gunakan perintah /rute atau /pins untuk navigasi peta teroptimasi._\nSemoga pengiriman hari ini lancar & aman! 🛵💨")
	return sb.String()
}

// SendDailyMorningDigest broadcasts today's delivery schedule to all couriers at 05:00 AM.
func (s *CourierBotService) SendDailyMorningDigest(ctx context.Context) {
	if s.bot == nil || s.deliverySvc == nil {
		return
	}

	today := time.Now()
	deliveries, err := s.deliverySvc.GetDeliveriesForDate(ctx, today, "confirmed")
	if err != nil {
		s.logger.Error("failed to get today's deliveries for morning digest", zap.Error(err))
		return
	}

	text := FormatDailyMorningDigest(today, deliveries)

	targetChats := s.getTargetCourierChatIDs(ctx)
	if len(targetChats) == 0 {
		s.logger.Warn("no registered courier chat IDs found to send morning digest")
		return
	}

	for _, chatID := range targetChats {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = tgbotapi.ModeMarkdown
		_, _ = s.bot.Send(msg)
	}

	s.logger.Info("daily morning delivery digest sent", zap.Int("total_deliveries", len(deliveries)), zap.Int("recipients", len(targetChats)))
}

// handleCallbackQuery processes button clicks from inline keyboards.
func (s *CourierBotService) handleCallbackQuery(cb *tgbotapi.CallbackQuery) {
	// Acknowledge callback immediately
	callbackCfg := tgbotapi.NewCallback(cb.ID, "")
	_, _ = s.bot.Request(callbackCfg)

	if cb.Message == nil {
		return
	}

	chatID := cb.Message.Chat.ID
	data := cb.Data

	switch {
	case data == "action_pins":
		s.sendLocationPins(chatID)
	case data == "action_summary":
		s.sendManifestSummary(chatID)
	case data == "action_rute":
		s.sendRouteMap(chatID)
	case strings.HasPrefix(data, "stop_"):
		// e.g. "stop_1"
		seqStr := strings.TrimPrefix(data, "stop_")
		if seq, err := strconv.Atoi(seqStr); err == nil {
			s.sendSinglePointDetail(chatID, seq)
		}
	case strings.HasPrefix(data, "approve_delivery_"):
		deliveryIDStr := strings.TrimPrefix(data, "approve_delivery_")
		if dID, err := strconv.ParseInt(deliveryIDStr, 10, 64); err == nil && s.deliverySvc != nil {
			_ = s.deliverySvc.CourierApprove(context.Background(), dID, 0)
			editMsg := tgbotapi.NewEditMessageText(chatID, cb.Message.MessageID, fmt.Sprintf("✅ *Pengantaran #%d Telah Anda Terima & Dikonfirmasi!*", dID))
			editMsg.ParseMode = tgbotapi.ModeMarkdown
			_, _ = s.bot.Send(editMsg)
		}
	case strings.HasPrefix(data, "reschedule_delivery_"):
		deliveryIDStr := strings.TrimPrefix(data, "reschedule_delivery_")
		if dID, err := strconv.ParseInt(deliveryIDStr, 10, 64); err == nil && s.deliverySvc != nil {
			// Propose next day schedule as default suggestion
			nextDay := time.Now().Add(24 * time.Hour)
			_ = s.deliverySvc.CourierSuggestReschedule(context.Background(), dID, nextDay, 2, "Kurir sedang dalam rute lain / kapasitas penuh")
			editMsg := tgbotapi.NewEditMessageText(chatID, cb.Message.MessageID, fmt.Sprintf("⚠️ *Saran Perubahan Jadwal untuk Pengantaran #%d telah dikirim ke Customer.*", dID))
			editMsg.ParseMode = tgbotapi.ModeMarkdown
			_, _ = s.bot.Send(editMsg)
		}
	}
}

// getTodayRealDeliveryTask fetches real confirmed deliveries for today and constructs a DeliveryTask.
func (s *CourierBotService) getTodayRealDeliveryTask(ctx context.Context) model.DeliveryTask {
	today := time.Now()
	task := model.DeliveryTask{
		ManifestID:  fmt.Sprintf("MNF-%s", today.Format("20060102")),
		CourierName: "Tim Kurir Niaga",
		Date:        today.Format("02 January 2006"),
		OriginName:  "Gudang Pusat Niaga Autoparts",
		OriginLat:   WarehouseLat,
		OriginLng:   WarehouseLng,
		Points:      []model.DeliveryPoint{},
	}

	if s.deliverySvc == nil {
		return task
	}

	deliveries, err := s.deliverySvc.GetDeliveriesForDate(ctx, today, "")
	if err != nil || len(deliveries) == 0 {
		return task
	}

	var validPoints []model.DeliveryPoint
	for _, d := range deliveries {
		// Filter yang statusnya confirmed atau on_delivery
		if d.Status != model.DeliveryStatusConfirmed && d.Status != model.DeliveryStatusOnDelivery {
			continue
		}

		lat := d.CustomerLatitude
		lng := d.CustomerLongitude
		if lat == 0 || lng == 0 {
			lat = WarehouseLat
			lng = WarehouseLng
		}

		itemsSummary := "1x Pesanan Sparepart"
		if s.orderSvc != nil && d.OrderID > 0 {
			if ord, errO := s.orderSvc.GetOrderByID(ctx, d.OrderID); errO == nil && ord != nil && len(ord.Items) > 0 {
				var names []string
				for _, it := range ord.Items {
					names = append(names, fmt.Sprintf("%dx %s", it.Quantity, it.ProductName))
				}
				itemsSummary = strings.Join(names, ", ")
			}
		}

		validPoints = append(validPoints, model.DeliveryPoint{
			OrderNumber:  d.OrderNumber,
			CustomerName: d.CustomerName,
			Phone:        d.CustomerPhone,
			Address:      d.CustomerAddress,
			Latitude:     lat,
			Longitude:    lng,
			Notes:        d.Notes,
			ItemsSummary: itemsSummary,
		})
	}

	task.Points = validPoints
	task.OptimizeRoute()
	return task
}

// sendManifestSummary sends the summary of today's delivery manifest with action buttons.
func (s *CourierBotService) sendManifestSummary(chatID int64) {
	task := s.getTodayRealDeliveryTask(context.Background())

	if len(task.Points) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📦 *Tidak ada tugas pengiriman aktif untuk hari ini.*\n\nSemua pesanan sudah selesai diantar atau belum ada pesanan terkonfirmasi untuk hari ini.")
		msg.ParseMode = tgbotapi.ModeMarkdown
		_, _ = s.bot.Send(msg)
		return
	}

	totalKm := utils.CalculateTotalDistance(task.OriginLat, task.OriginLng, getRoutables(task.Points), getSeqIndices(len(task.Points)))

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🚚 *DAFTAR TUGAS PENGIRIMAN REAL-TIME*\n"))
	sb.WriteString(fmt.Sprintf("📋 *Manifest:* `%s`\n", task.ManifestID))
	sb.WriteString(fmt.Sprintf("📅 *Tanggal:* %s\n", task.Date))
	sb.WriteString(fmt.Sprintf("🏢 *Asal (Gudang):* %s\n", task.OriginName))
	sb.WriteString(fmt.Sprintf("📍 *Total Titik Antar:* %d Lokasi\n", len(task.Points)))
	sb.WriteString(fmt.Sprintf("⚡ *Estimasi Total Jarak:* `%.2f km` (Rute Teroptimasi)\n", totalKm))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n\n")

	prevLat, prevLng := task.OriginLat, task.OriginLng
	for _, pt := range task.Points {
		distFromPrev := utils.CalculateHaversineDistanceKm(prevLat, prevLng, pt.Latitude, pt.Longitude)
		sb.WriteString(fmt.Sprintf("🛑 *Stop #%d: %s* (`+%.1f km`)\n", pt.Sequence, pt.CustomerName, distFromPrev))
		sb.WriteString(fmt.Sprintf("📦 No: `%s`\n", pt.OrderNumber))
		sb.WriteString(fmt.Sprintf("🏢 Alamat: %s\n", pt.Address))
		sb.WriteString(fmt.Sprintf("📞 Telp: `%s`\n", pt.Phone))
		sb.WriteString(fmt.Sprintf("🛍️ Barang: %s\n", pt.ItemsSummary))
		if pt.Notes != "" {
			sb.WriteString(fmt.Sprintf("📝 _Catatan: %s_\n", pt.Notes))
		}
		sb.WriteString("\n")
		prevLat, prevLng = pt.Latitude, pt.Longitude
	}

	routeURL := task.GenerateGoogleMapsRouteURL()
	webAppURL := s.webAppBase + "/api/v1/courier/map-view"

	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	// Row 1: Send Pin Points & Open Google Maps Multi-Route
	keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("📍 Kirim Pin Lokasi Antar", "action_pins"),
	))

	if routeURL != "" {
		keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🗺️ Buka Rute Multi-Stop (Google Maps)", routeURL),
		))
	}

	if s.webAppBase != "" && !strings.Contains(s.webAppBase, "localhost") && !strings.Contains(s.webAppBase, "127.0.0.1") {
		keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("📱 Buka Peta Interaktif (Leaflet Web)", webAppURL),
		))
	}

	// Dynamic buttons for each stop point
	var stopButtons []tgbotapi.InlineKeyboardButton
	for _, pt := range task.Points {
		stopButtons = append(stopButtons, tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("Stop #%d", pt.Sequence),
			fmt.Sprintf("stop_%d", pt.Sequence),
		))
	}
	if len(stopButtons) > 0 {
		keyboardRows = append(keyboardRows, stopButtons)
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)

	_, err := s.bot.Send(msg)
	if err != nil {
		s.logger.Error("failed to send manifest summary", zap.Error(err), zap.Int64("chat_id", chatID))
	}
}

// sendLocationPins sends Telegram native venue/location pins for each stop.
func (s *CourierBotService) sendLocationPins(chatID int64) {
	task := s.getTodayRealDeliveryTask(context.Background())

	if len(task.Points) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📦 Tidak ada titik pengantaran aktif untuk dikirimkan pin lokasinya.")
		_, _ = s.bot.Send(msg)
		return
	}

	introMsg := tgbotapi.NewMessage(chatID, "📌 *Mengirimkan Pin Lokasi Titik Pengantaran...*")
	introMsg.ParseMode = tgbotapi.ModeMarkdown
	_, _ = s.bot.Send(introMsg)

	// Send Origin (Gudang) Pin first
	originVenue := tgbotapi.NewVenue(
		chatID,
		fmt.Sprintf("🏢 [ASAL] %s", task.OriginName),
		"Titik Muat / Gudang Pusat",
		task.OriginLat,
		task.OriginLng,
	)
	_, _ = s.bot.Send(originVenue)

	// Send Delivery Points pins
	for _, pt := range task.Points {
		title := fmt.Sprintf("🛑 Stop #%d: %s", pt.Sequence, pt.CustomerName)
		address := fmt.Sprintf("%s (%s)", pt.Address, pt.OrderNumber)

		venue := tgbotapi.NewVenue(chatID, title, address, pt.Latitude, pt.Longitude)

		// Create navigation button specifically for this pin
		singleRouteURL := fmt.Sprintf("https://www.google.com/maps/dir/?api=1&destination=%f,%f&travelmode=driving", pt.Latitude, pt.Longitude)
		venue.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(fmt.Sprintf("🧭 Navigasi ke Stop #%d", pt.Sequence), singleRouteURL),
			),
		)

		_, err := s.bot.Send(venue)
		if err != nil {
			s.logger.Error("failed to send venue pin", zap.Error(err), zap.Int("sequence", pt.Sequence))
		}
	}

	// Final message with multi-stop summary link
	routeURL := task.GenerateGoogleMapsRouteURL()
	doneMsg := tgbotapi.NewMessage(chatID, "✅ *Semua pin lokasi telah dikirim!*\nSilakan pilih pin lokasi tujuan di atas atau buka rute multi-stop lengkap berikut.")
	doneMsg.ParseMode = tgbotapi.ModeMarkdown

	var rows [][]tgbotapi.InlineKeyboardButton
	if routeURL != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🗺️ Navigasi Semua Rute (Google Maps)", routeURL),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("📋 Menu Tugas", "action_summary"),
	))

	doneMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, _ = s.bot.Send(doneMsg)
}

// sendSinglePointDetail sends detailed info for a single stop point.
func (s *CourierBotService) sendSinglePointDetail(chatID int64, sequence int) {
	task := s.getTodayRealDeliveryTask(context.Background())

	var selected *model.DeliveryPoint
	for _, pt := range task.Points {
		if pt.Sequence == sequence {
			selected = &pt
			break
		}
	}

	if selected == nil {
		msg := tgbotapi.NewMessage(chatID, "❌ Titik pengantaran tidak ditemukan.")
		_, _ = s.bot.Send(msg)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🛑 *DETAIL PENGIRIMAN - STOP #%d*\n\n", selected.Sequence))
	sb.WriteString(fmt.Sprintf("👤 *Penerima:* %s\n", selected.CustomerName))
	sb.WriteString(fmt.Sprintf("📦 *No. Order:* `%s`\n", selected.OrderNumber))
	sb.WriteString(fmt.Sprintf("📞 *Telepon:* `%s`\n", selected.Phone))
	sb.WriteString(fmt.Sprintf("🏢 *Alamat:* %s\n", selected.Address))
	sb.WriteString(fmt.Sprintf("🛍️ *Barang:* %s\n", selected.ItemsSummary))
	if selected.Notes != "" {
		sb.WriteString(fmt.Sprintf("📝 *Catatan:* _%s_\n", selected.Notes))
	}

	singleRouteURL := fmt.Sprintf("https://www.google.com/maps/dir/?api=1&destination=%f,%f&travelmode=driving", selected.Latitude, selected.Longitude)
	waPhone := strings.TrimPrefix(selected.Phone, "0")
	if !strings.HasPrefix(waPhone, "62") && len(waPhone) > 5 {
		waPhone = "62" + waPhone
	}
	waURL := fmt.Sprintf("https://wa.me/%s", waPhone)

	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🧭 Navigasi Google Maps", singleRouteURL),
			tgbotapi.NewInlineKeyboardButtonURL("💬 Chat WhatsApp", waURL),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Kembali ke Daftar", "action_summary"),
		),
	)

	venue := tgbotapi.NewVenue(
		chatID,
		fmt.Sprintf("Stop #%d: %s", selected.Sequence, selected.CustomerName),
		selected.Address,
		selected.Latitude,
		selected.Longitude,
	)
	venue.ReplyMarkup = markup

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = markup

	_, _ = s.bot.Send(msg)
	_, _ = s.bot.Send(venue)
}

// sendRouteMap sends direct links for Google Maps and Web Map.
func (s *CourierBotService) sendRouteMap(chatID int64) {
	task := s.getTodayRealDeliveryTask(context.Background())
	if len(task.Points) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📦 Tidak ada pengantaran aktif hari ini untuk membuat rute.")
		_, _ = s.bot.Send(msg)
		return
	}

	routeURL := task.GenerateGoogleMapsRouteURL()
	webAppURL := s.webAppBase + "/api/v1/courier/map-view"

	msg := tgbotapi.NewMessage(chatID, "🗺️ *Rute Navigasi Pengiriman Multi-Stop*\n\nPilih salah satu opsi di bawah untuk membuka peta dan rute navigasi:")
	msg.ParseMode = tgbotapi.ModeMarkdown

	var rows [][]tgbotapi.InlineKeyboardButton
	if routeURL != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🧭 Buka Google Maps (Rute Multi-Stop)", routeURL),
		))
	}
	if s.webAppBase != "" && !strings.Contains(s.webAppBase, "localhost") && !strings.Contains(s.webAppBase, "127.0.0.1") {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("📱 Buka Peta Interaktif Leaflet", webAppURL),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("📋 Daftar Tugas", "action_summary"),
	))

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, _ = s.bot.Send(msg)
}

// sendHelp displays available commands.
func (s *CourierBotService) sendHelp(chatID int64) {
	helpText := `🤖 *Bantuan Bot Kurir Niaga Autoparts*

Perintah yang tersedia:
• /tugas atau /start - Tampilkan daftar tugas pengiriman hari ini
• /pins - Kirimkan seluruh pin point lokasi tujuan ke chat
• /rute - Tampilkan link rute multi-stop Google Maps & Web Peta
• /help - Tampilkan panduan ini`

	msg := tgbotapi.NewMessage(chatID, helpText)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, _ = s.bot.Send(msg)
}
