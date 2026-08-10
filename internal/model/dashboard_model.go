package model

import "time"

// DashboardSummary aggregates high-level metrics for the dashboard.
type DashboardSummary struct {
	TotalProducts      int64   `json:"total_products"`
	TotalCategories    int64   `json:"total_categories"`
	TotalCustomers     int64   `json:"total_customers"`
	TotalOrders        int64   `json:"total_orders"`
	PendingOrders      int64   `json:"pending_orders"`
	PaidOrders         int64   `json:"paid_orders"`
	CancelledOrders    int64   `json:"cancelled_orders"`
	TotalRevenue       float64 `json:"total_revenue"`
	LowStockProducts   int64   `json:"low_stock_products"`
	OutOfStockProducts int64   `json:"out_of_stock_products"`
}

// RecentOrderSummary provides a quick view of recently placed orders.
type RecentOrderSummary struct {
	ID           int64     `json:"id" db:"id"`
	OrderNumber  string    `json:"order_number" db:"order_number"`
	CustomerName string    `json:"customer_name" db:"customer_name"`
	TotalPrice   float64   `json:"total_price" db:"total_price"`
	Status       string    `json:"status" db:"status"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// LowStockItem represents products that require restock attention.
type LowStockItem struct {
	ID           int64   `json:"id" db:"id"`
	SKU          string  `json:"sku" db:"sku"`
	Name         string  `json:"name" db:"name"`
	Category     string  `json:"category" db:"category"`
	Stock        int     `json:"stock" db:"stock"`
	Reserved     int     `json:"reserved" db:"reserved"`
	Available    int     `json:"available" db:"available"`
	MinimumStock int     `json:"minimum_stock" db:"minimum_stock"`
	SellingPrice float64 `json:"price" db:"price"`
}

// DashboardData is the complete payload for dashboard endpoint.
type DashboardData struct {
	Summary       DashboardSummary     `json:"summary"`
	RecentOrders  []RecentOrderSummary `json:"recent_orders"`
	LowStockItems []LowStockItem       `json:"low_stock_items"`
}
