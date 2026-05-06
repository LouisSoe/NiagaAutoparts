-- ============================================================
-- WhatsApp AutoParts Chatbot - Database Schema (PostgreSQL)
-- PostgreSQL 12+
-- ============================================================

-- ─────────────────────────────────────────────────────────────
-- Trigram extension — wajib untuk fuzzy search pada products
-- ─────────────────────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ─────────────────────────────────────────────────────────────
-- products
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS products (
    id          BIGSERIAL           PRIMARY KEY,
    sku         VARCHAR(50)         NOT NULL,
    name        VARCHAR(255)        NOT NULL,
    category    VARCHAR(100)        NOT NULL DEFAULT '',
    description TEXT,
    stock       INT                 NOT NULL DEFAULT 0,
    reserved    INT                 NOT NULL DEFAULT 0,   -- stock held by pending orders
    location    VARCHAR(100)        NOT NULL DEFAULT '',  -- e.g. 'Rak A3-B2'
    price       NUMERIC(15,2)       NOT NULL DEFAULT 0.00,
    unit        VARCHAR(20)         NOT NULL DEFAULT 'pcs',
    image_url   VARCHAR(500),
    is_active   BOOLEAN             NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ         NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ         NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_products_sku UNIQUE (sku)
);

-- Regular B-tree indexes
CREATE INDEX IF NOT EXISTS idx_products_category  ON products (category);
CREATE INDEX IF NOT EXISTS idx_products_is_active ON products (is_active);

-- GIN trigram indexes — digunakan oleh operator % (similarity) di Search query
-- Mencakup name, sku, dan category sesuai requirement
CREATE INDEX IF NOT EXISTS idx_products_trgm_name     ON products USING GIN (name     gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_products_trgm_sku      ON products USING GIN (sku      gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_products_trgm_category ON products USING GIN (category gin_trgm_ops);

-- ─────────────────────────────────────────────────────────────
-- orders
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS orders (
    id           BIGSERIAL           PRIMARY KEY,
    order_number VARCHAR(30)         NOT NULL,
    phone_number VARCHAR(20)         NOT NULL,
    product_id   BIGINT              NOT NULL,
    product_name VARCHAR(255)        NOT NULL,  -- snapshot at order time
    quantity     INT                 NOT NULL DEFAULT 1,
    unit_price   NUMERIC(15,2)       NOT NULL,
    total_price  NUMERIC(15,2)       NOT NULL,
    status       VARCHAR(20)         NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','reserved','paid','cancelled')),
    notes        TEXT,
    expires_at   TIMESTAMPTZ,                  -- reservation expiry (NULL = no expiry)
    created_at   TIMESTAMPTZ         NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ         NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_orders_number UNIQUE (order_number),
    CONSTRAINT fk_orders_product FOREIGN KEY (product_id)
        REFERENCES products(id) ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_orders_phone   ON orders (phone_number);
CREATE INDEX IF NOT EXISTS idx_orders_product ON orders (product_id);
CREATE INDEX IF NOT EXISTS idx_orders_status  ON orders (status);
CREATE INDEX IF NOT EXISTS idx_orders_expires ON orders (expires_at);

-- ─────────────────────────────────────────────────────────────
-- sessions  (one row per phone number, upserted on each message)
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS sessions (
    id                BIGSERIAL           PRIMARY KEY,
    phone_number      VARCHAR(20)         NOT NULL,
    state             VARCHAR(30)         NOT NULL DEFAULT 'idle',
    last_intent       VARCHAR(50)         NOT NULL DEFAULT '',
    last_product_id   BIGINT,
    last_product_name VARCHAR(255)        NOT NULL DEFAULT '',
    pending_order_id  BIGINT,
    context           TEXT,               -- JSON blob for extra state
    updated_at        TIMESTAMPTZ         NOT NULL DEFAULT NOW(),
    expires_at        TIMESTAMPTZ         NOT NULL,

    CONSTRAINT uq_sessions_phone UNIQUE (phone_number)
);

CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions (expires_at);

-- ─────────────────────────────────────────────────────────────
-- price_references  (marketplace comparison data)
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS price_references (
    id           BIGSERIAL           PRIMARY KEY,
    product_id   BIGINT              NOT NULL,
    marketplace  VARCHAR(50)         NOT NULL,   -- tokopedia, shopee, lazada, etc.
    price        NUMERIC(15,2)       NOT NULL,
    url          VARCHAR(500)        NOT NULL DEFAULT '',
    fetched_at   TIMESTAMPTZ         NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_pref_product FOREIGN KEY (product_id)
        REFERENCES products(id) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_pref_product ON price_references (product_id);
CREATE INDEX IF NOT EXISTS idx_pref_fetched ON price_references (fetched_at);

-- ─────────────────────────────────────────────────────────────
-- Trigger: auto-update updated_at on row change
-- ─────────────────────────────────────────────────────────────
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE OR REPLACE TRIGGER trg_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE OR REPLACE TRIGGER trg_sessions_updated_at
    BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ─────────────────────────────────────────────────────────────
-- Seed data (sample products — sama dengan MySQL migration)
-- ─────────────────────────────────────────────────────────────
INSERT INTO products (sku, name, category, description, stock, location, price, unit) VALUES
('KR-HB-001',    'Kampas Rem Honda Beat Depan',    'Rem',         'OEM kompatibel, bahan semi-metallic',         15, 'Rak A1-B1', 35000.00,  'pcs'),
('KR-HB-002',    'Kampas Rem Honda Beat Belakang',  'Rem',         'OEM kompatibel',                              12, 'Rak A1-B2', 28000.00,  'pcs'),
('FO-AHM-001',   'Filter Oli Honda Beat/Vario',     'Filter',      'Filter oli orisinil AHM',                    30, 'Rak B2-C3', 22000.00,  'pcs'),
('FU-UNI-001',   'Filter Udara Universal',          'Filter',      'Cocok untuk motor 110-150cc',                20, 'Rak B2-D1', 45000.00,  'pcs'),
('AKI-GS-5A',    'Aki Kering GS Astra 5AH',        'Kelistrikan', 'Aki MF maintenance free',                     8, 'Rak C4-A1', 180000.00, 'unit'),
('BUS-NGK-CR6',  'Busi NGK CR6HSA',                 'Kelistrikan', 'Standar pabrik untuk Honda Beat/Vario',      50, 'Rak D1-A2', 25000.00,  'pcs'),
('BUS-DENI-U20', 'Busi Denso U20EPR',               'Kelistrikan', 'Original Denso',                             35, 'Rak D1-A3', 28000.00,  'pcs'),
('OLI-AHM-800',  'Oli Mesin AHM SPX 800ml',         'Pelumas',     '0.8L, 10W-30, JASO MB',                     40, 'Rak E2-B1', 32000.00,  'botol'),
('OLI-CASTROL-1L','Oli Mesin Castrol 1L',           'Pelumas',     '10W-40 untuk motor matic',                   25, 'Rak E2-B2', 55000.00,  'botol'),
('VBL-HB-SET',   'V-Belt Honda Beat/Scoopy Set',    'Transmisi',   'Satu set termasuk roller',                   10, 'Rak F3-C1', 120000.00, 'set')
ON CONFLICT (sku) DO NOTHING;

-- Sample marketplace price references
INSERT INTO price_references (product_id, marketplace, price, url, fetched_at)
SELECT id, 'tokopedia', price * 1.05, 'https://tokopedia.com', NOW()
FROM products
ON CONFLICT DO NOTHING;

INSERT INTO price_references (product_id, marketplace, price, url, fetched_at)
SELECT id, 'shopee', price * 1.03, 'https://shopee.co.id', NOW()
FROM products
ON CONFLICT DO NOTHING;

ALTER TABLE price_references
ADD CONSTRAINT uq_price_ref_product_marketplace
UNIQUE (product_id, marketplace);