package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/model"
)

// DeliveryRepository handles CRUD operations for the deliveries table.
type DeliveryRepository struct {
	db *sqlx.DB
}

func NewDeliveryRepository(db *sqlx.DB) *DeliveryRepository {
	return &DeliveryRepository{db: db}
}

// Create inserts a new delivery record.
func (r *DeliveryRepository) Create(ctx context.Context, d *model.Delivery) error {
	const q = `
		INSERT INTO deliveries (
			order_id, customer_id, schedule_id, courier_id, delivery_date, 
			status, shipping_cost, distance_km, notes, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, 
			$6, $7, $8, $9, NOW(), NOW()
		) RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, q,
		d.OrderID, d.CustomerID, d.ScheduleID, d.CourierID, d.DeliveryDate.Format("2006-01-02"),
		d.Status, d.ShippingCost, d.DistanceKm, d.Notes,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
}

// GetByID retrieves a delivery with full joins on Order, Customer, User, and Schedule.
func (r *DeliveryRepository) GetByID(ctx context.Context, id int64) (*model.Delivery, error) {
	const q = `
		SELECT 
			d.id, d.order_id, d.customer_id, d.schedule_id, d.courier_id, 
			d.delivery_date, d.status, d.shipping_cost, d.distance_km,
			d.suggested_schedule_id, d.suggested_date, d.rejection_reason, d.notes, 
			d.created_at, d.updated_at,
			COALESCE(o.order_number, '') AS order_number,
			COALESCE(u_cust.name, '') AS customer_name,
			COALESCE(u_cust.phone, '') AS customer_phone,
			COALESCE(c.address, '') AS customer_address,
			COALESCE(c.latitude, 0) AS customer_latitude,
			COALESCE(c.longitude, 0) AS customer_longitude,
			COALESCE(ds.slot_name, '') AS slot_name,
			COALESCE(ds_sug.slot_name, '') AS suggested_slot_name,
			COALESCE(u_cour.name, '') AS courier_name,
			COALESCE(o.telegram_chat_id, u_cust.telegram_chat_id, '') AS telegram_chat_id
		FROM deliveries d
		LEFT JOIN orders o ON o.id = d.order_id
		LEFT JOIN customers c ON c.id = d.customer_id
		LEFT JOIN users u_cust ON u_cust.id = c.user_id
		LEFT JOIN delivery_schedules ds ON ds.id = d.schedule_id
		LEFT JOIN delivery_schedules ds_sug ON ds_sug.id = d.suggested_schedule_id
		LEFT JOIN users u_cour ON u_cour.id = d.courier_id
		WHERE d.id = $1`

	var d model.Delivery
	if err := r.db.GetContext(ctx, &d, q, id); err != nil {
		return nil, err
	}
	return &d, nil
}

// GetByOrderID retrieves delivery information by order ID.
func (r *DeliveryRepository) GetByOrderID(ctx context.Context, orderID int64) (*model.Delivery, error) {
	const q = `
		SELECT 
			d.id, d.order_id, d.customer_id, d.schedule_id, d.courier_id, 
			d.delivery_date, d.status, d.shipping_cost, d.distance_km,
			d.suggested_schedule_id, d.suggested_date, d.rejection_reason, d.notes, 
			d.created_at, d.updated_at,
			COALESCE(o.order_number, '') AS order_number,
			COALESCE(u_cust.name, '') AS customer_name,
			COALESCE(u_cust.phone, '') AS customer_phone,
			COALESCE(c.address, '') AS customer_address,
			COALESCE(c.latitude, 0) AS customer_latitude,
			COALESCE(c.longitude, 0) AS customer_longitude,
			COALESCE(ds.slot_name, '') AS slot_name,
			COALESCE(ds_sug.slot_name, '') AS suggested_slot_name,
			COALESCE(u_cour.name, '') AS courier_name,
			COALESCE(o.telegram_chat_id, u_cust.telegram_chat_id, '') AS telegram_chat_id
		FROM deliveries d
		LEFT JOIN orders o ON o.id = d.order_id
		LEFT JOIN customers c ON c.id = d.customer_id
		LEFT JOIN users u_cust ON u_cust.id = c.user_id
		LEFT JOIN delivery_schedules ds ON ds.id = d.schedule_id
		LEFT JOIN delivery_schedules ds_sug ON ds_sug.id = d.suggested_schedule_id
		LEFT JOIN users u_cour ON u_cour.id = d.courier_id
		WHERE d.order_id = $1
		ORDER BY d.id DESC LIMIT 1`

	var d model.Delivery
	if err := r.db.GetContext(ctx, &d, q, orderID); err != nil {
		return nil, err
	}
	return &d, nil
}

// UpdateStatus changes the delivery status.
func (r *DeliveryRepository) UpdateStatus(ctx context.Context, id int64, status model.DeliveryStatus, courierID *int64) error {
	const q = `
		UPDATE deliveries 
		SET status = $1, courier_id = COALESCE($2, courier_id), updated_at = NOW() 
		WHERE id = $3`
	_, err := r.db.ExecContext(ctx, q, status, courierID, id)
	return err
}

// UpdateRescheduleSuggestion updates the delivery record when a courier rejects and proposes a new date/slot.
func (r *DeliveryRepository) UpdateRescheduleSuggestion(ctx context.Context, id int64, suggestedDate time.Time, suggestedScheduleID int64, reason string) error {
	const q = `
		UPDATE deliveries 
		SET status = $1, suggested_date = $2, suggested_schedule_id = $3, rejection_reason = $4, updated_at = NOW() 
		WHERE id = $5`
	_, err := r.db.ExecContext(ctx, q, model.DeliveryStatusReschedule, suggestedDate.Format("2006-01-02"), suggestedScheduleID, reason, id)
	return err
}

// AcceptRescheduledSchedule shifts the proposed schedule into the active schedule fields and confirms the delivery.
func (r *DeliveryRepository) AcceptRescheduledSchedule(ctx context.Context, id int64, newDate time.Time, newScheduleID int64) error {
	const q = `
		UPDATE deliveries 
		SET delivery_date = $1, schedule_id = $2, status = $3, suggested_date = NULL, suggested_schedule_id = NULL, updated_at = NOW() 
		WHERE id = $4`
	_, err := r.db.ExecContext(ctx, q, newDate.Format("2006-01-02"), newScheduleID, model.DeliveryStatusConfirmed, id)
	return err
}

// GetDeliveriesForDate returns all confirmed deliveries for a given date for route optimization.
func (r *DeliveryRepository) GetDeliveriesForDate(ctx context.Context, date time.Time) ([]model.Delivery, error) {
	const q = `
		SELECT 
			d.id, d.order_id, d.customer_id, d.schedule_id, d.courier_id, 
			d.delivery_date, d.status, d.shipping_cost, d.distance_km,
			d.suggested_schedule_id, d.suggested_date, d.rejection_reason, d.notes, 
			d.created_at, d.updated_at,
			COALESCE(o.order_number, '') AS order_number,
			COALESCE(u_cust.name, '') AS customer_name,
			COALESCE(u_cust.phone, '') AS customer_phone,
			COALESCE(c.address, '') AS customer_address,
			COALESCE(c.latitude, 0) AS customer_latitude,
			COALESCE(c.longitude, 0) AS customer_longitude,
			COALESCE(ds.slot_name, '') AS slot_name,
			COALESCE(ds_sug.slot_name, '') AS suggested_slot_name,
			COALESCE(u_cour.name, '') AS courier_name,
			COALESCE(o.telegram_chat_id, u_cust.telegram_chat_id, '') AS telegram_chat_id
		FROM deliveries d
		LEFT JOIN orders o ON o.id = d.order_id
		LEFT JOIN customers c ON c.id = d.customer_id
		LEFT JOIN users u_cust ON u_cust.id = c.user_id
		LEFT JOIN delivery_schedules ds ON ds.id = d.schedule_id
		LEFT JOIN delivery_schedules ds_sug ON ds_sug.id = d.suggested_schedule_id
		LEFT JOIN users u_cour ON u_cour.id = d.courier_id
		WHERE d.delivery_date = $1 AND d.status IN ('confirmed', 'on_delivery')
		ORDER BY d.id ASC`

	var list []model.Delivery
	err := r.db.SelectContext(ctx, &list, q, date.Format("2006-01-02"))
	return list, err
}
