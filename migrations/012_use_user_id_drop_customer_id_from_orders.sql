-- 012_use_user_id_drop_customer_id_from_orders.sql

-- Tambahkan kembali kolom user_id pada tabel orders jika belum ada
ALTER TABLE orders ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;

-- Hapus kolom customer_id dari tabel orders
ALTER TABLE orders DROP COLUMN IF EXISTS customer_id CASCADE;
