CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE product_status AS ENUM ('ACTIVE', 'INACTIVE', 'DELETED');
-- CREATE TYPE movement_type  AS ENUM ('IN', 'OUT', 'RESERVE', 'RELEASE');

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

-- CREATE TABLE stock_movements (
--     id            BIGSERIAL       PRIMARY KEY,
--     product_id    BIGINT          NOT NULL REFERENCES products(id),
--     type          movement_type   NOT NULL,
--     quantity      INT             NOT NULL CHECK (quantity > 0),
--     reference_id  VARCHAR(255),   -- order_id or other reference
--     reason        TEXT,
--     created_at    TIMESTAMPTZ     NOT NULL DEFAULT NOW()
-- );

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
-- CREATE        INDEX idx_stock_movements_product ON stock_movements(product_id);
-- CREATE        INDEX idx_stock_movements_ref     ON stock_movements(reference_id)
--                   WHERE reference_id IS NOT NULL;
