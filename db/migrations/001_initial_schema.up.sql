-- ApexStock Database Schema — Initial Migration
-- Version: 1.0.0
-- Applied to both apexstock_baseline and apexstock_optimized databases

BEGIN;

-- Extensions
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Enum types
CREATE TYPE flash_sale_status AS ENUM ('draft', 'scheduled', 'active', 'ended');
CREATE TYPE order_status AS ENUM ('pending_payment', 'paid', 'payment_failed', 'expired', 'cancelled');
CREATE TYPE payment_status AS ENUM ('pending', 'success', 'failed');
CREATE TYPE payment_method AS ENUM ('bank_transfer', 'simulated');

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Tables
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(255) NOT NULL CONSTRAINT uq_users_email UNIQUE,
    password_hash   VARCHAR(255) NOT NULL,
    name            VARCHAR(100) NOT NULL,
    phone           VARCHAR(20),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ
);

CREATE TABLE products (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL,
    slug            VARCHAR(255) NOT NULL CONSTRAINT uq_products_slug UNIQUE,
    description     TEXT,
    image_url       VARCHAR(500),
    original_price  BIGINT NOT NULL CHECK (original_price > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE flash_sale_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    start_time  TIMESTAMPTZ NOT NULL,
    end_time    TIMESTAMPTZ NOT NULL,
    status      flash_sale_status NOT NULL DEFAULT 'draft',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_flash_sale_time CHECK (end_time > start_time)
);

CREATE TABLE flash_sale_items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID NOT NULL REFERENCES flash_sale_events(id) ON DELETE CASCADE,
    product_id      UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    sale_price      BIGINT NOT NULL CHECK (sale_price > 0),
    total_quantity  INT NOT NULL CHECK (total_quantity > 0),
    sold_quantity   INT NOT NULL DEFAULT 0 CHECK (sold_quantity >= 0),
    max_per_user    INT NOT NULL DEFAULT 1 CHECK (max_per_user > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_sold_lte_total CHECK (sold_quantity <= total_quantity),
    CONSTRAINT uq_flash_sale_items_event_product UNIQUE (event_id, product_id)
);

CREATE TABLE orders (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    idempotency_key VARCHAR(255) NOT NULL,
    status          order_status NOT NULL DEFAULT 'pending_payment',
    total_amount    BIGINT NOT NULL CHECK (total_amount > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_orders_idempotency UNIQUE (user_id, idempotency_key)
);

CREATE TABLE order_items (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    flash_sale_item_id  UUID NOT NULL REFERENCES flash_sale_items(id) ON DELETE RESTRICT,
    product_id          UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity            INT NOT NULL CHECK (quantity > 0),
    unit_price          BIGINT NOT NULL CHECK (unit_price > 0)
);

CREATE TABLE payments (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id                UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    method                  payment_method NOT NULL DEFAULT 'bank_transfer',
    amount                  BIGINT NOT NULL CHECK (amount > 0),
    transaction_ref         VARCHAR(255),
    status                  payment_status NOT NULL DEFAULT 'pending',
    sepay_webhook_payload   JSONB,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ
);

CREATE TABLE idempotency_keys (
    key             VARCHAR(255) NOT NULL,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    response_status INT NOT NULL,
    response_body   JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_idempotency_keys PRIMARY KEY (key, user_id)
);

CREATE TABLE webhook_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID REFERENCES orders(id),
    provider        VARCHAR(50) NOT NULL,
    transaction_ref VARCHAR(255),
    status          VARCHAR(50) NOT NULL,
    payload         JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes (D0.8: regular indexes, no now() in predicates)
CREATE INDEX idx_users_phone ON users (phone) WHERE phone IS NOT NULL;
CREATE INDEX idx_flash_sale_events_active ON flash_sale_events (start_time, end_time) WHERE status IN ('scheduled', 'active');
CREATE INDEX idx_flash_sale_events_created ON flash_sale_events (created_at DESC);
CREATE INDEX idx_flash_sale_items_event ON flash_sale_items (event_id);
CREATE INDEX idx_flash_sale_items_product ON flash_sale_items (product_id);
CREATE INDEX idx_orders_user_created ON orders (user_id, created_at DESC);
CREATE INDEX idx_orders_pending_expires ON orders (expires_at) WHERE status = 'pending_payment';
CREATE INDEX idx_order_items_order ON order_items (order_id);
CREATE INDEX idx_order_items_flash_sale_item ON order_items (flash_sale_item_id);
CREATE INDEX idx_payments_order ON payments (order_id);
CREATE UNIQUE INDEX idx_payments_transaction_ref ON payments (transaction_ref) WHERE transaction_ref IS NOT NULL;
CREATE INDEX idx_payments_pending ON payments (created_at) WHERE status = 'pending';
CREATE INDEX idx_idempotency_keys_expires ON idempotency_keys (expires_at);
CREATE INDEX idx_webhook_logs_order ON webhook_logs (order_id, created_at DESC);

-- Triggers
CREATE TRIGGER set_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_orders_updated_at BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_payments_updated_at BEFORE UPDATE ON payments FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

COMMIT;
