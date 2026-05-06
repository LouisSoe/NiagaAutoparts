package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/model"
)

// ProductRepository handles all product database operations.
type ProductRepository struct {
	db *sqlx.DB
}

func NewProductRepository(db *sqlx.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// Search performs a fuzzy search on name, sku, and category using pg_trgm similarity.
// Strategy:
//   - Single word  → searchSingle (trigram + ILIKE fallback)
//   - Multi word   → searchMulti  (OR per kata, ranked by match count + score)
//
// Result cap:
//   - Jika ada produk dengan match count == len(words) dan score tinggi → kembalikan 1
//   - Jika tidak ada yang perfect match → kembalikan hingga 10 kandidat terdekat
func (r *ProductRepository) Search(ctx context.Context, words []string) ([]model.Product, error) {
	if len(words) == 1 {
		return r.searchSingle(ctx, words[0])
	}
	return r.searchMulti(ctx, words)
}

// searchSingle — satu kata, trigram + ILIKE fallback.
// Kembalikan 1 jika score teratas >= 0.5, selain itu kembalikan hingga 10.
func (r *ProductRepository) searchSingle(ctx context.Context, word string) ([]model.Product, error) {
	const q = `
		SELECT id, sku, name, category, description, stock, reserved,
		       location, price, unit, image_url, is_active, created_at, updated_at,
		       GREATEST(
		           similarity(name, $1),
		           similarity(sku,  $1),
		           similarity(category, $1)
		       ) AS _score
		FROM products
		WHERE is_active = true
		  AND (
		        name     % $1 OR sku % $1 OR category % $1
		        OR name        ILIKE '%' || $1 || '%'
		        OR sku         ILIKE '%' || $1 || '%'
		        OR category    ILIKE '%' || $1 || '%'
		        OR description ILIKE '%' || $1 || '%'
		  )
		ORDER BY _score DESC, stock DESC
		LIMIT 10`

	rows, err := r.db.QueryContext(ctx, q, word)
	if err != nil {
		return nil, fmt.Errorf("searchSingle: %w", err)
	}
	defer rows.Close()

	all, err := scanProductsWithScore(rows)
	if err != nil || len(all) == 0 {
		return toProducts(all), err
	}

	// Jika produk teratas punya score tinggi → kembalikan hanya 1
	if all[0].Score >= 0.5 {
		return []model.Product{all[0].Product}, nil
	}
	// Score rendah → kembalikan semua kandidat (max 10)
	return toProducts(all), nil
}

// searchMulti — multi kata, OR per kata, ranked by match count lalu similarity score.
// Kembalikan 1 jika ada produk yang match semua kata dengan score tinggi.
// Selain itu kembalikan hingga 10 kandidat.
func (r *ProductRepository) searchMulti(ctx context.Context, words []string) ([]model.Product, error) {
	args := []interface{}{}
	conditions := []string{}
	scoreExprs := []string{}
	matchCountExprs := []string{}
	argIdx := 1

	for _, word := range words {
		n := argIdx
		argIdx++
		args = append(args, word)

		conditions = append(conditions, fmt.Sprintf(
			`(name        ILIKE '%%'||$%d||'%%'
			  OR sku       ILIKE '%%'||$%d||'%%'
			  OR category  ILIKE '%%'||$%d||'%%'
			  OR description ILIKE '%%'||$%d||'%%'
			  OR name      %% $%d
			  OR sku       %% $%d
			  OR category  %% $%d)`,
			n, n, n, n, n, n, n,
		))

		scoreExprs = append(scoreExprs, fmt.Sprintf(
			`GREATEST(similarity(name,$%d), similarity(sku,$%d), similarity(category,$%d))`,
			n, n, n,
		))

		// +1 untuk setiap kata yang match di salah satu kolom
		matchCountExprs = append(matchCountExprs, fmt.Sprintf(
			`CASE WHEN (
				name       ILIKE '%%'||$%d||'%%' OR
				sku        ILIKE '%%'||$%d||'%%' OR
				name       %% $%d OR
				category   %% $%d
			) THEN 1 ELSE 0 END`,
			n, n, n, n,
		))
	}

	scoreSQL      := "(" + strings.Join(scoreExprs, " + ") + ")"
	matchCountSQL := "(" + strings.Join(matchCountExprs, " + ") + ")"
	limitArg := argIdx
	args = append(args, 10)

	q := fmt.Sprintf(`
		SELECT id, sku, name, category, description, stock, reserved,
		       location, price, unit, image_url, is_active, created_at, updated_at,
		       %s AS _score,
		       %s AS _match_count
		FROM products
		WHERE is_active = true
		  AND (%s)
		  AND %s > 0.05
		ORDER BY
		  %s DESC,
		  %s DESC,
		  stock DESC
		LIMIT $%d`,
		scoreSQL,
		matchCountSQL,
		strings.Join(conditions, "\n		  OR "),
		scoreSQL,
		matchCountSQL, // primary: berapa kata yang match
		scoreSQL,      // secondary: seberapa mirip
		limitArg,
	)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("searchMulti: %w", err)
	}
	defer rows.Close()

	all, err := scanProductsWithMatchCount(rows)
	if err != nil || len(all) == 0 {
		return toProductsMC(all), err
	}

	totalWords := len(words)
	top := all[0]

	// Perfect match: semua kata cocok + score cukup tinggi → kembalikan 1
	if top.MatchCount == totalWords && top.Score >= 0.3 {
		return []model.Product{top.Product}, nil
	}

	// Partial match tapi ada yang jauh lebih baik dari yang lain → kembalikan 1
	if len(all) > 1 && top.MatchCount > all[1].MatchCount && top.Score >= 0.4 {
		return []model.Product{top.Product}, nil
	}

	// Tidak ada yang dominan → kembalikan semua kandidat
	return toProductsMC(all), nil
}

// ── Scan helpers ─────────────────────────────────────────────────────────────

type productWithScore struct {
	model.Product
	Score float64
}

type productWithMatchCount struct {
	model.Product
	Score      float64
	MatchCount int
}

func scanProductsWithScore(rows *sql.Rows) ([]productWithScore, error) {
	var result []productWithScore
	for rows.Next() {
		var p model.Product
		var score float64
		if err := rows.Scan(
			&p.ID, &p.SKU, &p.Name, &p.Category, &p.Description,
			&p.Stock, &p.Reserved, &p.Location, &p.Price, &p.Unit,
			&p.ImageURL, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
			&score,
		); err != nil {
			return nil, err
		}
		result = append(result, productWithScore{p, score})
	}
	return result, rows.Err()
}

func scanProductsWithMatchCount(rows *sql.Rows) ([]productWithMatchCount, error) {
	var result []productWithMatchCount
	for rows.Next() {
		var p model.Product
		var score float64
		var matchCount int
		if err := rows.Scan(
			&p.ID, &p.SKU, &p.Name, &p.Category, &p.Description,
			&p.Stock, &p.Reserved, &p.Location, &p.Price, &p.Unit,
			&p.ImageURL, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
			&score, &matchCount,
		); err != nil {
			return nil, err
		}
		result = append(result, productWithMatchCount{p, score, matchCount})
	}
	return result, rows.Err()
}

func toProducts(src []productWithScore) []model.Product {
	out := make([]model.Product, len(src))
	for i, s := range src {
		out[i] = s.Product
	}
	return out
}

func toProductsMC(src []productWithMatchCount) []model.Product {
	out := make([]model.Product, len(src))
	for i, s := range src {
		out[i] = s.Product
	}
	return out
}

// scanProducts — dipertahankan untuk kompatibilitas method lain (GetAll, dll)
func scanProducts(rows *sql.Rows) ([]model.Product, error) {
	var products []model.Product
	for rows.Next() {
		var p model.Product
		if err := rows.Scan(
			&p.ID, &p.SKU, &p.Name, &p.Category, &p.Description,
			&p.Stock, &p.Reserved, &p.Location, &p.Price, &p.Unit,
			&p.ImageURL, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

// ── Token dictionary ──────────────────────────────────────────────────────────

func (r *ProductRepository) GetAllTokens(ctx context.Context) ([]string, error) {
	const q = `SELECT name, category, sku FROM products WHERE is_active = true`
	rows, err := r.db.QueryxContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("GetAllTokens: %w", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	dict := []string{}

	for rows.Next() {
		var name, category, sku string
		if err := rows.Scan(&name, &category, &sku); err != nil {
			return nil, err
		}
		for _, field := range []string{name, category, sku} {
			for _, word := range strings.Fields(strings.ToLower(field)) {
				word = strings.Trim(word, "/-.,()[]")
				if len([]rune(word)) >= 3 && !seen[word] {
					seen[word] = true
					dict = append(dict, word)
				}
			}
		}
	}
	return dict, rows.Err()
}

// ── Stock & order operations ──────────────────────────────────────────────────

// GetAll retrieves every active product from the database.
func (r *ProductRepository) GetAll(ctx context.Context) ([]model.Product, error) {
	const q = `
		SELECT id, sku, name, category, description, stock, reserved,
		       location, price, unit, image_url, is_active, created_at, updated_at
		FROM products
		WHERE is_active = true
		ORDER BY name ASC`
	var products []model.Product
	err := r.db.SelectContext(ctx, &products, q)
	return products, err
}

// GetByID retrieves a single product by its ID.
func (r *ProductRepository) GetByID(ctx context.Context, id int64) (*model.Product, error) {
	const q = `
		SELECT id, sku, name, category, description, stock, reserved,
		       location, price, unit, image_url, is_active, created_at, updated_at
		FROM products WHERE id = $1 AND is_active = true`
	var p model.Product
	if err := r.db.GetContext(ctx, &p, q, id); err != nil {
		return nil, err
	}
	return &p, nil
}

// ReserveStock atomically increases the reserved count if sufficient stock exists.
func (r *ProductRepository) ReserveStock(ctx context.Context, productID int64, qty int) error {
	const q = `
		UPDATE products
		SET reserved   = reserved + $1,
		    updated_at = NOW()
		WHERE id = $2
		  AND is_active = true
		  AND (stock - reserved) >= $3`
	res, err := r.db.ExecContext(ctx, q, qty, productID, qty)
	if err != nil {
		return fmt.Errorf("reserve stock exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reserve stock rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("insufficient stock for product %d (qty=%d)", productID, qty)
	}
	return nil
}

// DeductStock permanently reduces stock and reserved counts after payment.
// Must be called inside a transaction with the order status update.
func (r *ProductRepository) DeductStock(ctx context.Context, tx *sqlx.Tx, productID int64, qty int) error {
	const q = `
		UPDATE products
		SET stock      = stock - $1,
		    reserved   = reserved - $2,
		    updated_at = NOW()
		WHERE id = $3
		  AND stock    >= $4
		  AND reserved >= $5`
	res, err := tx.ExecContext(ctx, q, qty, qty, productID, qty, qty)
	if err != nil {
		return fmt.Errorf("deduct stock exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("deduct stock failed for product %d", productID)
	}
	return nil
}

// ReleaseReservation decrements the reserved count when an order is cancelled.
func (r *ProductRepository) ReleaseReservation(ctx context.Context, productID int64, qty int) error {
	const q = `
		UPDATE products
		SET reserved   = GREATEST(0, reserved - $1),
		    updated_at = NOW()
		WHERE id = $2`
	_, err := r.db.ExecContext(ctx, q, qty, productID)
	return err
}

// GetPriceReferences returns marketplace price references for a product.
func (r *ProductRepository) GetPriceReferences(ctx context.Context, productID int64) ([]model.PriceReference, error) {
	const q = `
		SELECT id, product_id, marketplace, price, url, fetched_at
		FROM price_references
		WHERE product_id = $1
		  AND fetched_at > $2
		ORDER BY price ASC`
	var refs []model.PriceReference
	cutoff := time.Now().AddDate(0, 0, -7)
	err := r.db.SelectContext(ctx, &refs, q, productID, cutoff)
	return refs, err
}