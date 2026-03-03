-- ============================================================
-- STEP 1: Create 5 logical databases
-- Run as superuser (postgres)
-- ============================================================

CREATE DATABASE ecommerce_users;
CREATE DATABASE ecommerce_products;
CREATE DATABASE ecommerce_carts;
CREATE DATABASE ecommerce_orders;
CREATE DATABASE ecommerce_payments;

-- ============================================================
-- STEP 2: ecommerce_users schema
-- Connect: \c ecommerce_users
-- ============================================================

\c ecommerce_users

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email                 VARCHAR(255) NOT NULL,
    password_hash         VARCHAR(255) NOT NULL,
    role                  VARCHAR(20)  NOT NULL DEFAULT 'CUSTOMER'
                              CHECK (role IN ('ADMIN', 'SELLER', 'CUSTOMER')),
    is_locked             BOOLEAN      NOT NULL DEFAULT FALSE,
    failed_login_attempts INT          NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE user_profiles (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    first_name  VARCHAR(100) NOT NULL,
    last_name   VARCHAR(100) NOT NULL,
    phone       VARCHAR(20),
    avatar_url  TEXT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_user_profiles_user_id UNIQUE (user_id)  -- enforces 1-to-1
);

CREATE TABLE user_addresses (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    street      VARCHAR(255) NOT NULL,
    city        VARCHAR(100) NOT NULL,
    state       VARCHAR(100),
    zip         VARCHAR(20)  NOT NULL,
    country     VARCHAR(100) NOT NULL,
    is_default  BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE auth_tokens (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id             UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash  VARCHAR(255) NOT NULL,
    expires_at          TIMESTAMPTZ  NOT NULL,
    revoked             BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Indexes: ecommerce_users
CREATE UNIQUE INDEX idx_users_email          ON users(email);
CREATE        INDEX idx_user_profiles_user   ON user_profiles(user_id);
CREATE        INDEX idx_user_addresses_user  ON user_addresses(user_id);
CREATE        INDEX idx_auth_tokens_user     ON auth_tokens(user_id);
CREATE        INDEX idx_auth_tokens_expiry   ON auth_tokens(expires_at)
                  WHERE revoked = FALSE;   -- partial: only active tokens


-- ============================================================
-- STEP 3: ecommerce_products schema
-- Connect: \c ecommerce_products
-- ============================================================

\c ecommerce_products

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE product_status AS ENUM ('ACTIVE', 'INACTIVE', 'DELETED');
CREATE TYPE movement_type  AS ENUM ('IN', 'OUT', 'RESERVE', 'RELEASE');

CREATE TABLE categories (
    id          BIGSERIAL    PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    slug        VARCHAR(100) NOT NULL,
    parent_id   BIGINT       REFERENCES categories(id) ON DELETE SET NULL,
    sort_order  INT          NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE products (
    id               BIGSERIAL       PRIMARY KEY,
    name             VARCHAR(200)    NOT NULL,
    description      TEXT,
    price            DECIMAL(10, 2)  NOT NULL CHECK (price >= 0),
    category_id      BIGINT          REFERENCES categories(id) ON DELETE SET NULL,
    seller_id        UUID            NOT NULL,     -- FK to ecommerce_users.users.id (cross-DB ref — enforced at app level)
    status           product_status  NOT NULL DEFAULT 'ACTIVE',
    stock_quantity   INT             NOT NULL DEFAULT 0 CHECK (stock_quantity >= 0),
    stock_reserved   INT             NOT NULL DEFAULT 0 CHECK (stock_reserved >= 0),
    version          BIGINT          NOT NULL DEFAULT 0,   -- optimistic locking (@Version)
    created_at       TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE TABLE product_images (
    id          BIGSERIAL    PRIMARY KEY,
    product_id  BIGINT       NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    url         TEXT         NOT NULL,
    alt_text    VARCHAR(255),
    sort_order  INT          NOT NULL DEFAULT 0
);

CREATE TABLE stock_movements (
    id            BIGSERIAL       PRIMARY KEY,
    product_id    BIGINT          NOT NULL REFERENCES products(id),
    type          movement_type   NOT NULL,
    quantity      INT             NOT NULL CHECK (quantity > 0),
    reference_id  VARCHAR(255),   -- order_id or other reference
    reason        TEXT,
    created_at    TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

-- Indexes: ecommerce_products
CREATE        INDEX idx_products_category       ON products(category_id);
CREATE        INDEX idx_products_seller         ON products(seller_id);
CREATE        INDEX idx_products_created_at     ON products(created_at DESC);
CREATE        INDEX idx_products_status         ON products(status);

-- Partial index — only index ACTIVE products (smaller, faster for catalog queries)
CREATE        INDEX idx_products_active_cat     ON products(category_id, created_at DESC)
                  WHERE status = 'ACTIVE';

-- GIN full-text search index
CREATE        INDEX idx_products_fts            ON products
                  USING GIN (to_tsvector('english', name || ' ' || COALESCE(description, '')));

CREATE UNIQUE INDEX idx_categories_slug         ON categories(slug);
CREATE        INDEX idx_categories_parent       ON categories(parent_id);
CREATE        INDEX idx_product_images_product  ON product_images(product_id);
CREATE        INDEX idx_stock_movements_product ON stock_movements(product_id);
CREATE        INDEX idx_stock_movements_ref     ON stock_movements(reference_id)
                  WHERE reference_id IS NOT NULL;


-- ============================================================
-- STEP 4: ecommerce_carts schema
-- Connect: \c ecommerce_carts
-- ============================================================

\c ecommerce_carts

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE cart_status AS ENUM ('ACTIVE', 'CHECKED_OUT', 'ABANDONED');

CREATE TABLE carts (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID        NOT NULL,   -- FK to ecommerce_users (cross-DB — app enforced)
    status      cart_status NOT NULL DEFAULT 'ACTIVE',
    expires_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '30 minutes'),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE cart_items (
    id            UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    cart_id       UUID           NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    product_id    BIGINT         NOT NULL,   -- FK to ecommerce_products (cross-DB — app enforced)
    product_name  VARCHAR(200)   NOT NULL,   -- denormalized snapshot to avoid cross-service join
    quantity      INT            NOT NULL CHECK (quantity > 0),
    unit_price    DECIMAL(10, 2) NOT NULL CHECK (unit_price >= 0),
    added_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

-- Indexes: ecommerce_carts
CREATE        INDEX idx_carts_user_id     ON carts(user_id);
CREATE        INDEX idx_carts_status      ON carts(status);
CREATE        INDEX idx_carts_expires_at  ON carts(expires_at)
                  WHERE status = 'ACTIVE';  -- partial: only active carts expire
CREATE        INDEX idx_cart_items_cart   ON cart_items(cart_id);
CREATE UNIQUE INDEX idx_cart_items_unique ON cart_items(cart_id, product_id);  -- one row per product per cart


-- ============================================================
-- STEP 5: ecommerce_orders schema
-- Connect: \c ecommerce_orders
-- ============================================================

\c ecommerce_orders

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE order_status        AS ENUM ('PENDING', 'CONFIRMED', 'CANCELLED', 'SHIPPED', 'DELIVERED');
CREATE TYPE notification_type   AS ENUM ('EMAIL', 'SMS');
CREATE TYPE notification_status AS ENUM ('SENT', 'FAILED', 'PENDING');

CREATE TABLE orders (
    id                UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id           UUID           NOT NULL,   -- cross-DB reference
    cart_id           UUID,                      -- cross-DB reference (nullable after checkout)
    total_amount      DECIMAL(10, 2) NOT NULL CHECK (total_amount >= 0),
    status            order_status   NOT NULL DEFAULT 'PENDING',
    shipping_address  JSONB          NOT NULL,   -- snapshot: {street, city, state, zip, country}
    created_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE TABLE order_items (
    id            UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id      UUID           NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id    BIGINT         NOT NULL,   -- cross-DB reference
    product_name  VARCHAR(200)   NOT NULL,   -- denormalized snapshot
    quantity      INT            NOT NULL CHECK (quantity > 0),
    unit_price    DECIMAL(10, 2) NOT NULL CHECK (unit_price >= 0)
);

CREATE TABLE order_status_history (
    id          BIGSERIAL    PRIMARY KEY,
    order_id    UUID         NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    old_status  order_status,                -- NULL for initial PENDING creation
    new_status  order_status NOT NULL,
    reason      TEXT,
    changed_by  VARCHAR(100),               -- userId or "system" or "kafka-consumer"
    changed_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE notifications (
    id        UUID                NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id  UUID                NOT NULL REFERENCES orders(id),
    user_id   UUID                NOT NULL,
    type      notification_type   NOT NULL,
    channel   VARCHAR(100),                 -- email address or phone number
    subject   VARCHAR(255),
    body      TEXT,
    status    notification_status NOT NULL DEFAULT 'PENDING',
    sent_at   TIMESTAMPTZ,
    created_at TIMESTAMPTZ        NOT NULL DEFAULT NOW()
);

-- Indexes: ecommerce_orders
CREATE INDEX idx_orders_user_id        ON orders(user_id);
CREATE INDEX idx_orders_status         ON orders(status);
CREATE INDEX idx_orders_created_at     ON orders(created_at DESC);
CREATE INDEX idx_order_items_order     ON order_items(order_id);
CREATE INDEX idx_order_items_product   ON order_items(product_id);
CREATE INDEX idx_order_history_order   ON order_status_history(order_id);
CREATE INDEX idx_notifications_order   ON notifications(order_id);
CREATE INDEX idx_notifications_user    ON notifications(user_id);
CREATE INDEX idx_notifications_status  ON notifications(status)
                WHERE status = 'PENDING';   -- partial: only unsent notifications


-- ============================================================
-- STEP 6: ecommerce_payments schema
-- Connect: \c ecommerce_payments
-- ============================================================

\c ecommerce_payments

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE payment_status AS ENUM ('PENDING', 'COMPLETED', 'FAILED', 'REFUNDED');
CREATE TYPE payment_method AS ENUM ('MOCK_CARD', 'MOCK_WALLET');

CREATE TABLE payments (
    id                 UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id           UUID           NOT NULL,   -- cross-DB reference
    user_id            UUID           NOT NULL,   -- cross-DB reference
    amount             DECIMAL(10, 2) NOT NULL CHECK (amount > 0),
    currency           CHAR(3)        NOT NULL DEFAULT 'USD',
    status             payment_status NOT NULL DEFAULT 'PENDING',
    method             payment_method NOT NULL DEFAULT 'MOCK_CARD',
    idempotency_key    VARCHAR(255)   NOT NULL,   -- unique constraint is the safety net
    gateway_reference  VARCHAR(255),              -- returned by mock gateway on success
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payments_order_id UNIQUE (order_id)  -- enforces 1 payment per order
);

CREATE TABLE payment_history (
    id          BIGSERIAL       PRIMARY KEY,
    payment_id  UUID            NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    old_status  payment_status,
    new_status  payment_status  NOT NULL,
    reason      TEXT,
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

-- Indexes: ecommerce_payments
CREATE UNIQUE INDEX idx_payments_idempotency   ON payments(idempotency_key);   -- CRITICAL: prevents double charge
CREATE        INDEX idx_payments_order_id      ON payments(order_id);
CREATE        INDEX idx_payments_user_id       ON payments(user_id);
CREATE        INDEX idx_payments_status        ON payments(status);
CREATE        INDEX idx_payment_history_payment ON payment_history(payment_id);