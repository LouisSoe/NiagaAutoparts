-- 010_remove_product_name_from_order_details.sql

-- Hapus kolom redundan product_name dari tabel order_details
ALTER TABLE order_details DROP COLUMN IF EXISTS product_name CASCADE;
