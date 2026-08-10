-- ============================================================
-- Migration 004: Refactor Orders into Header & Details
-- Add customer_id, user_id, source, payment_method to orders
-- Create order_details table
-- PostgreSQL 12+
-- ============================================================

-- 1. Create order_details table first
CREATE TABLE IF NOT EXISTS order_details (
    id           BIGSERIAL           PRIMARY KEY,
    order_id     BIGINT              NOT NULL,
    product_id   BIGINT              NOT NULL,
    product_name VARCHAR(255)        NOT NULL,  -- Snapshot at order time
    quantity     INT                 NOT NULL DEFAULT 1,
    unit_price   NUMERIC(15,2)       NOT NULL,
    subtotal     NUMERIC(15,2)       NOT NULL,
    created_at   TIMESTAMPTZ         NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_order_details_order_id   ON order_details (order_id);
CREATE INDEX IF NOT EXISTS idx_order_details_product_id ON order_details (product_id);

-- Add Foreign Keys for order_details
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'fk_order_details_order' 
          AND table_name = 'order_details'
    ) THEN
        ALTER TABLE order_details
            ADD CONSTRAINT fk_order_details_order
            FOREIGN KEY (order_id) REFERENCES orders(id)
            ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'fk_order_details_product' 
          AND table_name = 'order_details'
    ) THEN
        ALTER TABLE order_details
            ADD CONSTRAINT fk_order_details_product
            FOREIGN KEY (product_id) REFERENCES products(id)
            ON UPDATE CASCADE;
    END IF;
END $$;

-- 2. Add new columns to orders (Header)
ALTER TABLE orders 
    ADD COLUMN IF NOT EXISTS customer_id BIGINT,
    ADD COLUMN IF NOT EXISTS user_id BIGINT,
    ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'wa',
    ADD COLUMN IF NOT EXISTS payment_method VARCHAR(50);

-- Foreign Keys for orders (customer_id & user_id)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'fk_orders_customer' 
          AND table_name = 'orders'
    ) THEN
        ALTER TABLE orders
            ADD CONSTRAINT fk_orders_customer
            FOREIGN KEY (customer_id) REFERENCES customers(id)
            ON DELETE SET NULL ON UPDATE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'fk_orders_user' 
          AND table_name = 'orders'
    ) THEN
        ALTER TABLE orders
            ADD CONSTRAINT fk_orders_user
            FOREIGN KEY (user_id) REFERENCES users(id)
            ON DELETE SET NULL ON UPDATE CASCADE;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_orders_customer_id ON orders (customer_id);
CREATE INDEX IF NOT EXISTS idx_orders_user_id     ON orders (user_id);
CREATE INDEX IF NOT EXISTS idx_orders_source      ON orders (source);

-- 3. Migrate existing item data from orders into order_details (if orders table has product_id column)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'orders' AND column_name = 'product_id'
    ) THEN
        INSERT INTO order_details (order_id, product_id, product_name, quantity, unit_price, subtotal, created_at)
        SELECT id, product_id, product_name, quantity, unit_price, total_price, created_at
        FROM orders;
        
        -- Drop old product foreign key & item columns from orders
        ALTER TABLE orders DROP CONSTRAINT IF EXISTS fk_orders_product;
        ALTER TABLE orders 
            DROP COLUMN IF EXISTS product_id,
            DROP COLUMN IF EXISTS product_name,
            DROP COLUMN IF EXISTS quantity,
            DROP COLUMN IF EXISTS unit_price;
    END IF;
END $$;
