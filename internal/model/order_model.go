package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// OrderStatus represents the lifecycle of an order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"   // Waiting for user confirmation
	OrderStatusReserved  OrderStatus = "reserved"  // Stock reserved, awaiting payment
	OrderStatusPaid      OrderStatus = "paid"       // Payment confirmed, stock deducted
	OrderStatusCancelled OrderStatus = "cancelled"  // Cancelled, stock released
)

// OrderDetail represents an item within an order.
type OrderDetail struct {
	ID          int64     `db:"id" json:"id"`
	OrderID     int64     `db:"order_id" json:"order_id"`
	ProductID   int64     `db:"product_id" json:"product_id"`
	ProductName string    `db:"product_name" json:"product_name,omitempty"`
	Quantity    int       `db:"quantity" json:"quantity"`
	UnitPrice   float64   `db:"unit_price" json:"unit_price"` // Price at order time
	Subtotal    float64   `db:"subtotal" json:"subtotal"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// Order represents a customer order (Header).
type Order struct {
	ID            int64          `db:"id" json:"id"`
	OrderNumber   string         `db:"order_number" json:"order_number"`
	UserID        sql.NullInt64  `db:"user_id" json:"user_id"`
	TotalPrice    float64        `db:"total_price" json:"total_price"`
	AmountPaid    float64        `db:"amount_paid" json:"amount_paid"`
	ChangeAmount  float64        `db:"change_amount" json:"change_amount"`
	Status        OrderStatus    `db:"status" json:"status"`
	Source        string         `db:"source" json:"source"`                 // wa, web, pos
	PaymentMethod sql.NullString `db:"payment_method" json:"payment_method"` // cash, qris, transfer
	TelegramChatID sql.NullString `db:"telegram_chat_id" json:"telegram_chat_id"`
	Notes         string         `db:"notes" json:"notes"`
	ExpiresAt     *time.Time     `db:"expires_at" json:"expires_at"` // Reservation expiry
	CreatedAt     time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at" json:"updated_at"`

	Items []OrderDetail `db:"-" json:"items,omitempty"`
}

func (o Order) MarshalJSON() ([]byte, error) {
	type Alias Order
	var userID *int64
	if o.UserID.Valid {
		userID = &o.UserID.Int64
	}
	var paymentMethod *string
	if o.PaymentMethod.Valid {
		paymentMethod = &o.PaymentMethod.String
	}
	return json.Marshal(&struct {
		Alias
		UserID        *int64  `json:"user_id"`
		PaymentMethod *string `json:"payment_method"`
	}{
		Alias:         Alias(o),
		UserID:        userID,
		PaymentMethod: paymentMethod,
	})
}

