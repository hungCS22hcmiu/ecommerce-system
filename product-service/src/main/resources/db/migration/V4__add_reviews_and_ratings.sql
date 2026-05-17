ALTER TABLE products
  ADD COLUMN avg_rating  DECIMAL(3,2),
  ADD COLUMN rating_count INT NOT NULL DEFAULT 0;

CREATE TABLE product_reviews (
  id            BIGSERIAL    PRIMARY KEY,
  product_id    BIGINT       NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  customer_id   UUID         NOT NULL,
  order_item_id UUID         NOT NULL,
  rating        SMALLINT     NOT NULL CHECK (rating BETWEEN 1 AND 5),
  comment       TEXT,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE (order_item_id)
);

CREATE INDEX idx_product_reviews_product  ON product_reviews(product_id);
CREATE INDEX idx_product_reviews_customer ON product_reviews(customer_id);
