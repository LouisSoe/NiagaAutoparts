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
// It is provider-agnostic: the concrete MessageSender (Fonnte or Telegram)
// is injected via the model.MessageSender interface, so the same logic works
// for both WhatsApp and Telegram without any modification here.
type MessageProcessor struct {
	intentSvc    *IntentService
	productSvc   *ProductService
	orderSvc     *OrderService
	sessionRepo  *repository.SessionRepository
	messagingSvc model.MessageSender // interface — not tied to any provider
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

	if msg.IsImageMessage() {
		p.handleImage(ctx, msg, sess)
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
		reply = p.handleOrder(ctx, parsed, sess, sender)

	case model.IntentConfirmOrder:
		reply = p.handleConfirm(ctx, sess, sender)

	case model.IntentCancelOrder:
		reply = p.handleCancel(ctx, sess, sender)

	case model.IntentCheckOrder:
		reply = p.handleCheckOrders(ctx, sender)

	default:
		parsed.ProductQuery = msg.Message
		reply = p.handleSearch(ctx, parsed, sess)
	}

	sess.LastIntent = string(parsed.Intent)
	sess.ExpiresAt = time.Now().Add(p.sessionTTL)
	if err := p.sessionRepo.Save(ctx, sess); err != nil {
		p.logger.Warn("failed to save session", zap.Error(err))
	}

	if err := p.messagingSvc.SendText(ctx, msg.Platform, sender, reply); err != nil {
		p.logger.Error("failed to send reply", zap.String("sender", sender), zap.Error(err))
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

func (p *MessageProcessor) handleOrder(ctx context.Context, parsed *model.ParsedMessage, sess *model.Session, sender string) string {
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

	order, err := p.orderSvc.CreateReservation(ctx, sender, product, parsed.Quantity)
	if err != nil {
		p.logger.Error("create reservation failed", zap.Error(err))
		return FormatError("order_failed")
	}

	sess.PendingOrderID = &order.ID
	sess.State = model.StateAwaitingConfirm
	return FormatOrderConfirmation(order)
}

func (p *MessageProcessor) handleConfirm(ctx context.Context, sess *model.Session, sender string) string {
	if sess.PendingOrderID == nil {
		return "Tidak ada pesanan yang menunggu konfirmasi."
	}

	order, err := p.orderSvc.ConfirmOrder(ctx, *sess.PendingOrderID)
	if err != nil {
		p.logger.Error("confirm order failed", zap.Int64("order_id", *sess.PendingOrderID), zap.Error(err))
		return FormatError("order_failed")
	}

	_ = p.sessionRepo.Reset(ctx, sender)
	return FormatOrderSuccess(order)
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
		reply += fmt.Sprintf("%s *%s* — %s (x%d) — Rp %s\n",
			emoji, o.OrderNumber, o.ProductName, o.Quantity, formatIDR(o.TotalPrice))
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