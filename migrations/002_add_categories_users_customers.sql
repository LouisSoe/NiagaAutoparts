-- ============================================================
-- Migration 002: Add Categories, Users, Customers tables
-- Link Categories to Products
-- PostgreSQL 12+
-- ============================================================

-- ─────────────────────────────────────────────────────────────
-- 1. categories
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS categories (
    id          BIGSERIAL           PRIMARY KEY,
    name        VARCHAR(100)        NOT NULL,
    slug        VARCHAR(100)        NOT NULL DEFAULT '',
    description TEXT,
    created_at  TIMESTAMPTZ         NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ         NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_categories_name UNIQUE (name)
);

CREATE INDEX IF NOT EXISTS idx_categories_slug ON categories (slug);

-- ─────────────────────────────────────────────────────────────
-- 2. users (Internal system / admin / staff users)
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL           PRIMARY KEY,
    username      VARCHAR(50)         NOT NULL,
    email         VARCHAR(100),
    password_hash VARCHAR(255)        NOT NULL,
    name          VARCHAR(100)        NOT NULL,
    role          VARCHAR(20)         NOT NULL DEFAULT 'staff'
                      CHECK (role IN ('admin', 'staff', 'manager')),
    phone         VARCHAR(20),
    is_active     BOOLEAN             NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ         NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ         NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_users_username UNIQUE (username),
    CONSTRAINT uq_users_email UNIQUE (email)
);

CREATE INDEX IF NOT EXISTS idx_users_role      ON users (role);
CREATE INDEX IF NOT EXISTS idx_users_is_active ON users (is_active);

-- ─────────────────────────────────────────────────────────────
-- 3. customers (WhatsApp & store customers)
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS customers (
    id           BIGSERIAL           PRIMARY KEY,
    name         VARCHAR(100)        NOT NULL,
    phone_number VARCHAR(20)         NOT NULL,
    email        VARCHAR(100),
    address      TEXT,
    notes        TEXT,
    created_at   TIMESTAMPTZ         NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ         NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_customers_phone UNIQUE (phone_number)
);

CREATE INDEX IF NOT EXISTS idx_customers_phone ON customers (phone_number);

-- ─────────────────────────────────────────────────────────────
-- 4. Connect categories to products
-- ─────────────────────────────────────────────────────────────
ALTER TABLE products 
    ADD COLUMN IF NOT EXISTS category_id BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'fk_products_category' 
          AND table_name = 'products'
    ) THEN
        ALTER TABLE products
            ADD CONSTRAINT fk_products_category
            FOREIGN KEY (category_id) REFERENCES categories(id)
            ON DELETE SET NULL ON UPDATE CASCADE;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_products_category_id ON products (category_id);

-- Auto-triggers for updated_at
CREATE OR REPLACE TRIGGER trg_categories_updated_at
    BEFORE UPDATE ON categories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE OR REPLACE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE OR REPLACE TRIGGER trg_customers_updated_at
    BEFORE UPDATE ON customers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Populate initial categories from existing products table
INSERT INTO categories (name, slug)
SELECT DISTINCT category, LOWER(REPLACE(category, ' ', '-'))
FROM products
WHERE category IS NOT NULL AND category != ''
ON CONFLICT (name) DO NOTHING;

-- Backfill category_id in products table
UPDATE products p
SET category_id = c.id
FROM categories c
WHERE p.category = c.name AND (p.category_id IS NULL);
