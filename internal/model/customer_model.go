package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

type CustomerType string

const (
	CustomerTypeIndividual CustomerType = "INDIVIDUAL"
	CustomerTypeWorkshop   CustomerType = "WORKSHOP"
	CustomerTypeCompany    CustomerType = "COMPANY"
)

type Customer struct {
	ID           int64          `db:"id" json:"id"`
	UserID       int64          `db:"user_id" json:"user_id"`
	TypeCustomer CustomerType   `db:"type_customer" json:"type_customer"`
	Name         string         `db:"name" json:"name"`
	Phone        string         `db:"phone" json:"phone"`
	Email        string         `db:"email" json:"email"`
	Address      sql.NullString  `db:"address" json:"address"`
	Latitude     sql.NullFloat64 `db:"latitude" json:"latitude"`
	Longitude    sql.NullFloat64 `db:"longitude" json:"longitude"`
	Notes        sql.NullString  `db:"notes" json:"notes"`
	CreatedAt    time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at" json:"updated_at"`
}

func (c Customer) MarshalJSON() ([]byte, error) {
	type Alias Customer
	var address *string
	if c.Address.Valid {
		address = &c.Address.String
	}
	var lat *float64
	if c.Latitude.Valid {
		lat = &c.Latitude.Float64
	}
	var lng *float64
	if c.Longitude.Valid {
		lng = &c.Longitude.Float64
	}
	var notes *string
	if c.Notes.Valid {
		notes = &c.Notes.String
	}
	return json.Marshal(&struct {
		Alias
		Address   *string  `json:"address"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
		Notes     *string  `json:"notes"`
	}{
		Alias:     Alias(c),
		Address:   address,
		Latitude:  lat,
		Longitude: lng,
		Notes:     notes,
	})
}

