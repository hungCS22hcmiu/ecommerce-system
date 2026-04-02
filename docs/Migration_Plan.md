# Database Migration Plan

## Context

All 5 database schemas were originally created by a monolithic `script/init-databases.sql` (307 lines) mounted into the Postgres container's `docker-entrypoint-initdb.d/`. This ran only once (on empty data volume), provided no version tracking, and made schema evolution error-prone.

This plan transitions schema management to per-service migrations so each service owns its own database schema.

**user-service is READ-ONLY** -- its codebase, config, and schema logic were not touched.

---

## Before (Single Init Script)

```
script/init-databases.sql  (307 lines)
  ├── CREATE DATABASE x5
  ├── ecommerce_users schema    ← user-service (GORM AutoMigrate)
  ├── ecommerce_products schema ← product-service
  ├── ecommerce_carts schema    ← cart-service
  ├── ecommerce_orders schema   ← order-service
  └── ecommerce_payments schema ← payment-service
```

- Ran once on first Postgres boot, then never again
- No version tracking, no rollback capability
- All 5 schemas coupled in one file

## After (Per-Service Migrations)

```
script/init-databases.sql  (31 lines — CREATE DATABASE + extensions only)

product-service/src/main/resources/db/migration/
  └── V1__baseline_schema.sql          ← Flyway

order-service/src/main/resources/db/migration/
  └── V1__baseline_schema.sql          ← Flyway

cart-service/migrations/
  ├── 000001_baseline_schema.up.sql    ← golang-migrate
  └── 000001_baseline_schema.down.sql

payment-service/migrations/
  ├── 000001_baseline_schema.up.sql    ← golang-migrate
  └── 000001_baseline_schema.down.sql
```

---

## Tool Selection

| Service | Tool | Rationale |
|---|---|---|
| product-service | **Flyway** | Already in pom.xml, config scaffolded, empty `db/migration/` dir ready. Industry standard for Spring Boot. |
| order-service | **Flyway** | Same as product-service. |
| cart-service | **golang-migrate** | Standard SQL-based migration for Go. No ORM coupling. Matches existing raw-SQL schema style. |
| payment-service | **golang-migrate** | Same as cart-service. |
| user-service | **No change** | READ-ONLY. GORM AutoMigrate stays. |

---

## Existing-vs-Fresh Deploy Strategy

**Problem**: Existing deployments already have schemas from the old init-databases.sql. Fresh deployments need migrations to create them from scratch.

| Tool | Strategy |
|---|---|
| **Flyway** | `baseline-on-migrate: true` -- on non-empty schema, marks V1 as "BASELINE" (skips execution). On empty schema, runs V1 normally. |
| **golang-migrate** | **Idempotent V1** -- uses `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, `DO $$ IF NOT EXISTS` for enums. Safe on both empty and pre-existing schemas. |

### Scenario Matrix

| Scenario | What Happens |
|---|---|
| **Fresh deploy** (empty volume) | init-databases.sql creates DBs + extensions. Flyway runs V1. golang-migrate runs V1. |
| **Existing deploy** (has data) | init-databases.sql skipped (Postgres skips on non-empty data dir). Flyway baselines V1. golang-migrate V1 is idempotent no-op. |
| **`make db-nuke`** (full reset) | Volume wiped, init-databases.sql runs, all V1 migrations run from scratch. |

---

## What Was Implemented

### Phase 1: Slim init-databases.sql

- Archived original to `script/init-databases.full.sql.bak` (307 lines, for reference)
- Slimmed `script/init-databases.sql` to 31 lines: `CREATE DATABASE` x5 + `CREATE EXTENSION IF NOT EXISTS "uuid-ossp"` in each

### Phase 2: product-service (Flyway)

**Created**: `product-service/src/main/resources/db/migration/V1__baseline_schema.sql`
- Exact DDL from init-databases.sql for `ecommerce_products`
- 2 enums (`product_status`, `movement_type`)
- 4 tables (`categories`, `products`, `product_images`, `stock_movements`)
- 11 indexes (including partial index, GIN full-text search)

**Modified**: `product-service/src/main/resources/application.yaml`
```yaml
flyway:
  enabled: true
  locations: classpath:db/migration
  baseline-on-migrate: true
  baseline-version: 1
```

### Phase 3: order-service (Flyway)

**Created**: `order-service/src/main/resources/db/migration/V1__baseline_schema.sql`
- 3 enums (`order_status`, `notification_type`, `notification_status`)
- 4 tables (`orders`, `order_items`, `order_status_history`, `notifications`)
- 9 indexes (including partial index on pending notifications)

**Modified**: `order-service/src/main/resources/application.yaml`
- Same Flyway config as product-service

### Phase 4: cart-service (golang-migrate)

**Added dependency**: `github.com/golang-migrate/migrate/v4` to go.mod

**Created**: `cart-service/migrations/000001_baseline_schema.up.sql` (idempotent)
- 1 enum (`cart_status`)
- 2 tables (`carts`, `cart_items`)
- 5 indexes (including partial index, unique constraint)

**Created**: `cart-service/migrations/000001_baseline_schema.down.sql`
- Drops tables and enum in reverse order

**Modified**: `cart-service/cmd/server/main.go`
- Added `migrate.Up()` call after DB connection, before Redis connection

**Modified**: `cart-service/Dockerfile`
- Added `COPY --from=builder /app/migrations ./migrations`

### Phase 5: payment-service (golang-migrate)

**Added dependency**: `github.com/golang-migrate/migrate/v4` to go.mod

**Created**: `payment-service/migrations/000001_baseline_schema.up.sql` (idempotent)
- 2 enums (`payment_status`, `payment_method`)
- 2 tables (`payments`, `payment_history`)
- 5 indexes (including unique idempotency key)

**Created**: `payment-service/migrations/000001_baseline_schema.down.sql`

**Modified**: `payment-service/cmd/server/main.go` and `Dockerfile`
- Same pattern as cart-service

---

## What Was NOT Touched

- **user-service**: Entire codebase is read-only. GORM AutoMigrate continues to manage `ecommerce_users` schema.
- **docker-compose.yml**: No changes. Init script mount stays (still creates databases). All service definitions unchanged.
- **No schema changes**: Every V1 migration reproduces the original init SQL exactly -- no additions, no renames, no type changes.

---

## Verification Results (Existing Deploy)

Tested on a running Postgres instance that already had all schemas from the old init-databases.sql:

```
=== product-service Flyway ===
flyway_schema_history: version=1, type=BASELINE, success=true
Tables: categories, products, product_images, stock_movements, flyway_schema_history

=== Idempotency check ===
Restart: "Schema is up to date. No migration necessary."

=== user-service (unaffected) ===
Tables: users, user_profiles, user_addresses, auth_tokens (unchanged)
```

---

## Files Changed

| File | Action |
|---|---|
| `script/init-databases.sql` | Slimmed to CREATE DATABASE + extensions |
| `script/init-databases.full.sql.bak` | Archived original (307 lines) |
| `product-service/src/main/resources/db/migration/V1__baseline_schema.sql` | Created |
| `product-service/src/main/resources/application.yaml` | Enabled Flyway |
| `order-service/src/main/resources/db/migration/V1__baseline_schema.sql` | Created |
| `order-service/src/main/resources/application.yaml` | Enabled Flyway |
| `cart-service/migrations/000001_baseline_schema.up.sql` | Created |
| `cart-service/migrations/000001_baseline_schema.down.sql` | Created |
| `cart-service/cmd/server/main.go` | Added migration call |
| `cart-service/go.mod` | Added golang-migrate |
| `cart-service/Dockerfile` | Copy migrations/ |
| `payment-service/migrations/000001_baseline_schema.up.sql` | Created |
| `payment-service/migrations/000001_baseline_schema.down.sql` | Created |
| `payment-service/cmd/server/main.go` | Added migration call |
| `payment-service/go.mod` | Added golang-migrate |
| `payment-service/Dockerfile` | Copy migrations/ |

---

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Existing data loss | V1 is idempotent (Go) or baselined (Java) -- no destructive DDL |
| Flyway checksum mismatch if V1 edited post-deploy | Never edit applied migrations; create V2 for changes |
| golang-migrate IF NOT EXISTS masks wrong schema | `pg_dump --schema-only` comparison to verify |
| Missing migrations/ dir in Docker image | Dockerfile COPY step ensures it's included |
