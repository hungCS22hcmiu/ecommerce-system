-- ============================================================
-- Create the 5 logical databases.
-- Table schemas are now managed by per-service migrations:
--   - product-service, order-service: Flyway
--   - cart-service, payment-service:  golang-migrate
--   - user-service:                   GORM AutoMigrate
-- ============================================================

CREATE DATABASE ecommerce_users;
CREATE DATABASE ecommerce_products;
CREATE DATABASE ecommerce_carts;
CREATE DATABASE ecommerce_orders;
CREATE DATABASE ecommerce_payments;

-- Pre-install uuid-ossp in each database so migrations can use uuid_generate_v4().
-- Migrations also run CREATE EXTENSION IF NOT EXISTS, so this is belt-and-suspenders.

\c ecommerce_users
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

\c ecommerce_products
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

\c ecommerce_carts
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

\c ecommerce_orders
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

\c ecommerce_payments
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
