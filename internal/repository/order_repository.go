package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/model"
)

// OrderRepository handles all order database operations.
type OrderRepository struct {
	db *sqlx.DB
}

func NewOrderRepository(db *sqlx.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// Create inserts a new order in pending state.
// Uses RETURNING id instead of LastInsertId() (PostgreSQL does not support the latter).
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) error {
	const sql = `
		INSERT INTO orders
		  (order_number, phone_number, product_id, product_name, quantity,
		   unit_price, total_price, status, notes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`
	return r.db.QueryRowContext(ctx, sql,
		order.OrderNumber, order.PhoneNumber, order.ProductID, order.ProductName,
		order.Quantity, order.UnitPrice, order.TotalPrice, order.Status,
		order.Notes, order.ExpiresAt,
	).Scan(&order.ID)
}

// GetByID retrieves an order by its ID.
func (r *OrderRepository) GetByID(ctx context.Context, id int64) (*model.Order, error) {
	const sql = `
		SELECT id, order_number, phone_number, product_id, product_name, quantity,
		       unit_price, total_price, status, notes, expires_at, created_at, updated_at
		FROM orders WHERE id = $1
	`
	var o model.Order
	if err := r.db.GetContext(ctx, &o, sql, id); err != nil {
		return nil, err
	}
	return &o, nil
}

// GetByOrderNumber retrieves an order by its human-readable order number.
func (r *OrderRepository) GetByOrderNumber(ctx context.Context, orderNum string) (*model.Order, error) {
	const sql = `
		SELECT id, order_number, phone_number, product_id, product_name, quantity,
		       unit_price, total_price, status, notes, expires_at, created_at, updated_at
		FROM orders WHERE order_number = $1
	`
	var o model.Order
	if err := r.db.GetContext(ctx, &o, sql, orderNum); err != nil {
		return nil, err
	}
	return &o, nil
}

// UpdateStatus atomically changes an order's status.
func (r *OrderRepository) UpdateStatus(ctx context.Context, orderID int64, status model.OrderStatus) error {
	const sql = `UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, sql, status, orderID)
	return err
}

// UpdateStatusTx changes status inside a transaction (used with stock deduction).
func (r *OrderRepository) UpdateStatusTx(ctx context.Context, tx *sqlx.Tx, orderID int64, status model.OrderStatus) error {
	const sql = `UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := tx.ExecContext(ctx, sql, status, orderID)
	return err
}

// ListByPhone returns all orders for a given phone number, newest first.
func (r *OrderRepository) ListByPhone(ctx context.Context, phone string) ([]model.Order, error) {
	const sql = `
		SELECT id, order_number, phone_number, product_id, product_name, quantity,
		       unit_price, total_price, status, notes, expires_at, created_at, updated_at
		FROM orders
		WHERE phone_number = $1
		ORDER BY created_at DESC
		LIMIT 10
	`
	var orders []model.Order
	err := r.db.SelectContext(ctx, &orders, sql, phone)
	return orders, err
}

// ExpireReservations cancels orders whose reservation window has passed.
// Intended to be run by a background cron/ticker.
func (r *OrderRepository) ExpireReservations(ctx context.Context) ([]model.Order, error) {
	// Fetch candidates first so we can release their stock
	const selectSQL = `
		SELECT id, product_id, quantity
		FROM orders
		WHERE status = 'reserved'
		  AND expires_at IS NOT NULL
		  AND expires_at < $1
	`
	var expired []model.Order
	if err := r.db.SelectContext(ctx, &expired, selectSQL, time.Now()); err != nil {
		return nil, err
	}
	if len(expired) == 0 {
		return nil, nil
	}

	// Bulk-cancel
	const updateSQL = `
		UPDATE orders SET status = 'cancelled', updated_at = NOW()
		WHERE status = 'reserved' AND expires_at < $1
	`
	_, err := r.db.ExecContext(ctx, updateSQL, time.Now())
	return expired, err
}

// BeginTx starts a new database transaction.
func (r *OrderRepository) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return r.db.BeginTxx(ctx, nil)
}