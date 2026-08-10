-- 013_add_telegram_chat_id_to_orders.sql
ALTER TABLE orders ADD COLUMN IF NOT EXISTS telegram_chat_id VARCHAR(50);
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_chat_id VARCHAR(50);

CREATE INDEX IF NOT EXISTS idx_orders_telegram_chat_id ON orders (telegram_chat_id);
CREATE INDEX IF NOT EXISTS idx_users_telegram_chat_id ON users (telegram_chat_id);
