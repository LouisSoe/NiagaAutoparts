package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/model"
)

type ReportRepository struct {
	db *sqlx.DB
}

func NewReportRepository(db *sqlx.DB) *ReportRepository {
	return &ReportRepository{db: db}
}

func (r *ReportRepository) GetSalesReport(ctx context.Context, filter model.SalesReportFilter) (*model.SalesReportData, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	argID := 1

	if filter.StartDate != "" {
		where = append(where, fmt.Sprintf("o.created_at >= $%d::timestamp", argID))
		args = append(args, filter.StartDate+" 00:00:00")
		argID++
	}
	if filter.EndDate != "" {
		where = append(where, fmt.Sprintf("o.created_at <= $%d::timestamp", argID))
		args = append(args, filter.EndDate+" 23:59:59")
		argID++
	}
	if filter.Status != "" && filter.Status != "all" {
		where = append(where, fmt.Sprintf("o.status = $%d", argID))
		args = append(args, filter.Status)
		argID++
	}

	whereClause := strings.Join(where, " AND ")

	summaryQuery := fmt.Sprintf(`
		SELECT 
			COUNT(o.id) AS total_orders,
			COALESCE(SUM(CASE WHEN o.status = 'paid' THEN o.total_price ELSE 0 END), 0) AS total_revenue,
			COALESCE(SUM(od.total_items), 0) AS total_items
		FROM orders o
		LEFT JOIN (
			SELECT order_id, SUM(quantity) AS total_items FROM order_details GROUP BY order_id
		) od ON od.order_id = o.id
		WHERE %s
	`, whereClause)

	var summary model.SalesReportSummary
	if err := r.db.GetContext(ctx, &summary, summaryQuery, args...); err != nil {
		return nil, 0, err
	}

	orderQuery := fmt.Sprintf(`
		SELECT 
			o.id,
			o.order_number,
			COALESCE(u.name, 'Pelanggan Umum') AS customer_name,
			o.total_price,
			o.status,
			COALESCE(o.payment_method, '-') AS payment_method,
			o.source,
			COALESCE(od.total_items, 0) AS item_count,
			o.created_at
		FROM orders o
		LEFT JOIN users u ON u.id = o.user_id
		LEFT JOIN (
			SELECT order_id, SUM(quantity) AS total_items FROM order_details GROUP BY order_id
		) od ON od.order_id = o.id
		WHERE %s
		ORDER BY o.created_at DESC
	`, whereClause)

	if filter.Limit > 0 {
		offset := (filter.Page - 1) * filter.Limit
		if offset < 0 {
			offset = 0
		}
		orderQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argID, argID+1)
		args = append(args, filter.Limit, offset)
	}

	var rows []model.SalesReportRow
	if err := r.db.SelectContext(ctx, &rows, orderQuery, args...); err != nil {
		return nil, 0, err
	}

	if rows == nil {
		rows = []model.SalesReportRow{}
	}

	return &model.SalesReportData{
		Summary: summary,
		Orders:  rows,
	}, summary.TotalOrders, nil
}

func (r *ReportRepository) GetStockReport(ctx context.Context, categoryID int64, lowStockOnly bool) ([]model.StockReportRow, error) {
	where := []string{"p.is_active = true"}
	args := []interface{}{}
	argID := 1

	if categoryID > 0 {
		where = append(where, fmt.Sprintf("p.category_id = $%d", argID))
		args = append(args, categoryID)
		argID++
	}

	if lowStockOnly {
		where = append(where, " (p.stock - p.reserved) <= COALESCE(NULLIF(p.minimum_stock, 0), 5)")
	}

	whereClause := strings.Join(where, " AND ")

	query := fmt.Sprintf(`
		SELECT 
			p.id,
			p.sku,
			p.name,
			COALESCE(c.name, '') AS category,
			p.location,
			p.stock,
			p.reserved,
			(p.stock - p.reserved) AS available,
			COALESCE(NULLIF(p.minimum_stock, 0), 5) AS minimum_stock,
			p.unit,
			p.purchase_price,
			p.selling_price,
			CASE 
				WHEN (p.stock - p.reserved) <= 0 THEN 'Habis'
				WHEN (p.stock - p.reserved) <= COALESCE(NULLIF(p.minimum_stock, 0), 5) THEN 'Menipis'
				ELSE 'Tersedia'
			END AS status_label
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE %s
		ORDER BY (p.stock - p.reserved) ASC, p.name ASC
	`, whereClause)

	var rows []model.StockReportRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	if rows == nil {
		rows = []model.StockReportRow{}
	}

	return rows, nil
}
