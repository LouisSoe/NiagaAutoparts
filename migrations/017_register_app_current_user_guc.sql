-- Migration: 017_register_app_current_user_guc.sql
-- Description: Register app.current_user as a known database-level GUC parameter.
--
-- Root cause of username always being 'system':
--   In PostgreSQL 13 and earlier, custom GUC parameters (like app.current_user)
--   must be pre-registered before SET LOCAL can be used. Without registration,
--   SET LOCAL app.current_user = 'value' throws an error which aborts the
--   current transaction, causing all subsequent DML to fail with 25P02.
--
--   In PostgreSQL 14+, any 'namespace.parameter' GUC is auto-accepted, but
--   registering at the database level is still best practice.
--
-- Fix: Use ALTER DATABASE to set a default value for the GUC. This registers
--   the parameter as known, making SET LOCAL work reliably in all PG versions.
--   The default value 'system' means unauthenticated operations correctly
--   show 'system' in audit logs.
--
-- HOW TO APPLY: Run this file in psql, DBeaver, pgAdmin, or any SQL client
--   connected to the autoparts_db database, then RECONNECT all clients so
--   the new database-level default takes effect.

DO $$
DECLARE
    db_name TEXT := current_database();
BEGIN
    -- Register app.current_user at the database level with default = 'system'.
    -- After this, SET LOCAL app.current_user = 'username' will work in all
    -- PostgreSQL versions without requiring changes to postgresql.conf.
    EXECUTE format(
        'ALTER DATABASE %I SET "app.current_user" TO %L',
        db_name,
        'system'
    );

    RAISE NOTICE 'Registered app.current_user GUC for database: %', db_name;
END $$;

-- Reload configuration to apply the change immediately for new connections.
SELECT pg_reload_conf();

-- Verify the registration worked by checking pg_db_role_setting.
SELECT
    d.datname AS database,
    rs.setconfig AS settings
FROM pg_db_role_setting rs
JOIN pg_database d ON d.oid = rs.setdatabase
WHERE d.datname = current_database()
  AND rs.setrole = 0  -- database-wide (not role-specific)
  AND EXISTS (
      SELECT 1 FROM unnest(rs.setconfig) s WHERE s LIKE 'app.current_user%'
  );
