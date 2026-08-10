-- 008_add_type_customer_to_customers.sql

-- Tambahkan kolom type_customer ke tabel customers dengan default 'INDIVIDUAL'
ALTER TABLE customers ADD COLUMN IF NOT EXISTS type_customer VARCHAR(20) NOT NULL DEFAULT 'INDIVIDUAL';

-- Tambahkan CHECK constraint untuk pilihan role tipe customer (INDIVIDUAL, WORKSHOP, COMPANY)
ALTER TABLE customers DROP CONSTRAINT IF EXISTS customers_type_customer_check;
ALTER TABLE customers ADD CONSTRAINT customers_type_customer_check CHECK (type_customer IN ('INDIVIDUAL', 'WORKSHOP', 'COMPANY'));
