-- ============================================================
-- V1: Baseline schema for ecommerce_users
-- Idempotent: safe on both fresh and pre-existing databases.
-- Uses gen_random_uuid() — Postgres 13+ built-in, no extension needed.
-- ============================================================

-- uuid-ossp is pre-installed by init-databases.sql; this is belt-and-suspenders
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ── users ──────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id                    UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    email                 VARCHAR(255) NOT NULL,
    password_hash         VARCHAR(255) NOT NULL,
    role                  VARCHAR(50)  NOT NULL DEFAULT 'customer',
    is_locked             BOOLEAN      NOT NULL DEFAULT FALSE,
    failed_login_attempts INT          NOT NULL DEFAULT 0,
    is_verified           BOOLEAN      NOT NULL DEFAULT FALSE,
    verified_at           TIMESTAMPTZ,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email      ON users(email);
CREATE INDEX        IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

-- ── user_profiles (1-to-1 with users) ─────────────────────────────────────────
CREATE TABLE IF NOT EXISTS user_profiles (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    first_name  VARCHAR(100) NOT NULL,
    last_name   VARCHAR(100) NOT NULL,
    phone       VARCHAR(20),
    avatar_url  VARCHAR(500),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_profiles_user_id ON user_profiles(user_id);

-- ── user_addresses (1-to-many with users) ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS user_addresses (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label         VARCHAR(50),
    address_line1 VARCHAR(255) NOT NULL,
    address_line2 VARCHAR(255),
    city          VARCHAR(100) NOT NULL,
    state         VARCHAR(100),
    country       VARCHAR(100) NOT NULL,
    postal_code   VARCHAR(20),
    is_default    BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_addresses_user_id ON user_addresses(user_id);

-- ── auth_tokens (refresh token store — SHA-256 hash only, never raw token) ────
CREATE TABLE IF NOT EXISTS auth_tokens (
    id                 UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash VARCHAR(255) NOT NULL,
    expires_at         TIMESTAMPTZ  NOT NULL,
    revoked            BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX        IF NOT EXISTS idx_auth_tokens_user_id            ON auth_tokens(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_tokens_refresh_token_hash ON auth_tokens(refresh_token_hash);
