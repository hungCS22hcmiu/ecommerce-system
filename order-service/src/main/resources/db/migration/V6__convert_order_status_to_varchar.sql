-- Drop indexes that reference order_status enum columns before type conversion
DROP INDEX IF EXISTS idx_orders_seller_status;

-- Convert order_status enum columns to varchar so Hibernate can bind string parameters
-- without hitting "operator does not exist: order_status = character varying".
-- Follows the same pattern applied to the notifications table in V3.
ALTER TABLE orders
    ALTER COLUMN status TYPE VARCHAR(50) USING status::text;

ALTER TABLE order_status_history
    ALTER COLUMN old_status TYPE VARCHAR(50) USING old_status::text,
    ALTER COLUMN new_status TYPE VARCHAR(50) USING new_status::text;

-- Recreate the composite index (now works on varchar)
CREATE INDEX idx_orders_seller_status ON orders(seller_id, status);
