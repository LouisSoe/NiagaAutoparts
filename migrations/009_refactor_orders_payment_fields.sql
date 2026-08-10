-- 009_refactor_orders_payment_fields.sql

-- 1. Hapus kolom phone_number dari tabel orders
ALTER TABLE orders DROP COLUMN IF EXISTS phone_number CASCADE;

-- 2. Tambahkan kolom amount_paid (bayar berapa) dan change_amount (kembalian berapa)
ALTER TABLE orders ADD COLUMN IF NOT EXISTS amount_paid NUMERIC(15, 2) NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS change_amount NUMERIC(15, 2) NOT NULL DEFAULT 0;
