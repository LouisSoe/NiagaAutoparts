-- 020_add_courier_role_to_users.sql

-- Hapus CHECK constraint lama pada tabel users (jika ada)
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;

-- Tambahkan CHECK constraint baru yang mendukung role admin, staff, manager, customer, cashier, courier, dan guest
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'staff', 'manager', 'customer', 'cashier', 'courier', 'guest'));
