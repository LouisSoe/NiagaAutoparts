package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/ai"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"go.uber.org/zap"
)

// MessageProcessor is the brain of the chatbot.
// It ties together intent detection, product search, and order management.
type MessageProcessor struct {
	intentSvc    *IntentService
	productSvc   *ProductService
	orderSvc     *OrderService
	sessionRepo  *repository.SessionRepository
	messagingSvc *MessagingService
	aiSvc        *ai.AIService
	logger       *zap.Logger
	sessionTTL   time.Duration
}

// NewMessageProcessor wires up all dependencies.
func NewMessageProcessor(
	intentSvc *IntentService,
	productSvc *ProductService,
	orderSvc *OrderService,
	sessionRepo *repository.SessionRepository,
	messagingSvc *MessagingService,
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

// Process is the main entry point for async message handling.
func (p *MessageProcessor) Process(ctx context.Context, payload model.FonnteWebhookPayload) {
	phone := payload.Sender

	sess, err := p.sessionRepo.GetOrCreate(ctx, phone)
	if err != nil {
		p.logger.Error("session error", zap.String("phone", phone), zap.Error(err))
		_ = p.messagingSvc.SendText(ctx, phone, FormatError("session_error"))
		return
	}

	if payload.IsImageMessage() {
		p.handleImage(ctx, payload, sess)
		return
	}

	parsed, err := p.intentSvc.Detect(ctx, payload.Message, sess)
	if err != nil {
		p.logger.Error("intent detection failed", zap.Error(err))
		parsed = &model.ParsedMessage{
			OriginalText: payload.Message,
			Intent:       model.IntentUnknown,
		}
	}

	p.logger.Info("intent resolved",
		zap.String("phone", phone),
		zap.String("intent", string(parsed.Intent)),
		zap.Bool("from_ai", parsed.FromAI),
	)

	var reply string
	switch parsed.Intent {
	case model.IntentGreeting:
		reply = FormatWelcome(payload.Name)
		sess.State = model.StateIdle

	case model.IntentHelp:
		reply = FormatHelp()
		sess.State = model.StateIdle

	case model.IntentSearchProduct, model.IntentAskPrice:
		reply = p.handleSearch(ctx, parsed, sess)

	case model.IntentSelectProduct:
		reply = p.handleProductSelection(ctx, parsed, sess)

	case model.IntentOrder:
		reply = p.handleOrder(ctx, parsed, sess, phone)

	case model.IntentConfirmOrder:
		reply = p.handleConfirm(ctx, sess, phone)

	case model.IntentCancelOrder:
		reply = p.handleCancel(ctx, sess, phone)

	case model.IntentCheckOrder:
		reply = p.handleCheckOrders(ctx, phone)

	default:
		parsed.ProductQuery = payload.Message
		reply = p.handleSearch(ctx, parsed, sess)
	}

	sess.LastIntent = string(parsed.Intent)
	sess.ExpiresAt = time.Now().Add(p.sessionTTL)
	if err := p.sessionRepo.Save(ctx, sess); err != nil {
		p.logger.Warn("failed to save session", zap.Error(err))
	}

	if err := p.messagingSvc.SendText(ctx, phone, reply); err != nil {
		p.logger.Error("failed to send reply", zap.String("phone", phone), zap.Error(err))
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
	// Jika tidak sedang menunggu pilihan produk
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

func (p *MessageProcessor) handleOrder(ctx context.Context, parsed *model.ParsedMessage, sess *model.Session, phone string) string {
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

	order, err := p.orderSvc.CreateReservation(ctx, phone, product, parsed.Quantity)
	if err != nil {
		p.logger.Error("create reservation failed", zap.Error(err))
		return FormatError("order_failed")
	}

	sess.PendingOrderID = &order.ID
	sess.State = model.StateAwaitingConfirm
	return FormatOrderConfirmation(order)
}

func (p *MessageProcessor) handleConfirm(ctx context.Context, sess *model.Session, phone string) string {
	if sess.PendingOrderID == nil {
		return "Tidak ada pesanan yang menunggu konfirmasi."
	}

	order, err := p.orderSvc.ConfirmOrder(ctx, *sess.PendingOrderID)
	if err != nil {
		p.logger.Error("confirm order failed", zap.Int64("order_id", *sess.PendingOrderID), zap.Error(err))
		return FormatError("order_failed")
	}

	_ = p.sessionRepo.Reset(ctx, phone)
	return FormatOrderSuccess(order)
}

func (p *MessageProcessor) handleCancel(ctx context.Context, sess *model.Session, phone string) string {
	if sess.PendingOrderID == nil {
		return "Tidak ada pesanan yang aktif untuk dibatalkan."
	}

	if err := p.orderSvc.CancelOrder(ctx, *sess.PendingOrderID); err != nil {
		p.logger.Error("cancel order failed", zap.Error(err))
		return FormatError("order_failed")
	}

	_ = p.sessionRepo.Reset(ctx, phone)
	return "✅ Pesanan berhasil dibatalkan. Stok telah dikembalikan.\n\nAda yang bisa saya bantu?"
}

func (p *MessageProcessor) handleCheckOrders(ctx context.Context, phone string) string {
	orders, err := p.orderSvc.GetOrdersByPhone(ctx, phone)
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
		reply += fmt.Sprintf("%s *%s* — %s (x%d) — Rp %s\n",
			emoji, o.OrderNumber, o.ProductName, o.Quantity, formatIDR(o.TotalPrice))
	}
	return reply
}

func (p *MessageProcessor) handleImage(ctx context.Context, payload model.FonnteWebhookPayload, sess *model.Session) {
	phone := payload.Sender

	result, err := p.aiSvc.IdentifyProductFromImageURL(ctx, payload.File)
	if err != nil {
		p.logger.Warn("image identification failed", zap.Error(err))
		_ = p.messagingSvc.SendText(ctx, phone,
			"Maaf, tidak dapat memproses gambar. Coba kirim ulang atau ketik nama produknya.")
		return
	}

	reply := FormatImageIdentifyResult(result.PossibleProducts)
	if err := p.messagingSvc.SendText(ctx, phone, reply); err != nil {
		p.logger.Error("send image reply failed", zap.Error(err))
	}

	sess.State = model.StateSearching
	sess.LastIntent = string(model.IntentIdentifyImage)
	sess.ExpiresAt = time.Now().Add(p.sessionTTL)
	_ = p.sessionRepo.Save(ctx, sess)
}