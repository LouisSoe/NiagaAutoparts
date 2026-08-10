package model

import "time"

// SalesReportFilter holds filtering criteria for sales reports.
type SalesReportFilter struct {
	StartDate string `form:"start_date" json:"start_date"`
	EndDate   string `form:"end_date" json:"end_date"`
	Status    string `form:"status" json:"status"`
	Page      int    `form:"page" json:"page"`
	Limit     int    `form:"limit" json:"limit"`
}

// SalesReportRow represents a single transaction line in the sales report.
type SalesReportRow struct {
	ID            int64     `db:"id" json:"id"`
	OrderNumber   string    `db:"order_number" json:"order_number"`
	CustomerName  string    `db:"customer_name" json:"customer_name"`
	TotalPrice    float64   `db:"total_price" json:"total_price"`
	Status        string    `db:"status" json:"status"`
	PaymentMethod string    `db:"payment_method" json:"payment_method"`
	Source        string    `db:"source" json:"source"`
	ItemCount     int       `db:"item_count" json:"item_count"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// SalesReportSummary provides totals for a given filter.
type SalesReportSummary struct {
	TotalOrders  int64   `db:"total_orders" json:"total_orders"`
	TotalRevenue float64 `db:"total_revenue" json:"total_revenue"`
	TotalItems   int64   `db:"total_items" json:"total_items"`
}

// SalesReportData wraps summary statistics and transaction items.
type SalesReportData struct {
	Summary SalesReportSummary `json:"summary"`
	Orders  []SalesReportRow   `json:"orders"`
}

// StockReportRow represents an item row for the inventory stock report.
type StockReportRow struct {
	ID           int64   `db:"id" json:"id"`
	SKU          string  `db:"sku" json:"sku"`
	Name         string  `db:"name" json:"name"`
	Category     string  `db:"category" json:"category"`
	Location     string  `db:"location" json:"location"`
	Stock        int     `db:"stock" json:"stock"`
	Reserved     int     `db:"reserved" json:"reserved"`
	Available    int     `db:"available" json:"available"`
	MinimumStock int     `db:"minimum_stock" json:"minimum_stock"`
	Unit         string  `db:"unit" json:"unit"`
	BuyPrice     float64 `db:"purchase_price" json:"purchase_price"`
	SellPrice    float64 `db:"selling_price" json:"selling_price"`
	StatusLabel  string  `db:"status_label" json:"status_label"`
}
