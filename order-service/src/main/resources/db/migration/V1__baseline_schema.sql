-- ============================================================
-- V1: Baseline schema for ecommerce_orders
-- Mirrors the original init-databases.sql exactly.
-- ============================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE order_status        AS ENUM ('PENDING', 'CONFIRMED', 'CANCELLED', 'SHIPPED', 'DELIVERED');
CREATE TYPE notification_type   AS ENUM ('EMAIL', 'SMS');
CREATE TYPE notification_status AS ENUM ('SENT', 'FAILED', 'PENDING');

CREATE TABLE orders (
    id                UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id           UUID           NOT NULL,
    cart_id           UUID,
    total_amount      DECIMAL(10, 2) NOT NULL CHECK (total_amount >= 0),
    status            order_status   NOT NULL DEFAULT 'PENDING',
    shipping_address  JSONB          NOT NULL,
    created_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE TABLE order_items (
    id            UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id      UUID           NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id    BIGINT         NOT NULL,
    product_name  VARCHAR(200)   NOT NULL,
    quantity      INT            NOT NULL CHECK (quantity > 0),
    unit_price    DECIMAL(10, 2) NOT NULL CHECK (unit_price >= 0)
);

CREATE TABLE order_status_history (
    id          BIGSERIAL    PRIMARY KEY,
    order_id    UUID         NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    old_status  order_status,
    new_status  order_status NOT NULL,
    reason      TEXT,
    changed_by  VARCHAR(100),
    changed_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE notifications (
    id        UUID                NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id  UUID                NOT NULL REFERENCES orders(id),
    user_id   UUID                NOT NULL,
    type      notification_type   NOT NULL,
    channel   VARCHAR(100),
    subject   VARCHAR(255),
    body      TEXT,
    status    notification_status NOT NULL DEFAULT 'PENDING',
    sent_at   TIMESTAMPTZ,
    created_at TIMESTAMPTZ        NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_orders_user_id        ON orders(user_id);
CREATE INDEX idx_orders_status         ON orders(status);
CREATE INDEX idx_orders_created_at     ON orders(created_at DESC);
CREATE INDEX idx_order_items_order     ON order_items(order_id);
CREATE INDEX idx_order_items_product   ON order_items(product_id);
CREATE INDEX idx_order_history_order   ON order_status_history(order_id);
CREATE INDEX idx_notifications_order   ON notifications(order_id);
CREATE INDEX idx_notifications_user    ON notifications(user_id);
CREATE INDEX idx_notifications_status  ON notifications(status)
                WHERE status = 'PENDING';
