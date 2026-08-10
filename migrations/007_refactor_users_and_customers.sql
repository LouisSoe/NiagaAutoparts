-- 007_refactor_users_and_customers.sql

-- 1. Hapus constraint unik username lama jika ada & hapus kolom username
ALTER TABLE users DROP CONSTRAINT IF EXISTS uq_users_username;
ALTER TABLE users DROP COLUMN IF EXISTS username CASCADE;

-- Pastikan email unik & NOT NULL pada tabel users
ALTER TABLE users ALTER COLUMN email SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT uq_users_email UNIQUE (email);

-- 2. Modifikasi tabel customers: Tambahkan user_id yang merujuk ke users.id
ALTER TABLE customers ADD COLUMN IF NOT EXISTS user_id BIGINT UNIQUE REFERENCES users(id) ON DELETE CASCADE;

-- Hapus constraint unik phone lama pada customers jika ada
ALTER TABLE customers DROP CONSTRAINT IF EXISTS uq_customers_phone;

-- Hapus kolom name, phone_number, dan email yang redundan pada tabel customers
ALTER TABLE customers DROP COLUMN IF EXISTS name;
ALTER TABLE customers DROP COLUMN IF EXISTS phone_number;
ALTER TABLE customers DROP COLUMN IF EXISTS email;
