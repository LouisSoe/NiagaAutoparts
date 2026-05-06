package model
import "time"
// OrderStatus represents the lifecycle of an order.
type OrderStatus string
 
const (
	OrderStatusPending   OrderStatus = "pending"   // Waiting for user confirmation
	OrderStatusReserved  OrderStatus = "reserved"  // Stock reserved, awaiting payment
	OrderStatusPaid      OrderStatus = "paid"       // Payment confirmed, stock deducted
	OrderStatusCancelled OrderStatus = "cancelled"  // Cancelled, stock released
)
 
// Order represents a customer order.
type Order struct {
	ID          int64       `db:"id" json:"id"`
	OrderNumber string      `db:"order_number" json:"order_number"`
	PhoneNumber string      `db:"phone_number" json:"phone_number"` // WhatsApp sender
	ProductID   int64       `db:"product_id" json:"product_id"`
	ProductName string      `db:"product_name" json:"product_name"` // Snapshot at order time
	Quantity    int         `db:"quantity" json:"quantity"`
	UnitPrice   float64     `db:"unit_price" json:"unit_price"`   // Price at order time
	TotalPrice  float64     `db:"total_price" json:"total_price"`
	Status      OrderStatus `db:"status" json:"status"`
	Notes       string      `db:"notes" json:"notes"`
	ExpiresAt   *time.Time  `db:"expires_at" json:"expires_at"` // Reservation expiry
	CreatedAt   time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time   `db:"updated_at" json:"updated_at"`
}
