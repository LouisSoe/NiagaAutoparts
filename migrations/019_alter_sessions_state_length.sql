-- 019_alter_sessions_state_length.sql
-- Widen state column to VARCHAR(60) to accommodate longer state names
ALTER TABLE sessions ALTER COLUMN state TYPE TEXT;
