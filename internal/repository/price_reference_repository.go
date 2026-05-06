package repository

import (
    "context"
    "github.com/jmoiron/sqlx"
    "github.com/louissoe/niaga-autoparts/internal/model"
)

type PriceReferenceRepository struct {
    db *sqlx.DB
}

func NewPriceReferenceRepository(db *sqlx.DB) *PriceReferenceRepository {
    return &PriceReferenceRepository{db: db}
}

func (r *PriceReferenceRepository) Upsert(ctx context.Context, ref model.PriceReference) error {
    const q = `
        INSERT INTO price_references (product_id, marketplace, price, url, fetched_at)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (product_id, marketplace) DO UPDATE SET
            price      = EXCLUDED.price,
            url        = EXCLUDED.url,
            fetched_at = EXCLUDED.fetched_at`
    _, err := r.db.ExecContext(ctx, q,
        ref.ProductID, ref.Marketplace, ref.Price, ref.URL, ref.FetchedAt,
    )
    return err
}