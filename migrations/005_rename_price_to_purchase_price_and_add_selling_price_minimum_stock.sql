-- 005_rename_price_to_purchase_price_and_add_selling_price_minimum_stock.sql

-- Rename kolom price -> purchase_price (jika kolom price masih ada)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name='products' AND column_name='price'
    ) THEN
        ALTER TABLE products RENAME COLUMN price TO purchase_price;
    END IF;
END $$;

-- Rename kolom min_stock -> minimum_stock (jika min_stock sudah ada)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name='products' AND column_name='min_stock'
    ) THEN
        ALTER TABLE products RENAME COLUMN min_stock TO minimum_stock;
    END IF;
END $$;

-- Tambahkan kolom purchase_price jika belum ada (fallback)
ALTER TABLE products ADD COLUMN IF NOT EXISTS purchase_price NUMERIC(15, 2) NOT NULL DEFAULT 0.00;

-- Tambahkan kolom selling_price jika belum ada
ALTER TABLE products ADD COLUMN IF NOT EXISTS selling_price NUMERIC(15, 2) NOT NULL DEFAULT 0.00;

-- Tambahkan kolom minimum_stock jika belum ada
ALTER TABLE products ADD COLUMN IF NOT EXISTS minimum_stock INT NOT NULL DEFAULT 0;
