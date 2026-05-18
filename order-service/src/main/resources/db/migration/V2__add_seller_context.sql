ALTER TABLE orders ADD COLUMN seller_id UUID;
CREATE INDEX idx_orders_seller_id     ON orders(seller_id);
CREATE INDEX idx_orders_seller_status ON orders(seller_id, status);
