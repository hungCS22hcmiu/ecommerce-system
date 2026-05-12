# Database Migration Guide

## Overview

Each microservice owns its database schema through a dedicated migration tool. Migrations run automatically at service startup -- no manual steps required.

| Service | Database | Migration Tool | Migration Location |
|---|---|---|---|
| user-service | ecommerce_users | GORM AutoMigrate | Go model structs |
| product-service | ecommerce_products | Flyway | `src/main/resources/db/migration/` |
| order-service | ecommerce_orders | Flyway | `src/main/resources/db/migration/` |
| cart-service | ecommerce_carts | golang-migrate | `migrations/` |
| payment-service | ecommerce_payments | golang-migrate | `migrations/` |

---

## How It Works

### Database Creation

The Postgres container runs `script/init-databases.sql` on first boot (empty volume only). This script creates the 5 logical databases and installs the `uuid-ossp` extension in each. It does **not** create tables -- that's the job of each service's migration tool.

### Startup Flow

```
Postgres starts → init-databases.sql creates DBs (first boot only)
    ↓
Service starts → connects to its own DB
    ↓
Migration tool runs → applies any pending migrations
    ↓
Application code starts → HTTP server / Kafka consumer
```

### Flyway (product-service, order-service)

Flyway is a Java migration framework integrated with Spring Boot. It runs automatically before the application context fully loads.

**How it tracks state**: Creates a `flyway_schema_history` table in the database. Each applied migration gets a row with version, checksum, and timestamp.

**Startup behavior**:
1. Checks `flyway_schema_history` table
2. Compares migration files on classpath against applied versions
3. Applies any new migrations in version order
4. If schema is non-empty but no history table exists: `baseline-on-migrate` marks the current version as baseline (skips V1)

**Configuration** (in `application.yaml`):
```yaml
spring:
  flyway:
    enabled: true
    locations: classpath:db/migration
    baseline-on-migrate: true
    baseline-version: 1
```

### golang-migrate (cart-service, payment-service)

golang-migrate is a SQL-based migration library for Go. It runs in `main.go` after the DB connection is established, before the HTTP server starts.

**How it tracks state**: Creates a `schema_migrations` table with two columns: `version` (int) and `dirty` (bool).

**Startup behavior**:
1. Checks `schema_migrations` table
2. Runs `m.Up()` which applies all pending up-migrations
3. If no new migrations: returns `migrate.ErrNoChange` (handled gracefully)
4. V1 baseline uses `IF NOT EXISTS` so it's safe on pre-existing schemas

**Code in main.go**:
```go
import (
    "github.com/golang-migrate/migrate/v4"
    pgmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/source/file"
)

// After DB connection, before server start:
driver, _ := pgmigrate.WithInstance(sqlDB, &pgmigrate.Config{})
m, _ := migrate.NewWithDatabaseInstance("file://migrations", "db_name", driver)
if err := m.Up(); err != nil && err != migrate.ErrNoChange {
    slog.Error("migration failed", "error", err)
    os.Exit(1)
}
```

### GORM AutoMigrate (user-service)

user-service uses GORM's built-in `AutoMigrate()` which reads Go struct tags and creates/alters tables to match. This runs at startup in `cmd/server/main.go`.

```go
db.AutoMigrate(&model.User{}, &model.UserProfile{}, &model.UserAddress{}, &model.AuthToken{})
```

AutoMigrate is additive only -- it creates missing tables and adds missing columns, but never drops columns or tables.

---

## Adding a New Migration

### Flyway (product-service, order-service)

1. Create a new SQL file in `src/main/resources/db/migration/`:
   ```
   V2__add_product_tags.sql
   ```

2. Naming convention: `V{version}__{description}.sql`
   - Version must be sequential (V1, V2, V3, ...)
   - Double underscore between version and description
   - Description uses underscores for spaces

3. Write your DDL:
   ```sql
   CREATE TABLE product_tags (
       id          BIGSERIAL PRIMARY KEY,
       product_id  BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
       tag         VARCHAR(100) NOT NULL,
       created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
   );
   CREATE INDEX idx_product_tags_product ON product_tags(product_id);
   ```

4. Restart the service -- Flyway auto-applies on next startup.

**Rules**:
- **Never edit** an already-applied migration file. Flyway checksums detect changes and will refuse to start.
- Always create a new version file for schema changes.
- Test migrations locally before pushing.

### golang-migrate (cart-service, payment-service)

1. Create a pair of files in `migrations/`:
   ```
   000002_add_cart_notes.up.sql
   000002_add_cart_notes.down.sql
   ```

2. Naming convention: `{6-digit version}_{description}.{up|down}.sql`
   - Always provide both up and down files
   - Version must be sequential

3. Write your up migration:
   ```sql
   ALTER TABLE carts ADD COLUMN notes TEXT;
   ```

4. Write your down migration (rollback):
   ```sql
   ALTER TABLE carts DROP COLUMN IF EXISTS notes;
   ```

5. Restart the service -- migrations auto-apply on startup.

**Rules**:
- Always provide a down migration for rollback capability.
- Future migrations (V2+) do **not** need `IF NOT EXISTS` -- only V1 baseline uses that pattern.
- The `migrations/` directory must be present in the Docker image (Dockerfile copies it).

---

## Checking Migration Status

### Flyway services

```bash
# product-service
docker exec ecommerce-postgres psql -U postgres -d ecommerce_products \
  -c "SELECT version, type, description, installed_on, success FROM flyway_schema_history ORDER BY installed_rank;"

# order-service
docker exec ecommerce-postgres psql -U postgres -d ecommerce_orders \
  -c "SELECT version, type, description, installed_on, success FROM flyway_schema_history ORDER BY installed_rank;"
```

**Expected output** (after baseline):
```
 version |   type   |      description      |       installed_on        | success
---------+----------+-----------------------+---------------------------+---------
 1       | BASELINE | << Flyway Baseline >> | 2026-04-01 18:29:53.04576 | t
```

After V2 is applied:
```
 version |   type   |      description      |       installed_on        | success
---------+----------+-----------------------+---------------------------+---------
 1       | BASELINE | << Flyway Baseline >> | 2026-04-01 18:29:53.04576 | t
 2       | SQL      | add product tags      | 2026-04-05 10:00:00.00000 | t
```

### golang-migrate services

```bash
# cart-service
docker exec ecommerce-postgres psql -U postgres -d ecommerce_carts \
  -c "SELECT version, dirty FROM schema_migrations;"

# payment-service
docker exec ecommerce-postgres psql -U postgres -d ecommerce_payments \
  -c "SELECT version, dirty FROM schema_migrations;"
```

**Expected output**:
```
 version | dirty
---------+-------
       1 | f
```

If `dirty = true`, the last migration failed mid-way. See Troubleshooting below.

### List all tables in a database

```bash
docker exec ecommerce-postgres psql -U postgres -d ecommerce_products -c "\dt"
```

---

## Common Workflows

### Fresh Setup (New Developer)

```bash
cp .env.example .env                    # configure environment
docker compose up -d postgres redis     # start infrastructure
docker compose up -d user-service       # AutoMigrate creates user tables
docker compose up -d product-service    # Flyway runs V1
# cart/order/payment services follow the same pattern
```

### Full Reset (Wipe All Data)

```bash
make db-nuke
# This runs: docker compose down -v && docker compose up -d postgres redis
# Wipes postgres_data volume, init-databases.sql re-runs, all migrations re-apply on next service start
```

### Schema Dump (For Comparison)

```bash
docker exec ecommerce-postgres pg_dump -U postgres -d ecommerce_products --schema-only > products_schema.sql
```

---

## Troubleshooting

### Flyway: "Validate failed: Migrations have been applied that are not resolved locally"

You have applied migrations in the DB that don't exist in your code. Someone else may have added a migration you don't have yet. Pull latest code.

### Flyway: "Migration checksum mismatch"

An already-applied migration file was edited. **Never edit applied migrations.** To fix: restore the original file content, then create a new V{N+1} migration for your changes.

### golang-migrate: dirty state (`dirty = true`)

A migration failed mid-execution. The database is in an inconsistent state.

1. Check what partially applied:
   ```bash
   docker exec ecommerce-postgres psql -U postgres -d ecommerce_carts -c "\dt"
   ```
2. Manually fix the schema (apply remaining statements or revert partial ones)
3. Force the version:
   ```bash
   # Mark as clean at version 1 (adjust version as needed)
   docker exec ecommerce-postgres psql -U postgres -d ecommerce_carts \
     -c "UPDATE schema_migrations SET dirty = false WHERE version = 1;"
   ```

### golang-migrate: "no change" on startup

This is normal. It means all migrations are already applied. The service logs `"database migrations applied"` and continues.

### Service won't start: "migration init failed"

The `migrations/` directory is missing from the Docker image. Verify the Dockerfile has:
```dockerfile
COPY --from=builder /app/migrations ./migrations
```

---

## File Reference

### Migration Files

| Service | File | Description |
|---|---|---|
| product-service | `src/main/resources/db/migration/V1__baseline_schema.sql` | Baseline: 2 enums, 4 tables, 11 indexes |
| order-service | `src/main/resources/db/migration/V1__baseline_schema.sql` | Baseline: 3 enums, 4 tables, 9 indexes |
| cart-service | `migrations/000001_baseline_schema.up.sql` | Baseline: 1 enum, 2 tables, 5 indexes (idempotent) |
| cart-service | `migrations/000001_baseline_schema.down.sql` | Rollback: drops tables + enum |
| payment-service | `migrations/000001_baseline_schema.up.sql` | Baseline: 2 enums, 2 tables, 5 indexes (idempotent) |
| payment-service | `migrations/000001_baseline_schema.down.sql` | Rollback: drops tables + enums |

### Configuration Files

| Service | File | Key Settings |
|---|---|---|
| product-service | `src/main/resources/application.yaml` | `flyway.enabled: true`, `baseline-on-migrate: true` |
| order-service | `src/main/resources/application.yaml` | `flyway.enabled: true`, `baseline-on-migrate: true` |
| cart-service | `cmd/server/main.go` | `migrate.Up()` after DB connect |
| payment-service | `cmd/server/main.go` | `migrate.Up()` after DB connect |

### Infrastructure

| File | Purpose |
|---|---|
| `script/init-databases.sql` | Creates 5 databases + uuid-ossp extension (runs on first Postgres boot only) |
| `script/init-databases.full.sql.bak` | Archived original 307-line init script (reference only) |
