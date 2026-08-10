package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Product struct {
	ID            int64          `db:"id" json:"id"`
	SKU           string         `db:"sku" json:"sku"`
	Name          string         `db:"name" json:"name"`
	CategoryID    sql.NullInt64  `db:"category_id" json:"category_id"`
	CategoryName  string         `db:"category_name" json:"category_name,omitempty"`
	Description   sql.NullString `db:"description" json:"description"`
	Stock         int            `db:"stock" json:"stock"`
	MinimumStock  int            `db:"minimum_stock" json:"minimum_stock"`
	Reserved      int            `db:"reserved" json:"reserved"`
	Location      string         `db:"location" json:"location"`
	PurchasePrice float64        `db:"purchase_price" json:"purchase_price"`
	SellingPrice  float64        `db:"selling_price" json:"selling_price"`
	Unit          string         `db:"unit" json:"unit"`
	ImageURL      sql.NullString `db:"image_url" json:"image_url"`
	IsActive      bool           `db:"is_active" json:"is_active"`
	CreatedAt     time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at" json:"updated_at"`
}

func (p Product) MarshalJSON() ([]byte, error) {
	type Alias Product
	var categoryID *int64
	if p.CategoryID.Valid {
		categoryID = &p.CategoryID.Int64
	}
	var description *string
	if p.Description.Valid {
		description = &p.Description.String
	}
	var imageURL *string
	if p.ImageURL.Valid {
		imageURL = &p.ImageURL.String
	}
	return json.Marshal(&struct {
		Alias
		CategoryID  *int64  `json:"category_id"`
		Description *string `json:"description"`
		ImageURL    *string `json:"image_url"`
	}{
		Alias:       Alias(p),
		CategoryID:  categoryID,
		Description: description,
		ImageURL:    imageURL,
	})
}

func (p *Product) UnmarshalJSON(data []byte) error {
	type Alias Product
	aux := &struct {
		*Alias
		CategoryID    *int64   `json:"category_id"`
		Description   *string  `json:"description"`
		ImageURL      *string  `json:"image_url"`
		Price         *float64 `json:"price"`
		BuyPrice      *float64 `json:"buy_price"`
		PurchasePrice *float64 `json:"purchasePrice"`
		SellingPrice  *float64 `json:"sellingPrice"`
		MinimumStock  *int     `json:"minimumStock"`
		MinStock      *int     `json:"min_stock"`
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.CategoryID != nil {
		p.CategoryID.Int64 = *aux.CategoryID
		p.CategoryID.Valid = true
	}
	if aux.Description != nil {
		p.Description.String = *aux.Description
		p.Description.Valid = true
	}
	if aux.ImageURL != nil {
		p.ImageURL.String = *aux.ImageURL
		p.ImageURL.Valid = true
	}
	if aux.PurchasePrice != nil {
		p.PurchasePrice = *aux.PurchasePrice
	} else if aux.BuyPrice != nil {
		p.PurchasePrice = *aux.BuyPrice
	} else if aux.Price != nil && p.PurchasePrice == 0 {
		p.PurchasePrice = *aux.Price
	}
	if aux.SellingPrice != nil {
		p.SellingPrice = *aux.SellingPrice
	} else if aux.Price != nil && p.SellingPrice == 0 {
		p.SellingPrice = *aux.Price
	}
	if aux.MinimumStock != nil {
		p.MinimumStock = *aux.MinimumStock
	} else if aux.MinStock != nil {
		p.MinimumStock = *aux.MinStock
	}
	return nil
}

// AvailableStock returns stock that is not reserved.
func (p *Product) AvailableStock() int {
	return p.Stock - p.Reserved
}
