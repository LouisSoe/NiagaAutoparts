-- 006_update_users_role_check_constraint.sql

-- Hapus CHECK constraint lama pada tabel users (jika ada)
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;

-- Tambahkan CHECK constraint baru yang mendukung role admin, staff, manager, customer, dan cashier
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'staff', 'manager', 'customer', 'cashier'));
