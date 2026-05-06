package model

import (
	"database/sql"
	"time"
)

type Product struct {
	ID          int64          `db:"id" json:"id"`
	SKU         string         `db:"sku" json:"sku"`
	Name        string         `db:"name" json:"name"`
	Category    string         `db:"category" json:"category"`
	Description sql.NullString `db:"description" json:"description,omitempty"`
	Stock       int            `db:"stock" json:"stock"`
	Reserved    int            `db:"reserved" json:"reserved"` // Stock held by pending orders
	Location    string         `db:"location" json:"location"` // Warehouse rack/row/bin
	Price       float64        `db:"price" json:"price"`       // Selling price
	Unit        string         `db:"unit" json:"unit"`         // pcs, set, ltr, etc.
	ImageURL    sql.NullString `db:"image_url" json:"image_url,omitempty"`
	IsActive    bool           `db:"is_active" json:"is_active"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at" json:"updated_at"`
}
 
// AvailableStock returns stock that is not reserved.
func (p *Product) AvailableStock() int {
	return p.Stock - p.Reserved
}
