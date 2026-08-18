package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/ai"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"go.uber.org/zap"
)

// MessageProcessor is the brain of the chatbot.
// It is provider-agnostic: the concrete MessageSender (Fonnte or Telegram)
// is injected via the model.MessageSender interface, so the same logic works
// for both WhatsApp and Telegram without any modification here.
type MessageProcessor struct {
	intentSvc    *IntentService
	productSvc   *ProductService
	orderSvc     *OrderService
	deliverySvc  *DeliveryService
	customerRepo *repository.CustomerRepository
	userRepo     *repository.UserRepository
	sessionRepo  *repository.SessionRepository
	messagingSvc model.MessageSender // interface — not tied to any provider
	aiSvc        *ai.AIService
	midtransSvc  *MidtransService
	reportSvc    *ReportService
	logger       *zap.Logger
	sessionTTL   time.Duration
}

// NewMessageProcessor wires up all dependencies.
func NewMessageProcessor(
	intentSvc *IntentService,
	productSvc *ProductService,
	orderSvc *OrderService,
	sessionRepo *repository.SessionRepository,
	messagingSvc model.MessageSender,
	aiSvc *ai.AIService,
	sessionTTL time.Duration,
	logger *zap.Logger,
) *MessageProcessor {
	return &MessageProcessor{
		intentSvc:    intentSvc,
		productSvc:   productSvc,
		orderSvc:     orderSvc,
		sessionRepo:  sessionRepo,
		messagingSvc: messagingSvc,
		aiSvc:        aiSvc,
		logger:       logger,
		sessionTTL:   sessionTTL,
	}
}

func (p *MessageProcessor) SetDeliveryService(deliverySvc *DeliveryService) {
	p.deliverySvc = deliverySvc
}

func (p *MessageProcessor) SetCustomerRepository(customerRepo *repository.CustomerRepository) {
	p.customerRepo = customerRepo
}

func (p *MessageProcessor) SetUserRepository(userRepo *repository.UserRepository) {
	p.userRepo = userRepo
}

func (p *MessageProcessor) SetMidtransService(midtransSvc *MidtransService) {
	p.midtransSvc = midtransSvc
}

func (p *MessageProcessor) SetReportService(reportSvc *ReportService) {
	p.reportSvc = reportSvc
}

// Process is the main entry point for async message handling.
// It accepts the generic IncomingMessage — platform-agnostic.
func (p *MessageProcessor) Process(ctx context.Context, msg model.IncomingMessage) {
	sender := msg.Sender

	sess, err := p.sessionRepo.GetOrCreate(ctx, sender)
	if err != nil {
		p.logger.Error("session error", zap.String("sender", sender), zap.Error(err))
		_ = p.messagingSvc.SendText(ctx, msg.Platform, sender, FormatError("session_error"))
		return
	}

	if msg.IsFileMessage() {
		p.handleFile(ctx, msg, sess)
		return
	}

	if msg.IsImageMessage() {
		p.handleImage(ctx, msg, sess)
		return
	}

	if strings.HasPrefix(msg.Message, "/start") {
		reply := p.handleTelegramStart(ctx, msg, sess)
		if reply != "" {
			_ = p.messagingSvc.SendText(ctx, msg.Platform, sender, reply)
			return
		}
	}

	// Tangani alur state percakapan bertahap (Order Type & Delivery Flow)
	sess.LoadContext()
	if sess.State == model.StateAwaitingRescheduleDecision {
		reply := p.handleRescheduleDecision(ctx, msg, sess)
		p.finalizeAndSend(ctx, msg, sess, string(model.IntentOrder), reply)
		return
	} else if sess.State == model.StateAwaitingOrderType {
		reply := p.handleOrderTypeSelection(ctx, msg, sess)
		p.finalizeAndSend(ctx, msg, sess, string(model.IntentOrder), reply)
		return
	} else if sess.State == model.StateAwaitingDeliveryAddress {
		reply := p.handleDeliveryAddress(ctx, msg, sess)
		p.finalizeAndSend(ctx, msg, sess, string(model.IntentOrder), reply)
		return
	} else if sess.State == model.StateAwaitingDeliveryDate {
		reply := p.handleDeliveryDateSelection(ctx, msg, sess)
		p.finalizeAndSend(ctx, msg, sess, string(model.IntentOrder), reply)
		return
	} else if sess.State == model.StateAwaitingDeliverySchedule {
		reply := p.handleDeliveryScheduleSelection(ctx, msg, sess)
		p.finalizeAndSend(ctx, msg, sess, string(model.IntentOrder), reply)
		return
	}

	parsed, err := p.intentSvc.Detect(ctx, msg.Message, sess)
	if err != nil {
		p.logger.Error("intent detection failed", zap.Error(err))
		parsed = &model.ParsedMessage{
			OriginalText: msg.Message,
			Intent:       model.IntentUnknown,
		}
	}

	p.logger.Info("intent resolved",
		zap.String("sender", sender),
		zap.String("platform", string(msg.Platform)),
		zap.String("intent", string(parsed.Intent)),
		zap.Bool("from_ai", parsed.FromAI),
	)

	var reply string
	switch parsed.Intent {
	case model.IntentGreeting:
		reply = FormatWelcome(msg.SenderName)
		sess.State = model.StateIdle

	case model.IntentHelp:
		reply = FormatHelp()
		sess.State = model.StateIdle

	case model.IntentSearchProduct, model.IntentAskPrice:
		reply = p.handleSearch(ctx, parsed, sess)

	case model.IntentSelectProduct:
		reply = p.handleProductSelection(ctx, parsed, sess)

	case model.IntentOrder:
		reply = p.handleOrder(ctx, parsed, sess, msg)

	case model.IntentConfirmOrder:
		reply = p.handleConfirm(ctx, parsed, sess, sender)

	case model.IntentCancelOrder:
		reply = p.handleCancel(ctx, sess, sender)

	case model.IntentCheckOrder:
		reply = p.handleCheckOrders(ctx, sender)

	case model.IntentConfirmImport:
		reply = p.handleImportConfirm(ctx, sess, sender)

	case model.IntentHistory:
		reply = p.handleHistory(ctx, parsed, msg)
		if reply == "" {
			return
		}

	default:
		parsed.ProductQuery = msg.Message
		reply = p.handleSearch(ctx, parsed, sess)
	}

	p.finalizeAndSend(ctx, msg, sess, string(parsed.Intent), reply)
}

func (p *MessageProcessor) finalizeAndSend(ctx context.Context, msg model.IncomingMessage, sess *model.Session, lastIntent, reply string) {
	sess.SaveContext()
	sess.LastIntent = lastIntent
	sess.ExpiresAt = time.Now().Add(p.sessionTTL)
	if err := p.sessionRepo.Save(ctx, sess); err != nil {
		p.logger.Warn("failed to save session", zap.Error(err))
	}

	if reply != "" {
		if err := p.messagingSvc.SendText(ctx, msg.Platform, msg.Sender, reply); err != nil {
			p.logger.Error("failed to send reply", zap.String("sender", msg.Sender), zap.Error(err))
		}
	}
}

// ─── Intent Handlers ─────────────────────────────────────────────────────────

func (p *MessageProcessor) handleSearch(ctx context.Context, parsed *model.ParsedMessage, sess *model.Session) string {
	query := parsed.ProductQuery
	if query == "" {
		return "Produk apa yang Anda cari? Contoh: *kampas rem honda beat*"
	}

	products, err := p.productSvc.Search(ctx, query)
	if err != nil {
		p.logger.Error("product search error", zap.String("query", query), zap.Error(err))
		return FormatError("db_error")
	}
	if len(products) == 0 {
		return FormatError("not_found")
	}

	// Satu hasil → tampilkan detail langsung
	if len(products) == 1 {
		product, refs, err := p.productSvc.GetWithPriceRefs(ctx, products[0].ID)
		if err == nil {
			sess.LastProductID = &product.ID
			sess.LastProductName = product.Name
			sess.State = model.StateIdle
			sess.SearchResults = nil
			return FormatProductDetail(product, refs)
		}
	}

	// Banyak hasil → simpan ke session, minta user pilih nomor
	sess.State = model.StateAwaitingProductSelection
	sess.SearchResults = products
	sess.LastProductName = query
	return FormatProductList(products)
}

func (p *MessageProcessor) handleProductSelection(ctx context.Context, parsed *model.ParsedMessage, sess *model.Session) string {
	if sess.State != model.StateAwaitingProductSelection || len(sess.SearchResults) == 0 {
		return "Sesi pencarian sudah berakhir. Silakan cari produk lagi."
	}

	idx := parsed.Quantity - 1 // nomor 1 = index 0
	if idx < 0 || idx >= len(sess.SearchResults) {
		return fmt.Sprintf("Nomor tidak valid. Pilih antara 1 - %d.", len(sess.SearchResults))
	}

	product, refs, err := p.productSvc.GetWithPriceRefs(ctx, sess.SearchResults[idx].ID)
	if err != nil {
		p.logger.Error("get product detail failed", zap.Error(err))
		return FormatError("db_error")
	}

	sess.LastProductID = &product.ID
	sess.LastProductName = product.Name
	sess.State = model.StateIdle
	sess.SearchResults = nil

	return FormatProductDetail(product, refs)
}

func (p *MessageProcessor) handleOrder(ctx context.Context, parsed *model.ParsedMessage, sess *model.Session, msg model.IncomingMessage) string {
	sender := msg.Sender
	var productID int64
	if parsed.ProductQuery != "" {
		products, err := p.productSvc.Search(ctx, parsed.ProductQuery)
		if err != nil || len(products) == 0 {
			return FormatError("not_found")
		}
		if len(products) > 1 {
			sess.State = model.StateAwaitingProductSelection
			sess.SearchResults = products
			return "Ditemukan beberapa produk:\n\n" + FormatProductList(products) +
				"\n\nKetik nomor produk yang ingin dipesan."
		}
		productID = products[0].ID
	} else if sess.LastProductID != nil {
		productID = *sess.LastProductID
	} else {
		return "Produk apa yang ingin dipesan? Cari produk terlebih dahulu."
	}

	if parsed.Quantity == 0 {
		sess.State = model.StateAwaitingQty
		return fmt.Sprintf("Berapa jumlah *%s* yang ingin dipesan?", sess.LastProductName)
	}

	product, _, err := p.productSvc.GetWithPriceRefs(ctx, productID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FormatError("not_found")
		}
		return FormatError("db_error")
	}
	if product.AvailableStock() == 0 {
		return FormatError("out_of_stock")
	}

	order, err := p.orderSvc.CreateReservation(ctx, sender, msg.Platform, product, parsed.Quantity)
	if err != nil {
		p.logger.Error("create reservation failed", zap.Error(err))
		return FormatError("order_failed")
	}

	sess.PendingOrderID = &order.ID
	sess.State = model.StateAwaitingOrderType

	subtotal := product.SellingPrice * float64(parsed.Quantity)
	taxAmount := order.TotalPrice - subtotal

	return fmt.Sprintf(
		"🛒 *Pesanan Dibuat: %s*\n"+
			"━━━━━━━━━━━━━━━━\n"+
			"Produk      : %s (%d pcs)\n"+
			"Subtotal    : Rp %s\n"+
			"PPN (11%%)   : Rp %s\n"+
			"*Total      : Rp %s*\n"+
			"━━━━━━━━━━━━━━━━\n\n"+
			"Silakan pilih metode penerimaan pesanan:\n"+
			"1️⃣ Balas *1* atau *AMBIL* → Ambil di Toko (Pickup)\n"+
			"2️⃣ Balas *2* atau *KIRIM* → Diantar Kurir (Delivery)\n"+
			"3️⃣ Balas *BATAL* → Batalkan pesanan",
		order.OrderNumber,
		product.Name,
		parsed.Quantity,
		formatIDR(subtotal),
		formatIDR(taxAmount),
		formatIDR(order.TotalPrice),
	)
}

func (p *MessageProcessor) handleOrderTypeSelection(ctx context.Context, msg model.IncomingMessage, sess *model.Session) string {
	text := strings.ToLower(strings.TrimSpace(msg.Message))
	if text == "batal" || text == "3" {
		return p.handleCancel(ctx, sess, msg.Sender)
	}

	if text == "1" || text == "ambil" || text == "pickup" {
		sess.PendingOrderType = "pickup"
		sess.State = model.StateAwaitingConfirm

		if sess.PendingOrderID != nil {
			order, _ := p.orderSvc.GetOrderByID(ctx, *sess.PendingOrderID)
			if order != nil {
				return FormatOrderConfirmation(order)
			}
		}
		return "Pilihan Anda: *Ambil di Toko*.\nKetik *1* untuk Bayar Online (Midtrans) atau *2* untuk Bayar Cash di Kasir."
	}

	if text == "2" || text == "kirim" || text == "delivery" {
		sess.PendingOrderType = "delivery"

		// Cek apakah user sudah terhubung ke data customer (autofill)
		var existingCustomer *model.Customer
		if p.userRepo != nil && p.customerRepo != nil {
			u, err := p.userRepo.GetByTelegramChatID(ctx, msg.Sender)
			if err == nil && u != nil {
				c, errC := p.customerRepo.GetByUserID(ctx, u.ID)
				if errC == nil && c != nil {
					existingCustomer = c
				}
			}
		}

		if existingCustomer != nil && existingCustomer.Address.Valid && existingCustomer.Address.String != "" &&
			existingCustomer.Latitude.Valid && existingCustomer.Longitude.Valid {
			lat := existingCustomer.Latitude.Float64
			lng := existingCustomer.Longitude.Float64
			sess.PendingLat = &lat
			sess.PendingLng = &lng
			sess.PendingAddress = existingCustomer.Address.String

			// Hitung ongkir
			if p.deliverySvc != nil {
				est := p.deliverySvc.CalculateShippingCost(lat, lng)
				sess.PendingShipping = est.ShippingCost
				sess.PendingDistanceKm = est.DistanceKm
			}

			// Lanjut ke pemilihan tanggal pengantaran
			return p.askDeliveryDate(ctx, sess, existingCustomer.Address.String, sess.PendingShipping)
		}

		sess.State = model.StateAwaitingDeliveryAddress
		return "📍 *Pengantaran Kurir (Delivery)*\n\n" +
			"Silakan kirimkan alamat pengantaran Anda:\n" +
			"👉 Gunakan fitur *Kirim Lokasi (Share Location / GPS)* pada Telegram 📎, ATAU\n" +
			"👉 Ketik alamat lengkap Anda di sini."
	}

	return "Pilihan tidak valid. Silakan balas:\n1️⃣ *1* (Ambil di Toko)\n2️⃣ *2* (Diantar Kurir)"
}

func (p *MessageProcessor) handleDeliveryAddress(ctx context.Context, msg model.IncomingMessage, sess *model.Session) string {
	text := strings.TrimSpace(msg.Message)
	if strings.ToLower(text) == "batal" {
		return p.handleCancel(ctx, sess, msg.Sender)
	}

	var lat, lng float64
	var address string

	if msg.Latitude != nil && msg.Longitude != nil {
		lat = *msg.Latitude
		lng = *msg.Longitude
		address = fmt.Sprintf("Koordinat GPS: %.6f, %.6f", lat, lng)
	} else if strings.HasPrefix(text, "LOC:") {
		parts := strings.Split(strings.TrimPrefix(text, "LOC:"), ",")
		if len(parts) == 2 {
			_, _ = fmt.Sscanf(parts[0], "%f", &lat)
			_, _ = fmt.Sscanf(parts[1], "%f", &lng)
			address = text
		}
	} else {
		address = text
		// Jika hanya alamat teks tanpa koordinat, gunakan koordinat toko/default atau estimasi
		lat = WarehouseLat
		lng = WarehouseLng
	}

	sess.PendingAddress = address
	sess.PendingLat = &lat
	sess.PendingLng = &lng

	// Hitung Ongkir
	if p.deliverySvc != nil && lat != 0 && lng != 0 {
		est := p.deliverySvc.CalculateShippingCost(lat, lng)
		sess.PendingShipping = est.ShippingCost
		sess.PendingDistanceKm = est.DistanceKm
	}

	// Update / Create Customer Profile otomatis jika belum ada
	if p.userRepo != nil && p.customerRepo != nil {
		u, _ := p.userRepo.GetByTelegramChatID(ctx, msg.Sender)
		if u == nil {
			// Buat guest user & link telegram
			guestUser := &model.User{
				Email:          fmt.Sprintf("tele_%s@autoparts.local", msg.Sender),
				Name:           msg.SenderName,
				Role:           model.RoleCustomer,
				TelegramChatID: sql.NullString{String: msg.Sender, Valid: true},
				IsActive:       true,
			}
			if msg.SenderName == "" {
				guestUser.Name = "Customer Telegram"
			}
			if errU := p.userRepo.Create(ctx, guestUser); errU == nil && guestUser.ID > 0 {
				u = guestUser
			}
		}

		if u != nil {
			cust, _ := p.customerRepo.GetByUserID(ctx, u.ID)
			if cust == nil {
				_ = p.customerRepo.Create(ctx, &model.Customer{
					UserID:       u.ID,
					TypeCustomer: model.CustomerTypeIndividual,
					Address:      sql.NullString{String: address, Valid: address != ""},
					Latitude:     sql.NullFloat64{Float64: lat, Valid: lat != 0},
					Longitude:    sql.NullFloat64{Float64: lng, Valid: lng != 0},
				})
			} else {
				cust.Address = sql.NullString{String: address, Valid: address != ""}
				cust.Latitude = sql.NullFloat64{Float64: lat, Valid: lat != 0}
				cust.Longitude = sql.NullFloat64{Float64: lng, Valid: lng != 0}
				_ = p.customerRepo.Update(ctx, cust)
			}
		}
	}

	return p.askDeliveryDate(ctx, sess, address, sess.PendingShipping)
}

func (p *MessageProcessor) askDeliveryDate(ctx context.Context, sess *model.Session, address string, shippingCost float64) string {
	sess.State = model.StateAwaitingDeliveryDate

	t0 := time.Now()
	t1 := t0.Add(24 * time.Hour)
	t2 := t0.Add(48 * time.Hour)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📍 *Alamat:* %s\n", address))
	if sess.PendingDistanceKm > 0 {
		sb.WriteString(fmt.Sprintf("📏 *Estimasi Jarak:* %.2f km\n", sess.PendingDistanceKm))
	}
	sb.WriteString(fmt.Sprintf("🚚 *Ongkos Kirim:* Rp %s\n\n", formatIDR(shippingCost)))
	sb.WriteString("📅 *Pilih Tanggal Pengantaran:*\n")
	sb.WriteString(fmt.Sprintf("1️⃣ *Hari Ini* (%s)\n", t0.Format("02 Jan 2006")))
	sb.WriteString(fmt.Sprintf("2️⃣ *Besok* (%s)\n", t1.Format("02 Jan 2006")))
	sb.WriteString(fmt.Sprintf("3️⃣ *Lusa* (%s)\n", t2.Format("02 Jan 2006")))
	sb.WriteString("4️⃣ *Ketik tanggal lain* (Format: `YYYY-MM-DD`, contoh: `2026-08-25`)\n\n")
	sb.WriteString("Balas dengan angka *1, 2, 3* atau ketik tanggal pilihan Anda:")

	return sb.String()
}

func (p *MessageProcessor) handleDeliveryDateSelection(ctx context.Context, msg model.IncomingMessage, sess *model.Session) string {
	text := strings.TrimSpace(msg.Message)
	if strings.ToLower(text) == "batal" {
		return p.handleCancel(ctx, sess, msg.Sender)
	}

	var targetDate time.Time
	now := time.Now()

	switch text {
	case "1", "hari ini", "today":
		targetDate = now
	case "2", "besok", "tomorrow":
		targetDate = now.Add(24 * time.Hour)
	case "3", "lusa":
		targetDate = now.Add(48 * time.Hour)
	default:
		parsed, err := time.Parse("2006-01-02", text)
		if err != nil {
			return "⚠️ Format tanggal tidak valid.\nSilakan balas dengan angka *1* (Hari Ini), *2* (Besok), *3* (Lusa), atau ketik tanggal dengan format `YYYY-MM-DD` (contoh: `2026-08-25`)."
		}
		// Validasi tanggal tidak boleh di masa lampau
		todayZero := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if parsed.Before(todayZero) {
			return "⚠️ Tanggal pengantaran tidak boleh di masa lampau. Silakan pilih tanggal hari ini atau ke depan."
		}
		targetDate = parsed
	}

	sess.PendingDate = targetDate.Format("2006-01-02")
	return p.askDeliverySchedule(ctx, sess, targetDate)
}

func (p *MessageProcessor) askDeliverySchedule(ctx context.Context, sess *model.Session, targetDate time.Time) string {
	sess.State = model.StateAwaitingDeliverySchedule

	var schedules []model.DeliverySchedule
	if p.deliverySvc != nil {
		schedules, _ = p.deliverySvc.GetAvailableSchedules(ctx, targetDate)
	}

	sess.AvailSchedules = schedules

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📅 *Jadwal Pengantaran Tersedia (%s):*\n\n", targetDate.Format("02 Jan 2006")))

	if len(schedules) == 0 {
		sb.WriteString("_Belum ada slot jadwal tersedia untuk tanggal ini._\nKetik *BATAL* untuk membatalkan.")
		return sb.String()
	}

	hasSlot := false
	for i, s := range schedules {
		status := fmt.Sprintf("Tersedia (%d slot)", s.AvailableSlots)
		if s.IsFull {
			status = "❌ Penuh"
		} else {
			hasSlot = true
		}
		sb.WriteString(fmt.Sprintf("%d️⃣ *%s* (%s - %s) [%s]\n", i+1, s.SlotName, s.StartTime, s.EndTime, status))
	}

	if !hasSlot {
		sb.WriteString("\n⚠️ Semua jadwal pengantaran untuk tanggal ini sudah penuh. Silakan ketik *BATAL*.")
	} else {
		sb.WriteString("\nBalas dengan *nomor slot jadwal* yang Anda inginkan (misal: *1*).")
	}

	return sb.String()
}

func (p *MessageProcessor) handleDeliveryScheduleSelection(ctx context.Context, msg model.IncomingMessage, sess *model.Session) string {
	text := strings.TrimSpace(msg.Message)
	if strings.ToLower(text) == "batal" {
		return p.handleCancel(ctx, sess, msg.Sender)
	}

	if len(sess.AvailSchedules) == 0 {
		return "Tidak ada slot jadwal yang dipilih. Ketik *BATAL*."
	}

	var choice int
	if _, err := fmt.Sscanf(text, "%d", &choice); err != nil || choice < 1 || choice > len(sess.AvailSchedules) {
		return fmt.Sprintf("Nomor tidak valid. Balas dengan angka antara 1 dan %d.", len(sess.AvailSchedules))
	}

	selected := sess.AvailSchedules[choice-1]
	if selected.IsFull {
		return "Slot jadwal tersebut sudah penuh, silakan pilih nomor slot yang lain."
	}

	// Skenario A: Jika ini adalah alur Reschedule untuk delivery yang sudah ada
	if sess.PendingDeliveryID != nil && *sess.PendingDeliveryID > 0 {
		deliveryID := *sess.PendingDeliveryID
		targetDate, errD := time.Parse("2006-01-02", sess.PendingDate)
		if errD != nil {
			targetDate = time.Now().Add(24 * time.Hour)
		}

		if p.deliverySvc != nil {
			if errChg := p.deliverySvc.CustomerChangeSchedule(ctx, deliveryID, targetDate, selected.ID); errChg != nil {
				return fmt.Sprintf("⚠️ Gagal mengubah jadwal pengantaran: %s", errChg.Error())
			}
		}

		sess.State = model.StateIdle
		sess.PendingDeliveryID = nil

		return fmt.Sprintf(
			"✅ *Perubahan Jadwal Berhasil Diajukan!*\n\n"+
				"📅 Tanggal : *%s*\n"+
				"⏰ Slot    : *%s* (%s - %s)\n\n"+
				"Jadwal pengantaran baru Anda telah dikirimkan ke kurir dan menunggu persetujuan kurir. Terima kasih! 🚚",
			targetDate.Format("02 Jan 2006"), selected.SlotName, selected.StartTime, selected.EndTime,
		)
	}

	// Skenario B: Alur pembuatan order baru
	// Update order total price dengan menambahkan ongkir
	if sess.PendingOrderID != nil {
		order, _ := p.orderSvc.GetOrderByID(ctx, *sess.PendingOrderID)
		if order != nil {
			newTotal := order.TotalPrice + sess.PendingShipping
			_ = p.orderSvc.orderRepo.UpdateTotalPrice(ctx, order.ID, newTotal)

			// Buat delivery request di table deliveries
			var custID int64
			if p.userRepo != nil && p.customerRepo != nil {
				u, _ := p.userRepo.GetByTelegramChatID(ctx, msg.Sender)
				if u != nil {
					c, _ := p.customerRepo.GetByUserID(ctx, u.ID)
					if c != nil {
						custID = c.ID
					}
				}
			}

			if p.deliverySvc != nil {
				_, _ = p.deliverySvc.RequestDelivery(ctx, RequestDeliveryInput{
					OrderID:      order.ID,
					CustomerID:   custID,
					ScheduleID:   selected.ID,
					DeliveryDate: sess.PendingDate,
					Address:      sess.PendingAddress,
					Latitude:     sess.PendingLat,
					Longitude:    sess.PendingLng,
					Notes:        "Order via Telegram Chatbot",
				})
			}

			sess.State = model.StateAwaitingConfirm
			order.TotalPrice = newTotal

			return fmt.Sprintf(
				"🚚 *Jadwal Pengantaran Dipilih:*\n"+
					"⏰ Slot: *%s* (%s - %s)\n"+
					"📍 Alamat: %s\n"+
					"Ongkos Kirim: Rp %s\n\n"+
					"━━━━━━━━━━━━━━━━\n"+
					"🛒 *Total Akhir Pesanan: Rp %s*\n"+
					"━━━━━━━━━━━━━━━━\n"+
					"Pilih metode pembayaran:\n"+
					"1️⃣ Balas *1* atau *MIDTRANS* → Bayar Online (QRIS/Bank/E-Wallet)\n"+
					"2️⃣ Balas *BATAL* → Membatalkan pesanan\n\n"+
					"⏰ Reservasi berlaku selama 15 menit.",
				selected.SlotName, selected.StartTime, selected.EndTime,
				sess.PendingAddress,
				formatIDR(sess.PendingShipping),
				formatIDR(newTotal),
			)
		}
	}

	sess.State = model.StateAwaitingConfirm
	return "Jadwal pengantaran dikonfirmasi. Ketik *1* untuk bayar online via Midtrans."
}

func (p *MessageProcessor) handleRescheduleDecision(ctx context.Context, msg model.IncomingMessage, sess *model.Session) string {
	text := strings.ToLower(strings.TrimSpace(msg.Message))

	deliveryID := int64(0)
	if sess.PendingDeliveryID != nil {
		deliveryID = *sess.PendingDeliveryID
	}

	if deliveryID == 0 {
		sess.State = model.StateIdle
		return "Tidak ada permintaan reschedule pengantaran yang sedang aktif."
	}

	// 1. OPSI 1 / SETUJU: Terima saran jadwal dari kurir
	if text == "1" || text == "setuju" || text == "terima" || text == "ok" || text == "yes" {
		if p.deliverySvc != nil {
			if err := p.deliverySvc.CustomerAcceptReschedule(ctx, deliveryID); err != nil {
				return fmt.Sprintf("⚠️ Gagal menerima jadwal baru: %s", err.Error())
			}
		}
		sess.State = model.StateIdle
		sess.PendingDeliveryID = nil
		return "✅ *Konfirmasi Diterima!*\n\nJadwal pengantaran baru telah disetujui. Kurir kami akan mengantar pesanan Anda sesuai jadwal tersebut. Terima kasih!"
	}

	// 2. OPSI 2 / GANTI: Customer ingin memilih tanggal / slot lain
	if text == "2" || text == "ganti" || text == "ubah" || text == "reschedule" {
		address := sess.PendingAddress
		if address == "" {
			if p.deliverySvc != nil {
				if d, errD := p.deliverySvc.deliveryRepo.GetByID(ctx, deliveryID); errD == nil && d != nil {
					address = d.CustomerAddress
				}
			}
		}
		return p.askDeliveryDate(ctx, sess, address, sess.PendingShipping)
	}

	// 3. OPSI 3 / TOLAK: Batalkan pengantaran kurir & beralih ambil sendiri di toko
	if text == "3" || text == "tolak" || text == "batal" || text == "ambil sendiri" {
		if p.deliverySvc != nil {
			_ = p.deliverySvc.CustomerRejectReschedule(ctx, deliveryID)
		}
		sess.State = model.StateIdle
		sess.PendingDeliveryID = nil
		return "❌ *Pengantaran Kurir Dibatalkan*\n\nAnda dapat mengambil pesanan secara langsung di toko Niaga AutoParts. Terima kasih! 🏬"
	}

	return "Pilihan tidak valid. Silakan balas dengan:\n" +
		"1️⃣ *1* atau *SETUJU* → Terima jadwal saran kurir\n" +
		"2️⃣ *2* atau *GANTI*  → Pilih jadwal lain\n" +
		"3️⃣ *3* atau *TOLAK*  → Batalkan pengantaran (ambil di toko)"
}

var orderNumRegex = regexp.MustCompile(`(?i)APT-\d{8}-[A-Z0-9]{4}`)

func extractOrderNumber(text string) string {
	match := orderNumRegex.FindString(text)
	if match != "" {
		return strings.ToUpper(match)
	}
	return ""
}

func (p *MessageProcessor) handleConfirm(ctx context.Context, parsed *model.ParsedMessage, sess *model.Session, sender string) string {
	var existingOrder *model.Order
	var err error

	text := ""
	if parsed != nil {
		text = strings.TrimSpace(parsed.OriginalText)
	}

	// 1. Ekstrak nomor order spesifik dari teks (contoh: "Bayar APT-20260811-N2LI")
	orderNum := extractOrderNumber(text)
	if orderNum != "" {
		existingOrder, err = p.orderSvc.GetOrderByNumber(ctx, orderNum)
		if err != nil || existingOrder == nil {
			return fmt.Sprintf("Pesanan dengan kode *%s* tidak ditemukan.", orderNum)
		}
	}

	// 2. Jika user memilih nomor urut (1-9) dari daftar pesanan
	if existingOrder == nil {
		idx := -1
		if parsed != nil && parsed.Quantity > 0 {
			idx = parsed.Quantity - 1
		} else {
			cleanText := strings.TrimSpace(text)
			if len(cleanText) <= 2 && cleanText >= "1" && cleanText <= "9" {
				idx = int(cleanText[0] - '1')
			}
		}

		if idx >= 0 {
			orders, errList := p.orderSvc.GetOrdersByPhone(ctx, sender)
			if errList == nil && idx < len(orders) {
				existingOrder = &orders[idx]
			}
		}
	}

	// 3. Jika belum ketemu, ambil dari PendingOrderID di session
	if existingOrder == nil && sess.PendingOrderID != nil {
		existingOrder, _ = p.orderSvc.GetOrderByID(ctx, *sess.PendingOrderID)
	}

	// 4. Jika order dari session NULL atau SUDAH LUNAS / DIBATALKAN, cari order berstatus RESERVED/PENDING milik sender
	if existingOrder == nil || existingOrder.Status == model.OrderStatusPaid || existingOrder.Status == model.OrderStatusCancelled {
		orders, errList := p.orderSvc.GetOrdersByPhone(ctx, sender)
		if errList == nil && len(orders) > 0 {
			for i := range orders {
				if orders[i].Status == model.OrderStatusReserved || orders[i].Status == model.OrderStatusPending {
					existingOrder = &orders[i]
					break
				}
			}
		}
	}

	if existingOrder == nil {
		return "Tidak ada pesanan yang menunggu pembayaran atau konfirmasi."
	}

	// Simpan ID order aktif yang ditemukan ke session
	sess.PendingOrderID = &existingOrder.ID

	if existingOrder.Status == model.OrderStatusPaid {
		return fmt.Sprintf("✅ Pesanan *%s* sudah *LUNAS* dan sedang diproses oleh gudang. Terima kasih!", existingOrder.OrderNumber)
	}

	if existingOrder.Status == model.OrderStatusCancelled {
		return fmt.Sprintf("❌ Pesanan *%s* telah *DIBATALKAN*.", existingOrder.OrderNumber)
	}

	textLower := strings.ToLower(text)
	isCash := textLower == "2" || textLower == "cash" || textLower == "tunai"

	if isCash {
		_ = p.orderSvc.orderRepo.UpdatePaymentMethod(ctx, existingOrder.ID, "cash")
		newExpiry := time.Now().Add(24 * time.Hour)
		_ = p.orderSvc.orderRepo.UpdateExpiresAt(ctx, existingOrder.ID, newExpiry)
		_ = p.sessionRepo.Reset(ctx, sender)

		prodName := "-"
		if len(existingOrder.Items) > 0 {
			prodName = existingOrder.Items[0].ProductName
		}

		return fmt.Sprintf("✅ *Pesanan Dikonfirmasi (Bayar Cash di Toko)*\n\n"+
			"No. Pesanan : *%s*\n"+
			"Produk      : %s\n"+
			"Total       : *Rp %s*\n\n"+
			"🏬 *Instruksi Pembayaran & Pengambilan:*\n"+
			"1. Datang ke toko Niaga AutoParts.\n"+
			"2. Tunjukkan No. Pesanan *%s* ke admin kasir.\n"+
			"3. Bayar tunai (cash) langsung di toko saat mengambil barang.\n\n"+
			"⏰ Reservasi stok berlaku selama *24 Jam* (sampai %s WIB).\n"+
			"Terima kasih! 🙏",
			existingOrder.OrderNumber,
			prodName,
			formatIDR(existingOrder.TotalPrice),
			existingOrder.OrderNumber,
			newExpiry.Format("15:04, 02 Jan 2006"),
		)
	}

	// Midtrans Payment (Online)
	if p.midtransSvc == nil {
		return "Fasilitas pembayaran Midtrans saat ini belum aktif. Silakan pilih bayar Cash di Toko."
	}

	_ = p.orderSvc.orderRepo.UpdatePaymentMethod(ctx, existingOrder.ID, "qris")

	snapResp, err := p.midtransSvc.CreateSnapTransaction(ctx, existingOrder.ID)
	if err != nil {
		p.logger.Error("failed to create snap transaction for chatbot order", zap.Error(err))
		return fmt.Sprintf("⚠️ *Gagal Membuat Link Pembayaran Midtrans:*\n%s", err.Error())
	}

	_ = p.sessionRepo.Reset(ctx, sender)

	prodName := "-"
	if len(existingOrder.Items) > 0 {
		prodName = existingOrder.Items[0].ProductName
	}

	return fmt.Sprintf(
		"🛒 *Pesanan Dikonfirmasi — Pembayaran Midtrans*\n\n"+
			"No. Pesanan : *%s*\n"+
			"Produk      : %s\n"+
			"Total       : *Rp %s*\n\n"+
			"💳 *Silakan lakukan pembayaran melalui link Midtrans berikut:*\n%s\n\n"+
			"⏰ Link pembayaran & reservasi stok berlaku selama 15 menit.\n"+
			"Setelah pembayaran selesai, Anda akan menerima konfirmasi otomatis di sini.",
		existingOrder.OrderNumber,
		prodName,
		formatIDR(existingOrder.TotalPrice),
		snapResp.RedirectURL,
	)
}

func (p *MessageProcessor) handleCancel(ctx context.Context, sess *model.Session, sender string) string {
	if sess.PendingOrderID == nil {
		return "Tidak ada pesanan yang aktif untuk dibatalkan."
	}

	if err := p.orderSvc.CancelOrder(ctx, *sess.PendingOrderID); err != nil {
		p.logger.Error("cancel order failed", zap.Error(err))
		return FormatError("order_failed")
	}

	_ = p.sessionRepo.Reset(ctx, sender)
	return "✅ Pesanan berhasil dibatalkan. Stok telah dikembalikan.\n\nAda yang bisa saya bantu?"
}

func (p *MessageProcessor) handleCheckOrders(ctx context.Context, sender string) string {
	orders, err := p.orderSvc.GetOrdersByPhone(ctx, sender)
	if err != nil || len(orders) == 0 {
		return "Anda belum memiliki pesanan."
	}

	reply := "📦 *Pesanan Anda:*\n\n"
	for _, o := range orders {
		statusEmoji := map[model.OrderStatus]string{
			model.OrderStatusPending:   "⏳",
			model.OrderStatusReserved:  "🔒",
			model.OrderStatusPaid:      "✅",
			model.OrderStatusCancelled: "❌",
		}
		emoji := statusEmoji[o.Status]
		itemSummary := ""
		if len(o.Items) > 0 {
			itemSummary = fmt.Sprintf(" — %s (x%d)", o.Items[0].ProductName, o.Items[0].Quantity)
			if len(o.Items) > 1 {
				itemSummary += fmt.Sprintf(" +%d item lainnya", len(o.Items)-1)
			}
		}
		reply += fmt.Sprintf("%s *%s*%s — Rp %s\n",
			emoji, o.OrderNumber, itemSummary, formatIDR(o.TotalPrice))
	}
	return reply
}

// handleImage uses Gemini AI to identify a product from the attachment URL,
// then automatically searches the database for matching products using the
// existing search algorithm (normalization → typo correction → DB trigram search).
// Works identically for Fonnte and Telegram.
func (p *MessageProcessor) handleImage(ctx context.Context, msg model.IncomingMessage, sess *model.Session) {
	sender := msg.Sender

	// Step 1: Ask Gemini to identify the spare part in the image (bilingual)
	aiResult, err := p.aiSvc.IdentifyProductFromImageURL(ctx, msg.AttachmentURL)
	if err != nil {
		p.logger.Warn("image identification failed", zap.Error(err))
		_ = p.messagingSvc.SendText(ctx, msg.Platform, sender,
			"Maaf, tidak dapat memproses gambar. Coba kirim ulang atau ketik nama produknya.")
		return
	}

	// Combine Indonesian and English candidates into one list for searching
	allCandidates := append(aiResult.PossibleProductsID, aiResult.PossibleProductsEN...)
	if len(allCandidates) == 0 {
		_ = p.messagingSvc.SendText(ctx, msg.Platform, sender,
			"Maaf, saya tidak dapat mengidentifikasi suku cadang dari foto ini. "+
				"Coba kirim foto yang lebih jelas, atau ketik nama produknya langsung.")
		return
	}

	p.logger.Info("image identified by AI",
		zap.String("sender", sender),
		zap.Strings("candidates_id", aiResult.PossibleProductsID),
		zap.Strings("candidates_en", aiResult.PossibleProductsEN),
	)

	// Step 2: Search the database for every AI candidate, tracking results
	// separately per language group so we can use intersection for precision.
	seenByID := make(map[int64]struct{}) // found by Indonesian candidates
	seenByEN := make(map[int64]struct{}) // found by English candidates
	productByID := make(map[int64]model.Product)

	searchGroup := func(candidates []string, seen map[int64]struct{}) {
		for _, candidate := range candidates {
			results, searchErr := p.productSvc.Search(ctx, candidate)
			if searchErr != nil {
				p.logger.Debug("candidate search returned no results",
					zap.String("candidate", candidate), zap.Error(searchErr))
				continue
			}
			for _, prod := range results {
				seen[prod.ID] = struct{}{}
				productByID[prod.ID] = prod
			}
		}
	}
	searchGroup(aiResult.PossibleProductsID, seenByID)
	searchGroup(aiResult.PossibleProductsEN, seenByEN)

	// Build union of all found products
	unionIDs := make(map[int64]struct{})
	for id := range seenByID {
		unionIDs[id] = struct{}{}
	}
	for id := range seenByEN {
		unionIDs[id] = struct{}{}
	}

	// If union is small (≤5), use it directly.
	// If union is large, fall back to intersection (ID ∩ EN) to remove false positives
	// caused by generic words like "motor" matching all motorcycle parts.
	var merged []model.Product
	const maxUnionResults = 5
	if len(unionIDs) <= maxUnionResults {
		for id := range unionIDs {
			merged = append(merged, productByID[id])
		}
	} else {
		// Intersection: must appear in BOTH language groups
		for id := range seenByID {
			if _, inEN := seenByEN[id]; inEN {
				merged = append(merged, productByID[id])
			}
		}
		// If intersection is also empty (languages returned totally disjoint sets),
		// fall back to a capped union so the user at least gets something.
		if len(merged) == 0 {
			count := 0
			for id := range unionIDs {
				if count >= maxUnionResults {
					break
				}
				merged = append(merged, productByID[id])
				count++
			}
		}
	}

	// Step 3: Build reply based on how many DB matches were found
	var reply string
	if len(merged) == 0 {
		// No DB matches — show Gemini's bilingual suggestions for manual follow-up
		reply = FormatImageNoDBMatch(aiResult.PossibleProductsID, aiResult.PossibleProductsEN)
	} else if len(merged) == 1 {
		// Exactly one match → show full product detail immediately
		product, refs, detailErr := p.productSvc.GetWithPriceRefs(ctx, merged[0].ID)
		if detailErr != nil {
			reply = FormatProductList(merged)
		} else {
			sess.LastProductID = &product.ID
			sess.LastProductName = product.Name
			reply = FormatImageFoundSingle(aiResult.PossibleProductsID, aiResult.PossibleProductsEN, product, refs)
		}
		sess.State = model.StateIdle
	} else {
		// Multiple matches → numbered list so user can pick by number
		sess.State = model.StateAwaitingProductSelection
		sess.SearchResults = merged
		reply = FormatImageFoundMultiple(aiResult.PossibleProductsID, aiResult.PossibleProductsEN, merged)
	}

	sess.LastIntent = string(model.IntentIdentifyImage)
	sess.ExpiresAt = time.Now().Add(p.sessionTTL)
	_ = p.sessionRepo.Save(ctx, sess)

	if err := p.messagingSvc.SendText(ctx, msg.Platform, sender, reply); err != nil {
		p.logger.Error("send image reply failed", zap.Error(err))
	}
}

func (p *MessageProcessor) handleTelegramStart(ctx context.Context, msg model.IncomingMessage, sess *model.Session) string {
	parts := strings.Fields(msg.Message)
	if len(parts) < 2 {
		return FormatWelcome(msg.SenderName)
	}

	orderParam := strings.TrimSpace(parts[1])
	if !strings.HasPrefix(orderParam, "ORD-") && !strings.HasPrefix(orderParam, "ord-") {
		return FormatWelcome(msg.SenderName)
	}

	orderNum := strings.ToUpper(orderParam)

	// Link telegram_chat_id to this order
	if err := p.orderSvc.LinkTelegramChatID(ctx, orderNum, msg.Sender); err != nil {
		p.logger.Error("failed to link telegram chat_id to order", zap.String("order_number", orderNum), zap.Error(err))
		return "Gagal menghubungkan pesanan ke Telegram."
	}

	order, err := p.orderSvc.GetOrderByNumber(ctx, orderNum)
	if err != nil {
		p.logger.Warn("order not found for deep link", zap.String("order_number", orderNum), zap.Error(err))
		return fmt.Sprintf("Pesanan *%s* tidak ditemukan.", orderNum)
	}

	if order.Status == model.OrderStatusPaid {
		return fmt.Sprintf("✅ *Pesanan Terhubung*\n\nPesanan *%s* (Total: Rp %s) sudah *LUNAS* dan sedang diproses oleh gudang. Terima kasih!",
			order.OrderNumber, formatIDR(order.TotalPrice))
	}

	if order.Status == model.OrderStatusCancelled {
		return fmt.Sprintf("❌ *Pesanan Terhubung*\n\nPesanan *%s* telah *DIBATALKAN*.", order.OrderNumber)
	}

	reply := fmt.Sprintf("🔗 *Pesanan %s Berhasil Terhubung ke Telegram!*\n\nStatus: *%s*\nTotal: Rp %s",
		order.OrderNumber, order.Status, formatIDR(order.TotalPrice))

	if p.midtransSvc != nil {
		snapResp, err := p.midtransSvc.CreateSnapTransaction(ctx, order.ID)
		if err == nil && snapResp != nil && snapResp.RedirectURL != "" {
			reply += fmt.Sprintf("\n\n💳 *Link Pembayaran Midtrans:*\n%s", snapResp.RedirectURL)
		} else if err != nil {
			reply += fmt.Sprintf("\n\n⚠️ *Info Pembayaran:* %s", err.Error())
		}
	}

	return reply
}

func (p *MessageProcessor) handleHistory(ctx context.Context, parsed *model.ParsedMessage, msg model.IncomingMessage) string {
	sender := msg.Sender

	// If no month parameter was specified, return 3 days summary in chat
	if parsed.Month == 0 {
		orders, err := p.orderSvc.orderRepo.GetHistoryLastDays(ctx, sender, 3)
		if err != nil {
			p.logger.Error("failed to get last 3 days order history", zap.Error(err))
			return FormatError("db_error")
		}
		return FormatOrderHistorySummary(orders, 3)
	}

	// Month parameter specified -> generate Excel and send document
	year := parsed.Year
	if year == 0 {
		year = time.Now().Year()
	}

	orders, err := p.orderSvc.orderRepo.GetHistoryMonthly(ctx, sender, year, parsed.Month)
	if err != nil {
		p.logger.Error("failed to get monthly order history", zap.Error(err))
		return FormatError("db_error")
	}

	if len(orders) == 0 {
		return fmt.Sprintf("📜 Tidak ditemukan data pesanan untuk bulan %02d/%d.", parsed.Month, year)
	}

	var buf bytes.Buffer
	if p.reportSvc != nil {
		if err := p.reportSvc.GenerateUserOrdersExcel(orders, year, parsed.Month, &buf); err != nil {
			p.logger.Error("failed to generate monthly excel", zap.Error(err))
			return FormatError("db_error")
		}
	} else {
		return "Laporan Excel sementara tidak tersedia."
	}

	filename := fmt.Sprintf("riwayat_pesanan_%d_%02d.xlsx", year, parsed.Month)
	caption := fmt.Sprintf("📊 *Laporan Riwayat Pesanan Bulan %02d/%d*\nTotal Transaksi: %d pesanan.", parsed.Month, year, len(orders))

	if teleSvc, ok := p.messagingSvc.(*TelegramService); ok {
		if err := teleSvc.SendDocumentBytes(ctx, msg.Platform, sender, buf.Bytes(), filename, caption); err != nil {
			p.logger.Error("failed to send document bytes to telegram", zap.Error(err))
			return "Gagal mengirimkan file Excel ke chat."
		}
		return ""
	}

	return fmt.Sprintf("📊 *Laporan Excel Bulan %02d/%d Berhasil Dibuat!*", parsed.Month, year)
}
