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
    StateAwaitingConfirm          SessionState = "awaiting_confirm"
    StateOrdering                 SessionState = "ordering"
    StateAwaitingProductSelection SessionState = "awaiting_product_selection"
    StateAwaitingImportConfirm    SessionState = "awaiting_import_confirm"
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

    // SearchResults tidak disimpan langsung ke DB — dimuat dari Context
    SearchResults []Product `db:"-" json:"-"`
}

// sessionContext adalah struktur yang di-encode ke kolom context
type sessionContext struct {
    SearchResults []Product `json:"search_results,omitempty"`
}

// LoadContext mengisi field SearchResults dari kolom Context (JSON)
func (s *Session) LoadContext() {
    if s.Context == "" {
        return
    }
    var sc sessionContext
    if err := json.Unmarshal([]byte(s.Context), &sc); err == nil {
        s.SearchResults = sc.SearchResults
    }
}

// SaveContext menyimpan SearchResults ke kolom Context sebagai JSON
func (s *Session) SaveContext() {
    sc := sessionContext{SearchResults: s.SearchResults}
    if b, err := json.Marshal(sc); err == nil {
        s.Context = string(b)
    }
}