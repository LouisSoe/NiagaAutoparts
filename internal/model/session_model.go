package model

import (
    "encoding/json"
    "time"
)

type SessionState string

const (
    StateIdle                     SessionState = "idle"
    StateSearching                SessionState = "searching"
    StateAwaitingQty              SessionState = "awaiting_qty"
    StateAwaitingOrderType        SessionState = "awaiting_order_type"
    StateAwaitingDeliveryAddress        SessionState = "await_deliv_addr"
    StateAwaitingDeliveryAddressDetail  SessionState = "await_deliv_detail"
    StateAwaitingAddressConfirmation    SessionState = "await_addr_confirm"
    StateAwaitingDeliveryDate           SessionState = "await_deliv_date"
    StateAwaitingDeliverySchedule       SessionState = "await_deliv_sched"
    StateAwaitingConfirm                SessionState = "awaiting_confirm"
    StateOrdering                       SessionState = "ordering"
    StateAwaitingProductSelection       SessionState = "await_prod_select"
    StateAwaitingImportConfirm          SessionState = "await_import_conf"
    StateAwaitingRescheduleDecision     SessionState = "await_resched_dec"
)

type Session struct {
    ID              int64        `db:"id"               json:"id"`
    PhoneNumber     string       `db:"phone_number"     json:"phone_number"`
    State           SessionState `db:"state"            json:"state"`
    LastIntent      string       `db:"last_intent"      json:"last_intent"`
    LastProductID   *int64       `db:"last_product_id"  json:"last_product_id"`
    LastProductName string       `db:"last_product_name" json:"last_product_name"`
    PendingOrderID  *int64       `db:"pending_order_id" json:"pending_order_id"`
    Context         string       `db:"context"          json:"context"`
    UpdatedAt       time.Time    `db:"updated_at"       json:"updated_at"`
    ExpiresAt       time.Time    `db:"expires_at"       json:"expires_at"`

    // Fields tidak disimpan langsung ke DB — dimuat dari Context
    SearchResults     []Product          `db:"-" json:"-"`
    PendingOrderType  string             `db:"-" json:"-"`
    PendingAddress    string             `db:"-" json:"-"`
    PendingLat        *float64           `db:"-" json:"-"`
    PendingLng        *float64           `db:"-" json:"-"`
    PendingShipping   float64            `db:"-" json:"-"`
    PendingDistanceKm float64            `db:"-" json:"-"`
    PendingDeliveryID *int64             `db:"-" json:"-"`
    PendingDate       string             `db:"-" json:"-"`
    AvailSchedules    []DeliverySchedule `db:"-" json:"-"`
}

// sessionContext adalah struktur yang di-encode ke kolom context
type sessionContext struct {
    SearchResults     []Product          `json:"search_results,omitempty"`
    PendingOrderType  string             `json:"pending_order_type,omitempty"`
    PendingAddress    string             `json:"pending_address,omitempty"`
    PendingLat        *float64           `json:"pending_lat,omitempty"`
    PendingLng        *float64           `json:"pending_lng,omitempty"`
    PendingShipping   float64            `json:"pending_shipping,omitempty"`
    PendingDistanceKm float64            `json:"pending_distance_km,omitempty"`
    PendingDeliveryID *int64             `json:"pending_delivery_id,omitempty"`
    PendingDate       string             `json:"pending_date,omitempty"`
    AvailSchedules    []DeliverySchedule `json:"avail_schedules,omitempty"`
}

// LoadContext mengisi field dari kolom Context (JSON)
func (s *Session) LoadContext() {
    if s.Context == "" {
        return
    }
    var sc sessionContext
    if err := json.Unmarshal([]byte(s.Context), &sc); err == nil {
        s.SearchResults = sc.SearchResults
        s.PendingOrderType = sc.PendingOrderType
        s.PendingAddress = sc.PendingAddress
        s.PendingLat = sc.PendingLat
        s.PendingLng = sc.PendingLng
        s.PendingShipping = sc.PendingShipping
        s.PendingDistanceKm = sc.PendingDistanceKm
        s.PendingDeliveryID = sc.PendingDeliveryID
        s.PendingDate = sc.PendingDate
        s.AvailSchedules = sc.AvailSchedules
    }
}

// SaveContext menyimpan data ke kolom Context sebagai JSON
func (s *Session) SaveContext() {
    sc := sessionContext{
        SearchResults:     s.SearchResults,
        PendingOrderType:  s.PendingOrderType,
        PendingAddress:    s.PendingAddress,
        PendingLat:        s.PendingLat,
        PendingLng:        s.PendingLng,
        PendingShipping:   s.PendingShipping,
        PendingDistanceKm: s.PendingDistanceKm,
        PendingDeliveryID: s.PendingDeliveryID,
        PendingDate:       s.PendingDate,
        AvailSchedules:    s.AvailSchedules,
    }
    if b, err := json.Marshal(sc); err == nil {
        s.Context = string(b)
    }
}