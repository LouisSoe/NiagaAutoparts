package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// DeliverySchedule represents master delivery slots with maximum capacity.
type DeliverySchedule struct {
	ID          int64     `db:"id" json:"id"`
	DayOfWeek   string    `db:"day_of_week" json:"day_of_week"`
	SlotName    string    `db:"slot_name" json:"slot_name"`
	StartTime   string    `db:"start_time" json:"start_time"`
	EndTime     string    `db:"end_time" json:"end_time"`
	MaxCapacity int       `db:"max_capacity" json:"max_capacity"`
	IsActive    bool      `db:"is_active" json:"is_active"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`

	// Dynamic fields for booking availability
	BookedCount    int  `db:"booked_count" json:"booked_count"`
	AvailableSlots int  `db:"available_slots" json:"available_slots"`
	IsFull         bool `db:"is_full" json:"is_full"`
}

// DeliveryStatus represents the state of a delivery task.
type DeliveryStatus string

const (
	DeliveryStatusWaitingCourier DeliveryStatus = "waiting_courier_approval"
	DeliveryStatusConfirmed      DeliveryStatus = "confirmed"
	DeliveryStatusReschedule     DeliveryStatus = "reschedule_suggested"
	DeliveryStatusOnDelivery     DeliveryStatus = "on_delivery"
	DeliveryStatusDelivered      DeliveryStatus = "delivered"
	DeliveryStatusCancelled      DeliveryStatus = "cancelled"
)

// Delivery represents a delivery record for an order.
type Delivery struct {
	ID                  int64          `db:"id" json:"id"`
	OrderID             int64          `db:"order_id" json:"order_id"`
	CustomerID          sql.NullInt64  `db:"customer_id" json:"customer_id"`
	ScheduleID          int64          `db:"schedule_id" json:"schedule_id"`
	CourierID           sql.NullInt64  `db:"courier_id" json:"courier_id"`
	DeliveryDate        time.Time      `db:"delivery_date" json:"delivery_date"`
	Status              DeliveryStatus `db:"status" json:"status"`
	ShippingCost        float64        `db:"shipping_cost" json:"shipping_cost"`
	DistanceKm          float64        `db:"distance_km" json:"distance_km"`
	SuggestedScheduleID sql.NullInt64  `db:"suggested_schedule_id" json:"suggested_schedule_id"`
	SuggestedDate       *time.Time     `db:"suggested_date" json:"suggested_date"`
	RejectionReason     sql.NullString `db:"rejection_reason" json:"rejection_reason"`
	Notes               string         `db:"notes" json:"notes"`
	CreatedAt           time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time      `db:"updated_at" json:"updated_at"`

	// Joined fields
	OrderNumber           string  `db:"order_number" json:"order_number,omitempty"`
	CustomerName          string  `db:"customer_name" json:"customer_name,omitempty"`
	CustomerPhone         string  `db:"customer_phone" json:"customer_phone,omitempty"`
	CustomerAddress       string  `db:"customer_address" json:"customer_address,omitempty"`
	CustomerLatitude      float64 `db:"customer_latitude" json:"customer_latitude,omitempty"`
	CustomerLongitude     float64 `db:"customer_longitude" json:"customer_longitude,omitempty"`
	SlotName              string  `db:"slot_name" json:"slot_name,omitempty"`
	SuggestedSlotName     string  `db:"suggested_slot_name" json:"suggested_slot_name,omitempty"`
	CourierName           string  `db:"courier_name" json:"courier_name,omitempty"`
	TelegramChatID        string  `db:"telegram_chat_id" json:"telegram_chat_id,omitempty"`
}

func (d Delivery) MarshalJSON() ([]byte, error) {
	type Alias Delivery
	var custID *int64
	if d.CustomerID.Valid {
		custID = &d.CustomerID.Int64
	}
	var courierID *int64
	if d.CourierID.Valid {
		courierID = &d.CourierID.Int64
	}
	var suggestedSchedID *int64
	if d.SuggestedScheduleID.Valid {
		suggestedSchedID = &d.SuggestedScheduleID.Int64
	}
	var rejectionReason *string
	if d.RejectionReason.Valid {
		rejectionReason = &d.RejectionReason.String
	}

	return json.Marshal(&struct {
		Alias
		CustomerID          *int64  `json:"customer_id"`
		CourierID           *int64  `json:"courier_id"`
		SuggestedScheduleID *int64  `json:"suggested_schedule_id"`
		RejectionReason     *string `json:"rejection_reason"`
		DeliveryDate        string  `json:"delivery_date"`
	}{
		Alias:               Alias(d),
		CustomerID:          custID,
		CourierID:           courierID,
		SuggestedScheduleID: suggestedSchedID,
		RejectionReason:     rejectionReason,
		DeliveryDate:        d.DeliveryDate.Format("2006-01-02"),
	})
}
