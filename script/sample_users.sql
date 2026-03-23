-- ============================================================
-- Sample users for local development / testing
-- Connect to ecommerce_users before running:
--   \c ecommerce_users
--
-- Passwords (bcrypt cost 12):
--   admin@example.com    →  Admin@123
--   customer@example.com →  Customer@123
--   seller@example.com   →  Seller@123
--
-- All accounts are pre-verified (is_verified = TRUE) so you
-- can log in immediately without going through email verification.
-- ============================================================

\c ecommerce_users

-- ── Users ────────────────────────────────────────────────────────────────────

INSERT INTO users (id, email, password_hash, role, is_verified, verified_at)
VALUES
    ('00000000-0000-0000-0000-000000000001',
     'admin@example.com',
     '$2a$12$tguN3QY5dq0nlabt5FS0U.tDoQUlRnUJFi4zwSnJv/ldgkk8gyKgS',  -- Admin@123
     'admin',
     TRUE, NOW()),

    ('00000000-0000-0000-0000-000000000002',
     'customer@example.com',
     '$2a$12$WHt4CJT1pLm8l.rVAERs.ejuCg9kMkyXqWldsQ9YVy7X/mLQqBB0q',  -- Customer@123
     'customer',
     TRUE, NOW()),

    ('00000000-0000-0000-0000-000000000003',
     'seller@example.com',
     '$2a$12$nPmiGwsqKktEdlBDsy2Ca.fCR3/pee0lvf5ls.3Gu1RKBJN7dVdzG',  -- Seller@123
     'seller',
     TRUE, NOW());

-- ── Profiles ─────────────────────────────────────────────────────────────────

INSERT INTO user_profiles (user_id, first_name, last_name)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'Admin',    'User'),
    ('00000000-0000-0000-0000-000000000002', 'John',     'Doe'),
    ('00000000-0000-0000-0000-000000000003', 'Jane',     'Smith');
