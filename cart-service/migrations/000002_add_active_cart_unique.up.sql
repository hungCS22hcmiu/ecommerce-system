CREATE UNIQUE INDEX IF NOT EXISTS idx_carts_active_user
    ON carts(user_id) WHERE status = 'ACTIVE';
