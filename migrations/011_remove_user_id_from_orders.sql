-- 011_remove_user_id_from_orders.sql

-- Hapus kolom user_id dari tabel orders karena sudah ada customer_id
ALTER TABLE orders DROP COLUMN IF EXISTS user_id CASCADE;
