package service

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"github.com/louissoe/niaga-autoparts/internal/utils"
	"go.uber.org/zap"
)

const (
	// ReservationWindow is how long stock is held before auto-cancellation (for online payment/bot).
	ReservationWindow = 15 * time.Minute

	// CashReservationWindow is the payment expiry window for cash payment method (24 hours).
	CashReservationWindow = 24 * time.Hour
)

// OrderService handles the order lifecycle: create, reserve, confirm, cancel.
type OrderService struct {
	orderRepo    *repository.OrderRepository
	productRepo  *repository.ProductRepository
	userRepo     *repository.UserRepository
	customerRepo *repository.CustomerRepository
	msgSender    model.MessageSender
	midtransSvc  *MidtransService
	notifierSvc  *TelegramNotifierService
	logger       *zap.Logger
}

func NewOrderService(
	orderRepo *repository.OrderRepository,
	productRepo *repository.ProductRepository,
	logger *zap.Logger,
) *OrderService {
	return &OrderService{
		orderRepo:   orderRepo,
		productRepo: productRepo,
		logger:      logger,
	}
}

func (s *OrderService) SetUserRepository(repo *repository.UserRepository) {
	s.userRepo = repo
}

func (s *OrderService) SetCustomerRepository(repo *repository.CustomerRepository) {
	s.customerRepo = repo
}

func (s *OrderService) SetMessageSender(sender model.MessageSender) {
	s.msgSender = sender
}

func (s *OrderService) SetMidtransService(midtransSvc *MidtransService) {
	s.midtransSvc = midtransSvc
}

func (s *OrderService) SetNotifierService(notifierSvc *TelegramNotifierService) {
	s.notifierSvc = notifierSvc
}

func (s *OrderService) GetOrderByNumber(ctx context.Context, orderNum string) (*model.Order, error) {
	return s.orderRepo.GetByOrderNumber(ctx, orderNum)
}

func (s *OrderService) LinkTelegramChatID(ctx context.Context, orderNum, chatID string) error {
	return s.orderRepo.LinkTelegramChatID(ctx, orderNum, chatID)
}

// CreateReservation creates an order in "reserved" status and holds the stock.
// This is the first step of the 2-phase order flow.
func (s *OrderService) CreateReservation(ctx context.Context, sender string, platform model.Platform, product *model.Product, qty int) (*model.Order, error) {
	if qty <= 0 {
		return nil, fmt.Errorf("quantity must be at least 1")
	}
	if product.AvailableStock() < qty {
		return nil, fmt.Errorf("stok tidak cukup. Tersedia: %d %s", product.AvailableStock(), product.Unit)
	}

	// Reserve stock atomically
	if err := s.productRepo.ReserveStock(ctx, product.ID, qty); err != nil {
		return nil, fmt.Errorf("gagal mereservasi stok: %w", err)
	}

	sourceStr := "wa"
	var teleChatID sql.NullString
	var userID sql.NullInt64
	if platform == model.PlatformTelegram {
		sourceStr = "telegram"
		teleChatID = sql.NullString{String: sender, Valid: true}
		if s.userRepo != nil {
			if u, err := s.userRepo.GetByTelegramChatID(ctx, sender); err == nil && u != nil {
				userID = sql.NullInt64{Int64: u.ID, Valid: true}
			}
		}
	}

	expiry := time.Now().Add(ReservationWindow)
	subtotal := product.SellingPrice * float64(qty)
	order := &model.Order{
		OrderNumber:    generateOrderNumber(),
		UserID:         userID,
		TotalPrice:     subtotal,
		Status:         model.OrderStatusReserved,
		Source:         sourceStr,
		TelegramChatID: teleChatID,
		Notes:          sender,
		ExpiresAt:      &expiry,
		Items: []model.OrderDetail{
			{
				ProductID:   product.ID,
				ProductName: product.Name,
				Quantity:    qty,
				UnitPrice:   product.SellingPrice,
				Subtotal:    subtotal,
			},
		},
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		// Rollback the reservation on failure
		_ = s.productRepo.ReleaseReservation(ctx, product.ID, qty)
		return nil, fmt.Errorf("gagal membuat order: %w", err)
	}

	s.logger.Info("order reserved",
		zap.String("order_number", order.OrderNumber),
		zap.String("sender", sender),
		zap.Int("qty", qty),
		zap.Float64("total", order.TotalPrice),
	)
	return order, nil
}

// ConfirmOrder transitions an order from reserved → paid and deducts stock permanently.
// Uses a transaction to ensure atomicity between order update and stock deduction.
func (s *OrderService) ConfirmOrder(ctx context.Context, orderID int64) (*model.Order, error) {
	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}
	if order.Status != model.OrderStatusReserved {
		return nil, fmt.Errorf("order tidak dapat dikonfirmasi (status: %s)", order.Status)
	}

	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Propagate the authenticated actor into the PostgreSQL audit trigger
	// via SET LOCAL — scoped to this transaction, no pool-leak risk.
	// NOTE: "app.current_user" must be quoted — current_user is a reserved keyword.
	if actor := utils.ActorFromContext(ctx); actor != "" {
		safeActor := strings.ReplaceAll(actor, "'", "''")
		_, _ = tx.ExecContext(ctx, fmt.Sprintf(`SET LOCAL "app.current_user" = '%s'`, safeActor))
	}

	for _, item := range order.Items {
		if err = s.productRepo.DeductStock(ctx, tx, item.ProductID, item.Quantity); err != nil {
			return nil, fmt.Errorf("deduct stock: %w", err)
		}
	}
	if err = s.orderRepo.UpdateStatusTx(ctx, tx, order.ID, model.OrderStatusPaid); err != nil {
		return nil, fmt.Errorf("update order status: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	order.Status = model.OrderStatusPaid
	s.logger.Info("order confirmed", zap.String("order_number", order.OrderNumber))
	return order, nil
}

// CancelOrder cancels an order and releases the stock reservation.
func (s *OrderService) CancelOrder(ctx context.Context, orderID int64) error {
	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("order not found: %w", err)
	}
	if order.Status == model.OrderStatusCancelled || order.Status == model.OrderStatusPaid {
		return fmt.Errorf("order sudah %s, tidak bisa dibatalkan", order.Status)
	}

	if err := s.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusCancelled); err != nil {
		return err
	}
	// Release reserved stock for all items
	if order.Status == model.OrderStatusReserved {
		for _, item := range order.Items {
			_ = s.productRepo.ReleaseReservation(ctx, item.ProductID, item.Quantity)
		}
	}

	s.logger.Info("order cancelled", zap.String("order_number", order.OrderNumber))
	return nil
}

// ExpireOldReservations is meant to be run on a background ticker.
func (s *OrderService) ExpireOldReservations(ctx context.Context) {
	expired, err := s.orderRepo.ExpireReservations(ctx)
	if err != nil {
		s.logger.Error("expire reservations failed", zap.Error(err))
		return
	}
	for _, o := range expired {
		for _, item := range o.Items {
			_ = s.productRepo.ReleaseReservation(ctx, item.ProductID, item.Quantity)
			s.logger.Info("reservation expired",
				zap.Int64("order_id", o.ID),
				zap.Int64("product_id", item.ProductID),
			)
		}
	}
}

type CreateOrderItemInput struct {
	ProductID int64   `json:"product_id"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type CreateOrderInput struct {
	UserID        int64                  `json:"user_id"`
	CustomerName  string                 `json:"customer_name"`
	CustomerPhone string                 `json:"customer_phone"`
	CustomerEmail string                 `json:"customer_email"`
	Address       string                 `json:"address"`
	AmountPaid    float64                `json:"amount_paid"`
	ChangeAmount  float64                `json:"change_amount"`
	Source        string                 `json:"source"`
	PaymentMethod string                 `json:"payment_method"`
	Status        string                 `json:"status"`
	OrderType     string                 `json:"order_type"` // delivery or pickup
	Notes         string                 `json:"notes"`
	ShippingCost  *float64               `json:"shipping_cost"`
	TaxAmount     float64                `json:"tax_amount"`
	Items         []CreateOrderItemInput `json:"items"`
}

func (s *OrderService) GetOrderByID(ctx context.Context, id int64) (*model.Order, error) {
	return s.orderRepo.GetByID(ctx, id)
}

func (s *OrderService) CreateOrderHeaderWithItems(ctx context.Context, input CreateOrderInput) (*model.Order, error) {
	if len(input.Items) == 0 {
		return nil, fmt.Errorf("items tidak boleh kosong")
	}

	var totalPrice float64
	orderDetails := make([]model.OrderDetail, 0, len(input.Items))

	for _, item := range input.Items {
		if item.Quantity <= 0 {
			return nil, fmt.Errorf("quantity harus lebih besar dari 0")
		}
		subtotal := item.UnitPrice * float64(item.Quantity)
		totalPrice += subtotal

		orderDetails = append(orderDetails, model.OrderDetail{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
			Subtotal:  subtotal,
		})
	}

	// Tambahkan Tax (PPN) jika ada
	if input.TaxAmount > 0 {
		totalPrice += input.TaxAmount
	}

	// Tambahkan Ongkir jika ada (opsional / bisa null)
	if input.ShippingCost != nil && *input.ShippingCost > 0 {
		totalPrice += *input.ShippingCost
	}

	source := input.Source
	if source == "" {
		source = "pos"
	}
	paymentMethod := input.PaymentMethod
	if paymentMethod == "" {
		paymentMethod = "cash"
	}

	orderStatus := model.OrderStatusPaid
	if input.Status != "" {
		orderStatus = model.OrderStatus(input.Status)
	} else if paymentMethod == "midtrans" || source == "web" {
		orderStatus = model.OrderStatusReserved
	}

	order := &model.Order{
		OrderNumber:  generateOrderNumber(),
		TotalPrice:   totalPrice,
		AmountPaid:   input.AmountPaid,
		ChangeAmount: input.ChangeAmount,
		Status:       orderStatus,
		Source:       source,
		Notes:        input.Notes,
		Items:        orderDetails,
	}

	if orderStatus == model.OrderStatusReserved || orderStatus == model.OrderStatusPending {
		window := ReservationWindow
		if strings.EqualFold(paymentMethod, "cash") {
			window = CashReservationWindow
		}
		expiry := time.Now().Add(window)
		order.ExpiresAt = &expiry
	}

	var userTelegramChatID string
	if input.UserID > 0 {
		if s.orderRepo.UserExists(ctx, input.UserID) {
			order.UserID.Int64 = input.UserID
			order.UserID.Valid = true

			if s.userRepo != nil {
				if u, err := s.userRepo.GetByID(ctx, input.UserID); err == nil && u != nil {
					if u.TelegramChatID.Valid && u.TelegramChatID.String != "" {
						order.TelegramChatID = u.TelegramChatID
						userTelegramChatID = u.TelegramChatID.String
					}
				}
			}
		} else {
			s.logger.Warn("user_id not found in users table, setting order.user_id to null", zap.Int64("invalid_user_id", input.UserID))
			order.UserID.Valid = false
		}
	} else if (strings.TrimSpace(input.CustomerName) != "" || strings.TrimSpace(input.CustomerPhone) != "" || strings.TrimSpace(input.CustomerEmail) != "") && s.userRepo != nil {
		// Guest Checkout Flow: Automatically create or link a Guest User (role: guest, is_active: false)
		var existingUser *model.User
		var err error

		if strings.TrimSpace(input.CustomerEmail) != "" {
			existingUser, err = s.userRepo.GetByEmail(ctx, strings.TrimSpace(input.CustomerEmail))
		}
		if (existingUser == nil || err != nil) && strings.TrimSpace(input.CustomerPhone) != "" {
			existingUser, err = s.userRepo.GetByPhone(ctx, strings.TrimSpace(input.CustomerPhone))
		}

		if existingUser != nil && existingUser.ID > 0 {
			order.UserID.Int64 = existingUser.ID
			order.UserID.Valid = true
			if existingUser.TelegramChatID.Valid && existingUser.TelegramChatID.String != "" {
				order.TelegramChatID = existingUser.TelegramChatID
				userTelegramChatID = existingUser.TelegramChatID.String
			}
		} else {
			email := strings.TrimSpace(input.CustomerEmail)
			if email == "" {
				phoneClean := strings.ReplaceAll(strings.TrimSpace(input.CustomerPhone), " ", "")
				if phoneClean != "" {
					email = fmt.Sprintf("guest_%s@autoparts.local", phoneClean)
				} else {
					email = fmt.Sprintf("guest_%d@autoparts.local", time.Now().UnixNano())
				}
			}
			name := strings.TrimSpace(input.CustomerName)
			if name == "" {
				name = "Guest Customer"
			}

			guestUser := &model.User{
				Email:    email,
				Name:     name,
				Role:     model.RoleGuest,
				IsActive: false,
			}
			if strings.TrimSpace(input.CustomerPhone) != "" {
				guestUser.Phone.String = strings.TrimSpace(input.CustomerPhone)
				guestUser.Phone.Valid = true
			}

			if errCreate := s.userRepo.Create(ctx, guestUser); errCreate == nil && guestUser.ID > 0 {
				order.UserID.Int64 = guestUser.ID
				order.UserID.Valid = true
				s.logger.Info("created guest user for checkout", zap.Int64("user_id", guestUser.ID), zap.String("email", email))

				if strings.TrimSpace(input.Address) != "" && s.customerRepo != nil {
					_ = s.customerRepo.Create(ctx, &model.Customer{
						UserID:       guestUser.ID,
						TypeCustomer: model.CustomerTypeIndividual,
						Address:      sql.NullString{String: strings.TrimSpace(input.Address), Valid: true},
					})
				}
			} else if errCreate != nil {
				s.logger.Error("failed to create guest user", zap.Error(errCreate))
			}
		}
	}

	if order.TelegramChatID.Valid && order.TelegramChatID.String != "" {
		userTelegramChatID = order.TelegramChatID.String
	}

	if paymentMethod != "" {
		order.PaymentMethod.String = paymentMethod
		order.PaymentMethod.Valid = true
	}

	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("gagal membuka transaksi: %w", err)
	}
	defer tx.Rollback()

	// Propagate the authenticated actor into the PostgreSQL audit trigger
	// via SET LOCAL — scoped to this transaction, no pool-leak risk.
	// NOTE: "app.current_user" must be quoted — current_user is a reserved keyword.
	if actor := utils.ActorFromContext(ctx); actor != "" {
		safeActor := strings.ReplaceAll(actor, "'", "''")
		_, _ = tx.ExecContext(ctx, fmt.Sprintf(`SET LOCAL "app.current_user" = '%s'`, safeActor))
	}

	if err := s.orderRepo.CreateTx(ctx, tx, order); err != nil {
		return nil, fmt.Errorf("gagal membuat order: %w", err)
	}

	for _, item := range orderDetails {
		if orderStatus == model.OrderStatusPaid {
			if err := s.productRepo.DeductStock(ctx, tx, item.ProductID, item.Quantity); err != nil {
				return nil, fmt.Errorf("gagal memotong stok produk %d: %w", item.ProductID, err)
			}
		} else {
			if err := s.productRepo.ReserveStockTx(ctx, tx, item.ProductID, item.Quantity); err != nil {
				return nil, fmt.Errorf("gagal reservasi stok produk %d: %w", item.ProductID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("gagal commit transaksi order: %w", err)
	}

	s.logger.Info("order created via POS/REST",
		zap.String("order_number", order.OrderNumber),
		zap.String("status", string(order.Status)),
		zap.Float64("total", order.TotalPrice),
	)

	// Send notification via Telegram if messageSender is configured and target chat ID exists
	if s.msgSender != nil && userTelegramChatID != "" {
		msg := fmt.Sprintf("🛒 *Pesanan Baru Dibuat!*\n\nNomor Pesanan: *%s*\nTotal: *Rp %.0f*\nMetode Pembayaran: *%s*",
			order.OrderNumber, order.TotalPrice, paymentMethod)

		if paymentMethod == "midtrans" && s.midtransSvc != nil {
			snapResp, err := s.midtransSvc.CreateSnapTransaction(ctx, order.ID)
			if err == nil && snapResp != nil && snapResp.RedirectURL != "" {
				msg += fmt.Sprintf("\n\n💳 *Link Pembayaran Online Midtrans:*\n%s", snapResp.RedirectURL)
			}
		} else {
			msg += "\n\n📌 *Instruksi:* Silakan tunjukkan Nomor Pesanan ini kepada kasir kami untuk melakukan pembayaran."
		}

		if err := s.msgSender.SendText(ctx, model.PlatformTelegram, userTelegramChatID, msg); err != nil {
			s.logger.Error("gagal mengirim notifikasi pesanan ke Telegram", zap.String("chat_id", userTelegramChatID), zap.Error(err))
		}
	}

	// Send broadcast notification to Telegram Order Channel (Bot 2)
	if s.notifierSvc != nil {
		s.notifierSvc.SendOrderNotification(ctx, order)
	}

	return order, nil
}

func (s *OrderService) GetFilteredOrders(ctx context.Context, filter repository.OrderFilter) ([]model.Order, int64, error) {
	return s.orderRepo.FindFiltered(ctx, filter)
}

// GetOrdersByUserID lists all orders for a specific user ID.
func (s *OrderService) GetOrdersByUserID(ctx context.Context, userID int64) ([]model.Order, error) {
	return s.orderRepo.ListByUserID(ctx, userID)
}

// GetOrdersByPhone lists recent orders for a user.
func (s *OrderService) GetOrdersByPhone(ctx context.Context, phone string) ([]model.Order, error) {
	return s.orderRepo.ListByPhone(ctx, phone)
}

// DeleteOrder deletes an order permanently ONLY if its status is cancelled.
func (s *OrderService) DeleteOrder(ctx context.Context, id int64) error {
	order, err := s.orderRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("order tidak ditemukan: %w", err)
	}

	if order.Status != model.OrderStatusCancelled {
		return fmt.Errorf("hanya pesanan berstatus dibatalkan (cancelled) yang dapat dihapus. Status pesanan saat ini: %s", order.Status)
	}

	if err := s.orderRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("gagal menghapus pesanan: %w", err)
	}

	s.logger.Info("order deleted permanently", zap.String("order_number", order.OrderNumber), zap.Int64("order_id", id))
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// generateOrderNumber creates a unique order number like "APT-20240115-A3F9".
func generateOrderNumber() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	suffix := make([]byte, 4)
	for i := range suffix {
		suffix[i] = charset[rand.Intn(len(charset))]
	}
	return fmt.Sprintf("APT-%s-%s", time.Now().Format("20060102"), string(suffix))
}