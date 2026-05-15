ALTER TABLE products ADD COLUMN IF NOT EXISTS embedding vector(384);

CREATE INDEX IF NOT EXISTS idx_products_embedding
    ON products
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
