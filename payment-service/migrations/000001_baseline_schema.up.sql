-- ============================================================
-- V1: Baseline schema for ecommerce_payments
-- Idempotent: safe on both fresh and pre-existing databases.
-- Mirrors the original init-databases.sql exactly.
-- ============================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'payment_status') THEN
        CREATE TYPE payment_status AS ENUM ('PENDING', 'COMPLETED', 'FAILED', 'REFUNDED');
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'payment_method') THEN
        CREATE TYPE payment_method AS ENUM ('MOCK_CARD', 'MOCK_WALLET');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS payments (
    id                 UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id           UUID           NOT NULL,
    user_id            UUID           NOT NULL,
    amount             DECIMAL(10, 2) NOT NULL CHECK (amount > 0),
    currency           CHAR(3)        NOT NULL DEFAULT 'USD',
    status             payment_status NOT NULL DEFAULT 'PENDING',
    method             payment_method NOT NULL DEFAULT 'MOCK_CARD',
    idempotency_key    VARCHAR(255)   NOT NULL,
    gateway_reference  VARCHAR(255),
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payments_order_id UNIQUE (order_id)
);

CREATE TABLE IF NOT EXISTS payment_history (
    id          BIGSERIAL       PRIMARY KEY,
    payment_id  UUID            NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    old_status  payment_status,
    new_status  payment_status  NOT NULL,
    reason      TEXT,
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_idempotency    ON payments(idempotency_key);
CREATE        INDEX IF NOT EXISTS idx_payments_order_id       ON payments(order_id);
CREATE        INDEX IF NOT EXISTS idx_payments_user_id        ON payments(user_id);
CREATE        INDEX IF NOT EXISTS idx_payments_status         ON payments(status);
CREATE        INDEX IF NOT EXISTS idx_payment_history_payment ON payment_history(payment_id);
