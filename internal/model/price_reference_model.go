package model
import "time"

// PriceReference stores marketplace pricing data for comparison.
type PriceReference struct {
	ID          int64     `db:"id" json:"id"`
	ProductID   int64     `db:"product_id" json:"product_id"`
	Marketplace string    `db:"marketplace" json:"marketplace"` // tokopedia, shopee, lazada
	Price       float64   `db:"price" json:"price"`
	URL         string    `db:"url" json:"url"`
	FetchedAt   time.Time `db:"fetched_at" json:"fetched_at"`
}
