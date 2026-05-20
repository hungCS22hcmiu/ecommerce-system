CREATE TABLE orders_outbox (
    id           BIGSERIAL    PRIMARY KEY,
    order_id     UUID         NOT NULL,
    payload      JSONB        NOT NULL,
    headers      JSONB        NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX idx_orders_outbox_unpublished
    ON orders_outbox(created_at)
    WHERE published_at IS NULL;
