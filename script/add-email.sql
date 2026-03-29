ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_verified BOOLEAN NOT NULL DEFAULT FALSE,                                                          
    ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ; 