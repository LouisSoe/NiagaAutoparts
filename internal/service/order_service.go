package service

import (
	"context"
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

// CreateReservation creates an order in "reserved" status and holds the stock.
// This is the first step of the 2-phase order flow.
func (s *OrderService) CreateReservation(ctx context.Context, phone string, product *model.Product, qty int) (*model.Order, error) {
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

	expiry := time.Now().Add(ReservationWindow)
	order := &model.Order{
		OrderNumber: generateOrderNumber(),
		PhoneNumber: phone,
		ProductID:   product.ID,
		ProductName: product.Name,
		Quantity:    qty,
		UnitPrice:   product.Price,
		TotalPrice:  product.Price * float64(qty),
		Status:      model.OrderStatusReserved,
		ExpiresAt:   &expiry,
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		// Rollback the reservation on failure
		_ = s.productRepo.ReleaseReservation(ctx, product.ID, qty)
		return nil, fmt.Errorf("gagal membuat order: %w", err)
	}

	s.logger.Info("order reserved",
		zap.String("order_number", order.OrderNumber),
		zap.String("phone", phone),
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

	if err = s.productRepo.DeductStock(ctx, tx, order.ProductID, order.Quantity); err != nil {
		return nil, fmt.Errorf("deduct stock: %w", err)
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
	// Release reserved stock
	if order.Status == model.OrderStatusReserved {
		_ = s.productRepo.ReleaseReservation(ctx, order.ProductID, order.Quantity)
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
		_ = s.productRepo.ReleaseReservation(ctx, o.ProductID, o.Quantity)
		s.logger.Info("reservation expired",
			zap.Int64("order_id", o.ID),
			zap.Int64("product_id", o.ProductID),
		)
	}
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