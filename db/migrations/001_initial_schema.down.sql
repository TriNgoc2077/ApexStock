-- Rollback initial schema migration

BEGIN;

-- Drop triggers
DROP TRIGGER IF EXISTS set_payments_updated_at ON payments;
DROP TRIGGER IF EXISTS set_orders_updated_at ON orders;
DROP TRIGGER IF EXISTS set_users_updated_at ON users;

-- Drop tables in reverse FK dependency order
DROP TABLE IF EXISTS webhook_logs;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS flash_sale_items;
DROP TABLE IF EXISTS flash_sale_events;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS users;

-- Drop function
DROP FUNCTION IF EXISTS trigger_set_updated_at();

-- Drop enums
DROP TYPE IF EXISTS payment_method;
DROP TYPE IF EXISTS payment_status;
DROP TYPE IF EXISTS order_status;
DROP TYPE IF EXISTS flash_sale_status;

COMMIT;
