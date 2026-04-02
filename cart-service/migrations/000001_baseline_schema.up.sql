-- ============================================================
-- V1: Baseline schema for ecommerce_carts
-- Idempotent: safe on both fresh and pre-existing databases.
-- Mirrors the original init-databases.sql exactly.
-- ============================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'cart_status') THEN
        CREATE TYPE cart_status AS ENUM ('ACTIVE', 'CHECKED_OUT', 'ABANDONED');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS carts (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID        NOT NULL,
    status      cart_status NOT NULL DEFAULT 'ACTIVE',
    expires_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '30 minutes'),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cart_items (
    id            UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    cart_id       UUID           NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    product_id    BIGINT         NOT NULL,
    product_name  VARCHAR(200)   NOT NULL,
    quantity      INT            NOT NULL CHECK (quantity > 0),
    unit_price    DECIMAL(10, 2) NOT NULL CHECK (unit_price >= 0),
    added_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_carts_user_id     ON carts(user_id);
CREATE INDEX IF NOT EXISTS idx_carts_status      ON carts(status);
CREATE INDEX IF NOT EXISTS idx_carts_expires_at  ON carts(expires_at)
                  WHERE status = 'ACTIVE';
CREATE INDEX IF NOT EXISTS idx_cart_items_cart    ON cart_items(cart_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cart_items_unique ON cart_items(cart_id, product_id);
