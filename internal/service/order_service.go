package service

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"go.uber.org/zap"
)

const (
	// ReservationWindow is how long stock is held before auto-cancellation.
	ReservationWindow = 15 * time.Minute
)

// OrderService handles the order lifecycle: create, reserve, confirm, cancel.
type OrderService struct {
	orderRepo   *repository.OrderRepository
	productRepo *repository.ProductRepository
	logger      *zap.Logger
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
	if platform == model.PlatformTelegram {
		sourceStr = "telegram"
		teleChatID = sql.NullString{String: sender, Valid: true}
	}

	expiry := time.Now().Add(ReservationWindow)
	subtotal := product.SellingPrice * float64(qty)
	order := &model.Order{
		OrderNumber:    generateOrderNumber(),
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
	AmountPaid    float64                `json:"amount_paid"`
	ChangeAmount  float64                `json:"change_amount"`
	Source        string                 `json:"source"`
	PaymentMethod string                 `json:"payment_method"`
	Status        string                 `json:"status"`
	Notes         string                 `json:"notes"`
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
		expiry := time.Now().Add(ReservationWindow)
		order.ExpiresAt = &expiry
	}

	if input.UserID > 0 {
		_, uID := s.orderRepo.ResolveCustomerAndUserIDs(ctx, input.UserID)
		if uID != nil {
			order.UserID.Int64 = *uID
			order.UserID.Valid = true
		} else {
			s.logger.Warn("user_id not found in database, setting order.user_id to null", zap.Int64("invalid_user_id", input.UserID))
			order.UserID.Valid = false
		}
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

	return order, nil
}

func (s *OrderService) GetFilteredOrders(ctx context.Context, filter repository.OrderFilter) ([]model.Order, int64, error) {
	return s.orderRepo.FindFiltered(ctx, filter)
}

// GetOrdersByPhone lists recent orders for a user.
func (s *OrderService) GetOrdersByPhone(ctx context.Context, phone string) ([]model.Order, error) {
	return s.orderRepo.ListByPhone(ctx, phone)
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