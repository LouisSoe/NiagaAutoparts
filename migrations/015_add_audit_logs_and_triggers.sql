-- Migration: 015_add_audit_logs_and_triggers.sql
-- Description: Create audit_logs table and automatic triggers for INSERT, UPDATE, DELETE on products, orders, and order_details

-- 1. Create audit_logs table to store audit trail data
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    table_name VARCHAR(50) NOT NULL,
    action VARCHAR(10) NOT NULL, -- 'INSERT', 'UPDATE', 'DELETE'
    record_id BIGINT,
    username VARCHAR(100) DEFAULT 'system', -- Username pelaku aksi, atau 'system' jika NULL
    old_data JSONB,
    new_data JSONB,
    changed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Ensure username column exists if table was created previously
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS username VARCHAR(100) DEFAULT 'system';
ALTER TABLE audit_logs DROP COLUMN IF EXISTS user_id;

CREATE INDEX IF NOT EXISTS idx_audit_logs_table_action ON audit_logs(table_name, action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_changed_at ON audit_logs(changed_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_username ON audit_logs(username);

-- 2. Create universal trigger function for audit logging
CREATE OR REPLACE FUNCTION fn_audit_log_trigger()
RETURNS TRIGGER AS $$
DECLARE
    rec_id BIGINT;
    actor_username VARCHAR(100) := 'system';
    old_json JSONB := NULL;
    new_json JSONB := NULL;
BEGIN
    IF (TG_OP = 'DELETE') THEN
        rec_id := OLD.id;
        old_json := to_jsonb(OLD);
        -- Coba ekstrak username dari field username/user_name/created_by jika ada
        IF old_json ? 'username' AND old_json->>'username' IS NOT NULL AND old_json->>'username' != '' THEN
            actor_username := old_json->>'username';
        ELSIF old_json ? 'user_name' AND old_json->>'user_name' IS NOT NULL AND old_json->>'user_name' != '' THEN
            actor_username := old_json->>'user_name';
        END IF;
    ELSIF (TG_OP = 'UPDATE') THEN
        rec_id := NEW.id;
        old_json := to_jsonb(OLD);
        new_json := to_jsonb(NEW);
        IF new_json ? 'username' AND new_json->>'username' IS NOT NULL AND new_json->>'username' != '' THEN
            actor_username := new_json->>'username';
        ELSIF new_json ? 'user_name' AND new_json->>'user_name' IS NOT NULL AND new_json->>'user_name' != '' THEN
            actor_username := new_json->>'user_name';
        ELSIF old_json ? 'username' AND old_json->>'username' IS NOT NULL AND old_json->>'username' != '' THEN
            actor_username := old_json->>'username';
        END IF;
    ELSIF (TG_OP = 'INSERT') THEN
        rec_id := NEW.id;
        new_json := to_jsonb(NEW);
        IF new_json ? 'username' AND new_json->>'username' IS NOT NULL AND new_json->>'username' != '' THEN
            actor_username := new_json->>'username';
        ELSIF new_json ? 'user_name' AND new_json->>'user_name' IS NOT NULL AND new_json->>'user_name' != '' THEN
            actor_username := new_json->>'user_name';
        END IF;
    END IF;

    -- Jika actor_username null atau kosong, fallback ke 'system'
    IF actor_username IS NULL OR actor_username = '' THEN
        actor_username := 'system';
    END IF;

    INSERT INTO audit_logs (table_name, action, record_id, username, old_data, new_data, changed_at)
    VALUES (TG_TABLE_NAME, TG_OP, rec_id, actor_username, old_json, new_json, NOW());

    IF (TG_OP = 'DELETE') THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 3. Attach trigger to products table (Barang)
DROP TRIGGER IF EXISTS trg_audit_products ON products;
CREATE TRIGGER trg_audit_products
AFTER INSERT OR UPDATE OR DELETE ON products
FOR EACH ROW EXECUTE FUNCTION fn_audit_log_trigger();

-- 4. Attach trigger to orders table (Transaksi / Order Header)
DROP TRIGGER IF EXISTS trg_audit_orders ON orders;
CREATE TRIGGER trg_audit_orders
AFTER INSERT OR UPDATE OR DELETE ON orders
FOR EACH ROW EXECUTE FUNCTION fn_audit_log_trigger();

-- 5. Attach trigger to order_details table (Rincian Item Transaksi)
DROP TRIGGER IF EXISTS trg_audit_order_details ON order_details;
CREATE TRIGGER trg_audit_order_details
AFTER INSERT OR UPDATE OR DELETE ON order_details
FOR EACH ROW EXECUTE FUNCTION fn_audit_log_trigger();
