package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/model"
)

type DashboardRepository struct {
	db *sqlx.DB
}

func NewDashboardRepository(db *sqlx.DB) *DashboardRepository {
	return &DashboardRepository{db: db}
}

func (r *DashboardRepository) GetSummary(ctx context.Context) (*model.DashboardSummary, error) {
	const query = `
		SELECT
			(SELECT COUNT(*) FROM products WHERE is_active = true) AS total_products,
			(SELECT COUNT(*) FROM categories) AS total_categories,
			(SELECT COUNT(*) FROM customers) AS total_customers,
			(SELECT COUNT(*) FROM orders) AS total_orders,
			(SELECT COUNT(*) FROM orders WHERE status IN ('pending', 'reserved')) AS pending_orders,
			(SELECT COUNT(*) FROM orders WHERE status = 'paid') AS paid_orders,
			(SELECT COUNT(*) FROM orders WHERE status = 'cancelled') AS cancelled_orders,
			(SELECT COALESCE(SUM(total_price), 0) FROM orders WHERE status = 'paid') AS total_revenue,
			(SELECT COUNT(*) FROM products WHERE is_active = true AND (stock - reserved) <= COALESCE(NULLIF(minimum_stock, 0), 5) AND (stock - reserved) > 0) AS low_stock_products,
			(SELECT COUNT(*) FROM products WHERE is_active = true AND (stock - reserved) <= 0) AS out_of_stock_products
	`

	var summary model.DashboardSummary
	err := r.db.GetContext(ctx, &summary, query)
	if err != nil {
		return nil, err
	}

	return &summary, nil
}

func (r *DashboardRepository) GetRecentOrders(ctx context.Context, limit int) ([]model.RecentOrderSummary, error) {
	if limit <= 0 {
		limit = 5
	}

	const query = `
		SELECT 
			o.id, 
			o.order_number, 
			COALESCE(u.name, 'Pelanggan Umum') AS customer_name, 
			o.total_price, 
			o.status, 
			o.created_at
		FROM orders o
		LEFT JOIN users u ON u.id = o.user_id
		ORDER BY o.created_at DESC
		LIMIT $1
	`

	var orders []model.RecentOrderSummary
	err := r.db.SelectContext(ctx, &orders, query, limit)
	if err != nil {
		return nil, err
	}

	if orders == nil {
		orders = []model.RecentOrderSummary{}
	}

	return orders, nil
}

func (r *DashboardRepository) GetLowStockItems(ctx context.Context, limit int) ([]model.LowStockItem, error) {
	if limit <= 0 {
		limit = 10
	}

	const query = `
		SELECT 
			p.id, 
			p.sku, 
			p.name, 
			COALESCE(c.name, '') AS category, 
			p.stock, 
			p.reserved, 
			(p.stock - p.reserved) AS available, 
			COALESCE(NULLIF(p.minimum_stock, 0), 5) AS minimum_stock, 
			p.selling_price AS price
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE p.is_active = true AND (p.stock - p.reserved) <= COALESCE(NULLIF(p.minimum_stock, 0), 5)
		ORDER BY (p.stock - p.reserved) ASC
		LIMIT $1
	`

	var items []model.LowStockItem
	err := r.db.SelectContext(ctx, &items, query, limit)
	if err != nil {
		return nil, err
	}

	if items == nil {
		items = []model.LowStockItem{}
	}

	return items, nil
}
