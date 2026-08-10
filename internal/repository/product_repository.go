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

// Search performs a fuzzy search on name, sku, and category name using pg_trgm similarity.
func (r *ProductRepository) Search(ctx context.Context, words []string) ([]model.Product, error) {
	if len(words) == 1 {
		return r.searchSingle(ctx, words[0])
	}
	return r.searchMulti(ctx, words)
}

// searchSingle — satu kata, trigram + ILIKE fallback
func (r *ProductRepository) searchSingle(ctx context.Context, word string) ([]model.Product, error) {
	const q = `
		SELECT p.id, p.sku, p.name, p.category_id, COALESCE(c.name, '') AS category_name, p.description, p.stock, p.minimum_stock, p.reserved,
		       p.location, p.purchase_price, p.selling_price, p.unit, p.image_url, p.is_active, p.created_at, p.updated_at,
		       GREATEST(
		           similarity(p.name, $1),
		           similarity(p.sku,  $1),
		           similarity(COALESCE(c.name, ''), $1)
		       ) AS _score
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE p.is_active = true
		  AND (
		        p.name % $1 OR p.sku % $1 OR COALESCE(c.name, '') % $1
		        OR p.name        ILIKE '%' || $1 || '%'
		        OR p.sku         ILIKE '%' || $1 || '%'
		        OR c.name        ILIKE '%' || $1 || '%'
		        OR p.description ILIKE '%' || $1 || '%'
		  )
		ORDER BY _score DESC, p.stock DESC
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

// searchMulti — multi kata, OR per kata.
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
			`(p.name          ILIKE '%%'||$%d||'%%'
			  OR p.sku        ILIKE '%%'||$%d||'%%'
			  OR c.name       ILIKE '%%'||$%d||'%%'
			  OR p.description ILIKE '%%'||$%d||'%%'
			  OR p.name       %% $%d
			  OR p.sku        %% $%d
			  OR c.name       %% $%d)`,
			n, n, n, n, n, n, n,
		))

		scoreExprs = append(scoreExprs, fmt.Sprintf(
			`GREATEST(similarity(p.name,$%d), similarity(p.sku,$%d), similarity(COALESCE(c.name, ''),$%d))`,
			n, n, n,
		))

		// +1 untuk setiap kata yang match di salah satu kolom
		matchCountExprs = append(matchCountExprs, fmt.Sprintf(
			`CASE WHEN (
				p.name       ILIKE '%%'||$%d||'%%' OR
				p.sku        ILIKE '%%'||$%d||'%%' OR
				p.name       %% $%d OR
				c.name       %% $%d
			) THEN 1 ELSE 0 END`,
			n, n, n, n,
		))
	}

	scoreSQL := "(" + strings.Join(scoreExprs, " + ") + ")"
	matchCountSQL := "(" + strings.Join(matchCountExprs, " + ") + ")"
	limitArg := argIdx
	args = append(args, 10)

	q := fmt.Sprintf(`
		SELECT p.id, p.sku, p.name, p.category_id, COALESCE(c.name, '') AS category_name, p.description, p.stock, p.minimum_stock, p.reserved,
		       p.location, p.purchase_price, p.selling_price, p.unit, p.image_url, p.is_active, p.created_at, p.updated_at,
		       %s AS _score,
		       %s AS _match_count
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE p.is_active = true
		  AND (%s)
		  AND %s > 0.05
		ORDER BY
		  %s DESC,
		  %s DESC,
		  p.stock DESC
		LIMIT $%d`,
		scoreSQL,
		matchCountSQL,
		strings.Join(conditions, " OR "),
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
			&p.ID, &p.SKU, &p.Name, &p.CategoryID, &p.CategoryName, &p.Description,
			&p.Stock, &p.MinimumStock, &p.Reserved, &p.Location, &p.PurchasePrice, &p.SellingPrice, &p.Unit,
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
			&p.ID, &p.SKU, &p.Name, &p.CategoryID, &p.CategoryName, &p.Description,
			&p.Stock, &p.MinimumStock, &p.Reserved, &p.Location, &p.PurchasePrice, &p.SellingPrice, &p.Unit,
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
			&p.ID, &p.SKU, &p.Name, &p.CategoryID, &p.CategoryName, &p.Description,
			&p.Stock, &p.MinimumStock, &p.Reserved, &p.Location, &p.PurchasePrice, &p.SellingPrice, &p.Unit,
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
	const q = `
		SELECT p.name, COALESCE(c.name, '') AS category_name, p.sku
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE p.is_active = true`
	rows, err := r.db.QueryxContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("GetAllTokens: %w", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	dict := []string{}

	for rows.Next() {
		var name, categoryName, sku string
		if err := rows.Scan(&name, &categoryName, &sku); err != nil {
			return nil, err
		}
		for _, field := range []string{name, categoryName, sku} {
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
		SELECT p.id, p.sku, p.name, p.category_id, COALESCE(c.name, '') AS category_name, p.description, p.stock, p.minimum_stock, p.reserved,
		       p.location, p.purchase_price, p.selling_price, p.unit, p.image_url, p.is_active, p.created_at, p.updated_at
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE p.is_active = true
		ORDER BY p.name ASC`
	var products []model.Product
	err := r.db.SelectContext(ctx, &products, q)
	return products, err
}

// GetByID retrieves a single product by its ID.
func (r *ProductRepository) GetByID(ctx context.Context, id int64) (*model.Product, error) {
	const q = `
		SELECT p.id, p.sku, p.name, p.category_id, COALESCE(c.name, '') AS category_name, p.description, p.stock, p.minimum_stock, p.reserved,
		       p.location, p.purchase_price, p.selling_price, p.unit, p.image_url, p.is_active, p.created_at, p.updated_at
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE p.id = $1 AND p.is_active = true`
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

// ReserveStockTx atomically increases the reserved count if sufficient stock exists within an active transaction.
func (r *ProductRepository) ReserveStockTx(ctx context.Context, tx *sqlx.Tx, productID int64, qty int) error {
	const q = `
		UPDATE products
		SET reserved   = reserved + $1,
		    updated_at = NOW()
		WHERE id = $2
		  AND is_active = true
		  AND (stock - reserved) >= $3`
	res, err := tx.ExecContext(ctx, q, qty, productID, qty)
	if err != nil {
		return fmt.Errorf("reserve stock exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reserve stock rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("stok tidak cukup untuk produk %d (qty=%d)", productID, qty)
	}
	return nil
}

// DeductStock permanently reduces stock and reserved counts after payment for reserved orders.
func (r *ProductRepository) DeductStock(ctx context.Context, tx *sqlx.Tx, productID int64, qty int) error {
	const q = `
		UPDATE products
		SET stock      = stock - $1,
		    reserved   = GREATEST(0, reserved - $2),
		    updated_at = NOW()
		WHERE id = $3
		  AND stock    >= $4`
	res, err := tx.ExecContext(ctx, q, qty, qty, productID, qty)
	if err != nil {
		return fmt.Errorf("deduct stock exec: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("deduct stock failed for product %d (stok tidak mencukupi)", productID)
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

// ── Excel Import Operations ───────────────────────────────────────────────────

// ExcelProductInput data produk dari file Excel untuk upsert
type ExcelProductInput struct {
	SKU           string
	Name          string
	Category      string
	Description   string
	Stock         int
	MinimumStock  int
	PurchasePrice float64
	SellingPrice  float64
	Unit          string
	Location      string
}

// UpsertFromExcel insert produk baru atau update stok/harga jika SKU sudah ada.
// Jika SKU kosong, match by name.
func (r *ProductRepository) UpsertFromExcel(ctx context.Context, input ExcelProductInput) error {
	if input.Name == "" {
		return fmt.Errorf("nama produk tidak boleh kosong")
	}

	// set default unit jika kosong
	if input.Unit == "" {
		input.Unit = "pcs"
	}

	// Lookup or create category_id from category name
	var categoryID sql.NullInt64
	if input.Category != "" {
		var catID int64
		err := r.db.GetContext(ctx, &catID, `SELECT id FROM categories WHERE LOWER(name) = LOWER($1)`, input.Category)
		if err == sql.ErrNoRows {
			slug := strings.ToLower(strings.ReplaceAll(input.Category, " ", "-"))
			err = r.db.QueryRowContext(ctx, `INSERT INTO categories (name, slug, created_at, updated_at) VALUES ($1, $2, NOW(), NOW()) RETURNING id`, input.Category, slug).Scan(&catID)
		}
		if err == nil && catID > 0 {
			categoryID.Int64 = catID
			categoryID.Valid = true
		}
	}

	const q = `
		INSERT INTO products
		  (sku, name, category_id, description, stock, minimum_stock, purchase_price, selling_price, unit, location, is_active, created_at, updated_at)
		VALUES
		  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true, NOW(), NOW())
		ON CONFLICT (sku) DO UPDATE SET
		  name           = CASE WHEN EXCLUDED.name != '' THEN EXCLUDED.name ELSE products.name END,
		  category_id    = EXCLUDED.category_id,
		  description    = CASE WHEN EXCLUDED.description != '' THEN EXCLUDED.description ELSE products.description END,
		  stock          = CASE WHEN EXCLUDED.stock > 0 THEN EXCLUDED.stock ELSE products.stock END,
		  minimum_stock  = EXCLUDED.minimum_stock,
		  purchase_price = CASE WHEN EXCLUDED.purchase_price > 0 THEN EXCLUDED.purchase_price ELSE products.purchase_price END,
		  selling_price  = CASE WHEN EXCLUDED.selling_price > 0 THEN EXCLUDED.selling_price ELSE products.selling_price END,
		  unit           = CASE WHEN EXCLUDED.unit != '' THEN EXCLUDED.unit ELSE products.unit END,
		  location       = CASE WHEN EXCLUDED.location != '' THEN EXCLUDED.location ELSE products.location END,
		  updated_at     = NOW()
	`

	sku := input.SKU
	if sku == "" {
		// generate SKU sementara dari nama jika tidak ada
		sku = GenerateSKU(input.Name, input.Category)
	}

	_, err := r.db.ExecContext(ctx, q,
		sku,
		input.Name,
		categoryID,
		input.Description,
		input.Stock,
		input.MinimumStock,
		input.PurchasePrice,
		input.SellingPrice,
		input.Unit,
		input.Location,
	)
	if err != nil {
		return fmt.Errorf("upsert product '%s': %w", input.Name, err)
	}
	return nil
}

// Create inserts a new product into the database.
func (r *ProductRepository) Create(ctx context.Context, p *model.Product) error {
	const q = `
		INSERT INTO products (
			sku, name, category_id, description, stock, minimum_stock, reserved,
			location, purchase_price, selling_price, unit, image_url, is_active, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW()
		) RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(
		ctx, q,
		p.SKU, p.Name, p.CategoryID, p.Description, p.Stock, p.MinimumStock, p.Reserved,
		p.Location, p.PurchasePrice, p.SellingPrice, p.Unit, p.ImageURL, p.IsActive,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

// Update modifies an existing product in the database.
func (r *ProductRepository) Update(ctx context.Context, p *model.Product) error {
	const q = `
		UPDATE products SET
			sku            = $1,
			name           = $2,
			category_id    = $3,
			description    = $4,
			stock          = $5,
			minimum_stock  = $6,
			reserved       = $7,
			location       = $8,
			purchase_price = $9,
			selling_price  = $10,
			unit           = $11,
			image_url      = $12,
			is_active      = $13,
			updated_at     = NOW()
		WHERE id = $14`
	res, err := r.db.ExecContext(
		ctx, q,
		p.SKU, p.Name, p.CategoryID, p.Description, p.Stock, p.MinimumStock, p.Reserved,
		p.Location, p.PurchasePrice, p.SellingPrice, p.Unit, p.ImageURL, p.IsActive, p.ID,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("product not found or no change")
	}
	return nil
}

// Delete performs soft-delete on product setting is_active = false.
func (r *ProductRepository) Delete(ctx context.Context, id int64) error {
	const q = `UPDATE products SET is_active = false, updated_at = NOW() WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("product not found")
	}
	return nil
}

// GenerateSKU buat SKU sederhana dari nama dan kategori jika tidak ada
func GenerateSKU(name, category string) string {
	// ambil 3 huruf pertama kategori + 3 huruf pertama nama + timestamp
	catPart := "GEN"
	if len(category) >= 3 {
		catPart = category[:3]
	}
	namePart := "XXX"
	if len(name) >= 3 {
		namePart = name[:3]
	}
	ts := time.Now().Format("0601") // YYMM
	return fmt.Sprintf("%s-%s-%s", catPart, namePart, ts)
}

type ProductFilter struct {
	Q                string
	CategoryID       *int64
	StockStatus      string // "available", "low", "empty"
	IsActive         *bool
	LowStockPriority *bool
	Page             int
	Limit            int
}

func (r *ProductRepository) FindFiltered(ctx context.Context, filter ProductFilter) ([]model.Product, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if filter.Q != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(p.sku ILIKE $%d OR p.name ILIKE $%d OR p.location ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+filter.Q+"%")
		argIdx++
	}

	if filter.CategoryID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("p.category_id = $%d", argIdx))
		args = append(args, *filter.CategoryID)
		argIdx++
	}

	if filter.IsActive != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("p.is_active = $%d", argIdx))
		args = append(args, *filter.IsActive)
		argIdx++
	}

	switch filter.StockStatus {
	case "available":
		whereClauses = append(whereClauses, "(p.stock - p.reserved) > p.minimum_stock")
	case "low":
		whereClauses = append(whereClauses, "(p.stock - p.reserved) > 0 AND (p.stock - p.reserved) <= p.minimum_stock")
	case "empty":
		whereClauses = append(whereClauses, "(p.stock - p.reserved) <= 0")
	}

	whereStmt := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) 
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE %s`, whereStmt)

	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("FindFiltered count: %w", err)
	}

	orderBy := "p.id DESC"
	if filter.LowStockPriority != nil {
		if *filter.LowStockPriority {
			// true: Prioritaskan barang low stock ((stock - reserved) <= minimum_stock) di posisi paling atas untuk restock
			orderBy = "CASE WHEN (p.stock - p.reserved) <= p.minimum_stock THEN 0 ELSE 1 END ASC, (p.stock - p.reserved) ASC, p.id DESC"
		} else {
			// false: Prioritaskan barang yang available ((stock - reserved) > 0) di posisi paling atas untuk POS
			orderBy = "CASE WHEN (p.stock - p.reserved) > 0 THEN 0 ELSE 1 END ASC, (p.stock - p.reserved) DESC, p.id DESC"
		}
	}

	selectQuery := fmt.Sprintf(`
		SELECT p.id, p.sku, p.name, p.category_id, COALESCE(c.name, '') AS category_name, p.description, p.stock, p.minimum_stock, p.reserved,
		       p.location, p.purchase_price, p.selling_price, p.unit, p.image_url, p.is_active, p.created_at, p.updated_at
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE %s
		ORDER BY %s`, whereStmt, orderBy)

	if filter.Limit > 0 {
		if filter.Page <= 0 {
			filter.Page = 1
		}
		offset := (filter.Page - 1) * filter.Limit
		selectQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
		args = append(args, filter.Limit, offset)
	}

	var products []model.Product
	if err := r.db.SelectContext(ctx, &products, selectQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("FindFiltered select: %w", err)
	}

	return products, total, nil
}
