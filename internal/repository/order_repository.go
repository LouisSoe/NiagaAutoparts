package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

// CreateTx inserts a new order header along with its details within an existing transaction.
func (r *OrderRepository) CreateTx(ctx context.Context, tx *sqlx.Tx, order *model.Order) error {
	if order.Source == "" {
		order.Source = "wa"
	}

	const headerSQL = `
		INSERT INTO orders
		  (order_number, user_id, total_price, amount_paid, change_amount,
		   status, source, payment_method, telegram_chat_id, notes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at, updated_at
	`
	err := tx.QueryRowContext(ctx, headerSQL,
		order.OrderNumber, order.UserID, order.TotalPrice,
		order.AmountPaid, order.ChangeAmount, order.Status, order.Source,
		order.PaymentMethod, order.TelegramChatID, order.Notes, order.ExpiresAt,
	).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return err
	}

	const itemSQL = `
		INSERT INTO order_details
		  (order_id, product_id, quantity, unit_price, subtotal)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`
	for i := range order.Items {
		item := &order.Items[i]
		item.OrderID = order.ID
		if item.Subtotal == 0 {
			item.Subtotal = float64(item.Quantity) * item.UnitPrice
		}
		err = tx.QueryRowContext(ctx, itemSQL,
			item.OrderID, item.ProductID, item.Quantity,
			item.UnitPrice, item.Subtotal,
		).Scan(&item.ID, &item.CreatedAt)
		if err != nil {
			return err
		}
	}

	return nil
}

// Create inserts a new order header along with its order details in a standalone transaction.
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := r.CreateTx(ctx, tx, order); err != nil {
		return err
	}

	return tx.Commit()
}

// populateOrderItems retrieves details for a single order.
func (r *OrderRepository) populateOrderItems(ctx context.Context, order *model.Order) error {
	const sql = `
		SELECT d.id, d.order_id, d.product_id, COALESCE(p.name, '') AS product_name, d.quantity, d.unit_price, d.subtotal, d.created_at
		FROM order_details d
		LEFT JOIN products p ON p.id = d.product_id
		WHERE d.order_id = $1
		ORDER BY d.id ASC
	`
	var items []model.OrderDetail
	if err := r.db.SelectContext(ctx, &items, sql, order.ID); err != nil {
		return err
	}
	order.Items = items
	return nil
}

// GetByID retrieves an order by its ID with items.
func (r *OrderRepository) GetByID(ctx context.Context, id int64) (*model.Order, error) {
	const sql = `
		SELECT id, order_number, user_id, total_price, amount_paid, change_amount,
		       status, source, payment_method, telegram_chat_id, notes, expires_at, created_at, updated_at
		FROM orders WHERE id = $1
	`
	var o model.Order
	if err := r.db.GetContext(ctx, &o, sql, id); err != nil {
		return nil, err
	}
	if err := r.populateOrderItems(ctx, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// GetByOrderNumber retrieves an order by its human-readable order number with items.
func (r *OrderRepository) GetByOrderNumber(ctx context.Context, orderNum string) (*model.Order, error) {
	const sql = `
		SELECT id, order_number, user_id, total_price, amount_paid, change_amount,
		       status, source, payment_method, telegram_chat_id, notes, expires_at, created_at, updated_at
		FROM orders WHERE order_number = $1
	`
	var o model.Order
	if err := r.db.GetContext(ctx, &o, sql, orderNum); err != nil {
		return nil, err
	}
	if err := r.populateOrderItems(ctx, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// LinkTelegramChatID links a Telegram chat_id to an order by its order_number.
func (r *OrderRepository) LinkTelegramChatID(ctx context.Context, orderNum, chatID string) error {
	const sql = `UPDATE orders SET telegram_chat_id = $1, updated_at = NOW() WHERE order_number = $2`
	_, err := r.db.ExecContext(ctx, sql, chatID, orderNum)
	return err
}

// UpdatePaymentMethod changes the payment method for an order.
func (r *OrderRepository) UpdatePaymentMethod(ctx context.Context, orderID int64, paymentMethod string) error {
	const sql = `UPDATE orders SET payment_method = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, sql, paymentMethod, orderID)
	return err
}

// UpdateTotalPrice updates the total_price for an order (e.g. after adding shipping cost).
func (r *OrderRepository) UpdateTotalPrice(ctx context.Context, orderID int64, totalPrice float64) error {
	const sql = `UPDATE orders SET total_price = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, sql, totalPrice, orderID)
	return err
}

// UpdateExpiresAt updates the expiration timestamp for an order.
func (r *OrderRepository) UpdateExpiresAt(ctx context.Context, orderID int64, expiresAt time.Time) error {
	const sql = `UPDATE orders SET expires_at = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, sql, expiresAt, orderID)
	return err
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

const orderSelectColumns = `o.id, o.order_number, o.user_id, o.total_price, o.amount_paid, o.change_amount, o.status, o.source, o.payment_method, o.telegram_chat_id, o.notes, o.expires_at, o.created_at, o.updated_at`

// GetHistoryLastDays returns orders for a specific user/sender created in the last N days.
func (r *OrderRepository) GetHistoryLastDays(ctx context.Context, sender string, days int) ([]model.Order, error) {
	const sql = `
		SELECT ` + orderSelectColumns + `
		FROM orders o
		WHERE (o.telegram_chat_id = $1 OR o.notes = $1)
		  AND o.created_at >= NOW() - ($2 || ' days')::INTERVAL
		ORDER BY o.created_at DESC
		LIMIT 20
	`
	var orders []model.Order
	err := r.db.SelectContext(ctx, &orders, sql, sender, days)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		_ = r.populateOrderItems(ctx, &orders[i])
	}
	return orders, nil
}

// GetHistoryMonthly returns all orders for a specific user/sender created in a given year and month.
func (r *OrderRepository) GetHistoryMonthly(ctx context.Context, sender string, year, month int) ([]model.Order, error) {
	const sql = `
		SELECT ` + orderSelectColumns + `
		FROM orders o
		WHERE (o.telegram_chat_id = $1 OR o.notes = $1)
		  AND EXTRACT(YEAR FROM o.created_at) = $2
		  AND EXTRACT(MONTH FROM o.created_at) = $3
		ORDER BY o.created_at DESC
	`
	var orders []model.Order
	err := r.db.SelectContext(ctx, &orders, sql, sender, year, month)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		_ = r.populateOrderItems(ctx, &orders[i])
	}
	return orders, nil
}

// ListByPhone returns all orders for a given customer phone, telegram chat id, or sender notes, newest first with items.
func (r *OrderRepository) ListByPhone(ctx context.Context, phone string) ([]model.Order, error) {
	const sql = `
		SELECT o.id, o.order_number, o.user_id, o.total_price, o.amount_paid, o.change_amount,
		       o.status, o.source, o.payment_method, o.notes, o.expires_at, o.created_at, o.updated_at
		FROM orders o
		LEFT JOIN users u ON u.id = o.user_id
		WHERE u.phone = $1 OR o.telegram_chat_id = $1 OR o.notes = $1
		ORDER BY o.created_at DESC
		LIMIT 10
	`
	var orders []model.Order
	err := r.db.SelectContext(ctx, &orders, sql, phone)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		_ = r.populateOrderItems(ctx, &orders[i])
	}
	return orders, nil
}

// ExpireReservations cancels orders whose reservation window has passed.
func (r *OrderRepository) ExpireReservations(ctx context.Context) ([]model.Order, error) {
	const selectSQL = `
		SELECT o.id, o.order_number, o.user_id, o.total_price, o.amount_paid, o.change_amount,
		       o.status, o.source, o.payment_method, o.notes, o.expires_at, o.created_at, o.updated_at
		FROM orders o
		WHERE o.status = 'reserved'
		  AND o.expires_at IS NOT NULL
		  AND o.expires_at < $1
	`
	var expired []model.Order
	if err := r.db.SelectContext(ctx, &expired, selectSQL, time.Now()); err != nil {
		return nil, err
	}
	if len(expired) == 0 {
		return nil, nil
	}

	for i := range expired {
		_ = r.populateOrderItems(ctx, &expired[i])
	}

	const updateSQL = `
		UPDATE orders SET status = 'cancelled', updated_at = NOW()
		WHERE status = 'reserved' AND expires_at < $1
	`
	_, err := r.db.ExecContext(ctx, updateSQL, time.Now())
	return expired, err
}

type OrderFilter struct {
	Q         string
	Status    string
	UserID    int64
	StartDate string
	EndDate   string
	Date      string
	Page      int
	Limit     int
}

func (r *OrderRepository) FindFiltered(ctx context.Context, filter OrderFilter) ([]model.Order, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if filter.Q != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(o.order_number ILIKE $%d OR COALESCE(o.notes, '') ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+filter.Q+"%")
		argIdx++
	}

	if filter.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("LOWER(o.status) = $%d", argIdx))
		args = append(args, strings.ToLower(filter.Status))
		argIdx++
	}

	if filter.UserID > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("o.user_id = $%d", argIdx))
		args = append(args, filter.UserID)
		argIdx++
	}

	if filter.StartDate != "" {
		sd := filter.StartDate
		if len(sd) == 10 {
			sd += " 00:00:00"
		}
		whereClauses = append(whereClauses, fmt.Sprintf("o.created_at >= $%d", argIdx))
		args = append(args, sd)
		argIdx++
	}

	if filter.EndDate != "" {
		ed := filter.EndDate
		if len(ed) == 10 {
			ed += " 23:59:59"
		}
		whereClauses = append(whereClauses, fmt.Sprintf("o.created_at <= $%d", argIdx))
		args = append(args, ed)
		argIdx++
	}

	if filter.Date != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("DATE(o.created_at) = $%d", argIdx))
		args = append(args, filter.Date)
		argIdx++
	}

	whereStmt := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM orders o WHERE %s", whereStmt)
	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("FindFiltered count: %w", err)
	}

	selectQuery := fmt.Sprintf(`
		SELECT o.id, o.order_number, o.user_id, o.total_price, o.amount_paid, o.change_amount,
		       o.status, o.source, o.payment_method, o.notes, o.expires_at, o.created_at, o.updated_at
		FROM orders o
		WHERE %s
		ORDER BY o.created_at DESC`, whereStmt)

	if filter.Limit > 0 {
		if filter.Page <= 0 {
			filter.Page = 1
		}
		offset := (filter.Page - 1) * filter.Limit
		selectQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
		args = append(args, filter.Limit, offset)
	}

	var orders []model.Order
	if err := r.db.SelectContext(ctx, &orders, selectQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("FindFiltered select: %w", err)
	}

	for i := range orders {
		_ = r.populateOrderItems(ctx, &orders[i])
	}

	return orders, total, nil
}

// ListByUserID returns all orders belonging to a given user ID, newest first.
func (r *OrderRepository) ListByUserID(ctx context.Context, userID int64) ([]model.Order, error) {
	filter := OrderFilter{
		UserID: userID,
	}
	orders, _, err := r.FindFiltered(ctx, filter)
	return orders, err
}

// UserExists checks if a user ID exists in the users table.
func (r *OrderRepository) UserExists(ctx context.Context, userID int64) bool {
	if userID <= 0 {
		return false
	}
	var exists bool
	_ = r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists)
	return exists
}

// ResolveCustomerAndUserIDs validates input ID against customers and users tables to prevent foreign key errors.
func (r *OrderRepository) ResolveCustomerAndUserIDs(ctx context.Context, inputID int64) (*int64, *int64) {
	if inputID <= 0 {
		return nil, nil
	}

	var cID int64
	var uID sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT id, user_id FROM customers WHERE id = $1`, inputID).Scan(&cID, &uID)
	if err == nil {
		var uPtr *int64
		if uID.Valid && uID.Int64 > 0 {
			uPtr = &uID.Int64
		}
		return &cID, uPtr
	}

	var userExistID int64
	err = r.db.QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1`, inputID).Scan(&userExistID)
	if err == nil {
		var custExistID int64
		var cPtr *int64
		if errCust := r.db.QueryRowContext(ctx, `SELECT id FROM customers WHERE user_id = $1`, userExistID).Scan(&custExistID); errCust == nil {
			cPtr = &custExistID
		}
		return cPtr, &userExistID
	}

	return nil, nil
}

// BeginTx starts a new database transaction.
func (r *OrderRepository) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return r.db.BeginTxx(ctx, nil)
}

// Delete permanently removes an order and its detail items within a transaction.
func (r *OrderRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for delete order: %w", err)
	}
	defer tx.Rollback()

	setAuditActor(ctx, tx)

	if _, err := tx.ExecContext(ctx, `DELETE FROM order_details WHERE order_id = $1`, id); err != nil {
		return fmt.Errorf("delete order_details: %w", err)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM orders WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete orders: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("order with id %d not found", id)
	}

	return tx.Commit()
}