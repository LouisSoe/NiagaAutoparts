-- ============================================================
-- Migration 003: Drop category column from products
-- Use category_id exclusively
-- PostgreSQL 12+
-- ============================================================

-- 1. Ensure categories are seeded from any remaining products.category text
INSERT INTO categories (name, slug)
SELECT DISTINCT category, LOWER(REPLACE(category, ' ', '-'))
FROM products
WHERE category IS NOT NULL AND category != ''
ON CONFLICT (name) DO NOTHING;

-- 2. Backfill category_id in products table
UPDATE products p
SET category_id = c.id
FROM categories c
WHERE p.category = c.name AND (p.category_id IS NULL);

-- 3. Drop indexes on category column
DROP INDEX IF EXISTS idx_products_category;
DROP INDEX IF EXISTS idx_products_trgm_category;

-- 4. Drop category column from products table
ALTER TABLE products DROP COLUMN IF EXISTS category;
