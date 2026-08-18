package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/model"
)

// DeliveryScheduleRepository handles queries for master delivery time slots.
type DeliveryScheduleRepository struct {
	db *sqlx.DB
}

func NewDeliveryScheduleRepository(db *sqlx.DB) *DeliveryScheduleRepository {
	return &DeliveryScheduleRepository{db: db}
}

// GetAvailableSchedulesByDate returns all active schedules for the day of week corresponding to the given date,
// calculating current bookings and capacity status.
func (r *DeliveryScheduleRepository) GetAvailableSchedulesByDate(ctx context.Context, targetDate time.Time) ([]model.DeliverySchedule, error) {
	// Use English lowercase weekday for filtering
	dayNameEng := strings.ToLower(targetDate.Weekday().String())
	dateStr := targetDate.Format("2006-01-02")

	const q = `
		SELECT 
			ds.id, ds.day_of_week, ds.slot_name, 
			ds.start_time::text AS start_time, ds.end_time::text AS end_time, 
			ds.max_capacity, ds.is_active, ds.created_at, ds.updated_at,
			COUNT(d.id) AS booked_count
		FROM delivery_schedules ds
		LEFT JOIN deliveries d ON d.schedule_id = ds.id 
			AND d.delivery_date = $1 
			AND d.status NOT IN ('cancelled')
		WHERE ds.is_active = TRUE AND ds.day_of_week = $2
		GROUP BY ds.id
		ORDER BY ds.start_time ASC`

	rows, err := r.db.QueryxContext(ctx, q, dateStr, dayNameEng)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch available schedules: %w", err)
	}
	defer rows.Close()

	schedules := make([]model.DeliverySchedule, 0)
	for rows.Next() {
		var s model.DeliverySchedule
		if err := rows.StructScan(&s); err != nil {
			return nil, err
		}

		s.AvailableSlots = s.MaxCapacity - s.BookedCount
		if s.AvailableSlots <= 0 {
			s.AvailableSlots = 0
			s.IsFull = true
		} else {
			s.IsFull = false
		}
		schedules = append(schedules, s)
	}

	return schedules, nil
}

// GetAll returns all delivery schedules optionally filtered by day_of_week and is_active.
func (r *DeliveryScheduleRepository) GetAll(ctx context.Context, dayOfWeek string, isActive *bool) ([]model.DeliverySchedule, error) {
	q := `
		SELECT id, day_of_week, slot_name, start_time::text AS start_time, end_time::text AS end_time, max_capacity, is_active, created_at, updated_at
		FROM delivery_schedules
		WHERE 1=1`
	var args []interface{}
	idx := 1

	if dayOfWeek != "" {
		q += fmt.Sprintf(" AND LOWER(day_of_week) = LOWER($%d)", idx)
		args = append(args, dayOfWeek)
		idx++
	}
	if isActive != nil {
		q += fmt.Sprintf(" AND is_active = $%d", idx)
		args = append(args, *isActive)
		idx++
	}

	q += ` ORDER BY 
		CASE LOWER(day_of_week)
			WHEN 'senin' THEN 1
			WHEN 'monday' THEN 1
			WHEN 'selasa' THEN 2
			WHEN 'tuesday' THEN 2
			WHEN 'rabu' THEN 3
			WHEN 'wednesday' THEN 3
			WHEN 'kamis' THEN 4
			WHEN 'thursday' THEN 4
			WHEN 'jumat' THEN 5
			WHEN 'friday' THEN 5
			WHEN 'sabtu' THEN 6
			WHEN 'saturday' THEN 6
			WHEN 'minggu' THEN 7
			WHEN 'sunday' THEN 7
			ELSE 8
		END, start_time ASC`

	schedules := make([]model.DeliverySchedule, 0)
	if err := r.db.SelectContext(ctx, &schedules, q, args...); err != nil {
		return nil, fmt.Errorf("failed to list delivery schedules: %w", err)
	}
	return schedules, nil
}

// GetByID returns a single schedule by ID.
func (r *DeliveryScheduleRepository) GetByID(ctx context.Context, id int64) (*model.DeliverySchedule, error) {
	const q = `
		SELECT id, day_of_week, slot_name, start_time::text AS start_time, end_time::text AS end_time, max_capacity, is_active, created_at, updated_at
		FROM delivery_schedules
		WHERE id = $1`
	var s model.DeliverySchedule
	if err := r.db.GetContext(ctx, &s, q, id); err != nil {
		return nil, err
	}
	return &s, nil
}

// Create inserts a new delivery schedule record.
func (r *DeliveryScheduleRepository) Create(ctx context.Context, s *model.DeliverySchedule) error {
	const q = `
		INSERT INTO delivery_schedules (day_of_week, slot_name, start_time, end_time, max_capacity, is_active, created_at, updated_at)
		VALUES ($1, $2, $3::time, $4::time, $5, $6, NOW(), NOW())
		RETURNING id, created_at, updated_at`
	return r.db.QueryRowxContext(ctx, q, s.DayOfWeek, s.SlotName, s.StartTime, s.EndTime, s.MaxCapacity, s.IsActive).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

// Update updates an existing delivery schedule record.
func (r *DeliveryScheduleRepository) Update(ctx context.Context, s *model.DeliverySchedule) error {
	const q = `
		UPDATE delivery_schedules
		SET day_of_week = $1, slot_name = $2, start_time = $3::time, end_time = $4::time, max_capacity = $5, is_active = $6, updated_at = NOW()
		WHERE id = $7
		RETURNING updated_at`
	return r.db.QueryRowxContext(ctx, q, s.DayOfWeek, s.SlotName, s.StartTime, s.EndTime, s.MaxCapacity, s.IsActive, s.ID).Scan(&s.UpdatedAt)
}

// Delete removes a delivery schedule record by ID.
func (r *DeliveryScheduleRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM delivery_schedules WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("delivery schedule not found")
	}
	return nil
}

