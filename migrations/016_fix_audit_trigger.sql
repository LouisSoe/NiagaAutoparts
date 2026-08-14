-- Migration: 016_fix_audit_trigger.sql
-- Description: Fix fn_audit_log_trigger to read app.current_user session variable
--              (set by AuthMiddleware from JWT claims) as the primary username source.
--              This fixes the bug where username was always recorded as 'system'.
--
-- Root cause:
--   1. The old trigger only looked for 'username'/'user_id' columns in the row data,
--      which don't exist in products, orders, or order_details tables.
--   2. The middleware was calling SET LOCAL on a different pool connection than the
--      one executing the DML statement.
--
-- Solution:
--   - Trigger now reads current_setting('app.current_user', true) first.
--   - Middleware will use a dedicated *sql.Conn per request (see middleware.go),
--     ensuring SET and DML run on the same connection.

CREATE OR REPLACE FUNCTION fn_audit_log_trigger()
RETURNS TRIGGER AS $$
DECLARE
    rec_id         BIGINT;
    actor_username VARCHAR(100);
    uid_val        BIGINT  := NULL;
    old_json       JSONB   := NULL;
    new_json       JSONB   := NULL;
BEGIN
    -- ① Primary source: session variable set by AuthMiddleware from JWT claims.
    --    current_setting(..., true) returns NULL instead of error if var is not set.
    actor_username := NULLIF(TRIM(COALESCE(current_setting('app.current_user', true), '')), '');

    -- ② Fallback: inspect the row data for a username/user_id column.
    --    Useful for tables that carry their own actor reference.
    IF actor_username IS NULL THEN
        IF (TG_OP = 'DELETE') THEN
            rec_id   := OLD.id;
            old_json := to_jsonb(OLD);

            IF old_json ? 'username' AND old_json->>'username' IS NOT NULL AND old_json->>'username' != '' THEN
                actor_username := old_json->>'username';
            ELSIF old_json ? 'user_name' AND old_json->>'user_name' IS NOT NULL AND old_json->>'user_name' != '' THEN
                actor_username := old_json->>'user_name';
            ELSIF old_json ? 'user_id' AND old_json->>'user_id' IS NOT NULL AND old_json->>'user_id' != '' THEN
                uid_val := (old_json->>'user_id')::BIGINT;
            END IF;

        ELSIF (TG_OP = 'UPDATE') THEN
            rec_id   := NEW.id;
            old_json := to_jsonb(OLD);
            new_json := to_jsonb(NEW);

            IF new_json ? 'username' AND new_json->>'username' IS NOT NULL AND new_json->>'username' != '' THEN
                actor_username := new_json->>'username';
            ELSIF new_json ? 'user_name' AND new_json->>'user_name' IS NOT NULL AND new_json->>'user_name' != '' THEN
                actor_username := new_json->>'user_name';
            ELSIF new_json ? 'user_id' AND new_json->>'user_id' IS NOT NULL AND new_json->>'user_id' != '' THEN
                uid_val := (new_json->>'user_id')::BIGINT;
            ELSIF old_json ? 'username' AND old_json->>'username' IS NOT NULL AND old_json->>'username' != '' THEN
                actor_username := old_json->>'username';
            ELSIF old_json ? 'user_id' AND old_json->>'user_id' IS NOT NULL AND old_json->>'user_id' != '' THEN
                uid_val := (old_json->>'user_id')::BIGINT;
            END IF;

        ELSIF (TG_OP = 'INSERT') THEN
            rec_id   := NEW.id;
            new_json := to_jsonb(NEW);

            IF new_json ? 'username' AND new_json->>'username' IS NOT NULL AND new_json->>'username' != '' THEN
                actor_username := new_json->>'username';
            ELSIF new_json ? 'user_name' AND new_json->>'user_name' IS NOT NULL AND new_json->>'user_name' != '' THEN
                actor_username := new_json->>'user_name';
            ELSIF new_json ? 'user_id' AND new_json->>'user_id' IS NOT NULL AND new_json->>'user_id' != '' THEN
                uid_val := (new_json->>'user_id')::BIGINT;
            END IF;
        END IF;
    ELSE
        -- Session var was found — still need to populate rec_id and json snapshots.
        IF (TG_OP = 'DELETE') THEN
            rec_id   := OLD.id;
            old_json := to_jsonb(OLD);
        ELSIF (TG_OP = 'UPDATE') THEN
            rec_id   := NEW.id;
            old_json := to_jsonb(OLD);
            new_json := to_jsonb(NEW);
        ELSIF (TG_OP = 'INSERT') THEN
            rec_id   := NEW.id;
            new_json := to_jsonb(NEW);
        END IF;
    END IF;

    -- ③ If we only have a user_id, resolve the name from the users table.
    IF actor_username IS NULL AND uid_val IS NOT NULL THEN
        SELECT COALESCE(NULLIF(TRIM(name), ''), NULLIF(TRIM(email), ''), 'system')
          INTO actor_username
          FROM users
         WHERE id = uid_val;
    END IF;

    -- ④ Ultimate fallback — background workers, webhooks, or unauthenticated calls.
    IF actor_username IS NULL OR TRIM(actor_username) = '' THEN
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
