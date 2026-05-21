# cart-service

Shopping cart microservice for the distributed e-commerce platform. Runs on port **8002**.

## Overview

cart-service uses a **Redis-first** architecture: all reads and writes go to Redis for sub-millisecond latency. A background worker asynchronously flushes the Redis state to PostgreSQL every 30 seconds, so the database serves as durable backup — not the hot path.

**Concurrency:** mutations use Redis `WATCH / MULTI / EXEC` (optimistic locking). Up to 3 retries are attempted on conflict before returning `409 Conflict` to the caller.

**Resilience:** calls to product-service go through a three-state circuit breaker (CLOSED → OPEN → HALF-OPEN). Read-only cart operations (get, update, remove, clear) bypass the product client entirely and continue working even when the circuit is open.

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.23 |
| HTTP framework | Gin |
| ORM | GORM |
| Primary store | Redis 7 (Hash per user) |
| Durable store | PostgreSQL 15 (`ecommerce_carts`) |
| Migrations | golang-migrate |
| Auth | RS256 JWT (public key from user-service) |

## Architecture

```
                ┌──────────────────────────────────┐
                │          cart-service             │
                │                                  │
  Client ──────▶│  Handler → Service → RedisRepo   │──▶ Redis
                │                ↓                 │
                │         ProductClient            │──▶ product-service
                │          (+ CircuitBreaker)      │
                │                                  │
                │     Background SyncWorker (30s)  │──▶ PostgreSQL
                └──────────────────────────────────┘
```

**Redis key format:** `cart:{userID}` — a Hash where each field is a `productId` (string) and the value is a JSON-encoded `CartItemValue` (`product_name`, `quantity`, `unit_price`). TTL: 7 days, refreshed on every write.

## API

All routes require `Authorization: Bearer <JWT>`.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/cart` | Get the current user's cart |
| `POST` | `/api/v1/cart/items` | Add an item (validates against product-service) |
| `PUT` | `/api/v1/cart/items/:productId` | Update quantity of an existing item |
| `DELETE` | `/api/v1/cart/items/:productId` | Remove a single item |
| `DELETE` | `/api/v1/cart` | Clear the entire cart (Redis + Postgres) |
| `GET` | `/health/live` | Liveness probe |
| `GET` | `/health/ready` | Readiness probe (checks Postgres + Redis) |

### Request / Response

**Add item**
```json
POST /api/v1/cart/items
{ "product_id": 1, "quantity": 2 }
```

**Update item**
```json
PUT /api/v1/cart/items/1
{ "quantity": 5 }
```

**Cart response envelope**
```json
{
  "success": true,
  "data": {
    "user_id": "...",
    "status": "ACTIVE",
    "items": [
      { "product_id": 1, "product_name": "Widget", "quantity": 2, "unit_price": 9.99, "subtotal": 19.98 }
    ],
    "total": 19.98,
    "updated_at": "2025-01-01T00:00:00Z"
  }
}
```

### Error codes

| HTTP | Code | Meaning |
|---|---|---|
| 404 | `NOT_FOUND` | Product doesn't exist or is not ACTIVE |
| 403 | `SELLER_CANNOT_BUY_OWN_PRODUCT` | Seller attempted to add their own product to cart |
| 409 | `INSUFFICIENT_STOCK` | Not enough available stock for the requested quantity |
| 409 | `CONCURRENT_UPDATE` | Redis WATCH conflict — client should retry |
| 503 | `SERVICE_UNAVAILABLE` | product-service circuit is open |

## Configuration

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8002` | HTTP listen port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | PostgreSQL user |
| `DB_PASSWORD` | `postgres` | PostgreSQL password |
| `DB_NAME` | `ecommerce_carts` | PostgreSQL database |
| `REDIS_HOST` | `localhost` | Redis host |
| `REDIS_PORT` | `6379` | Redis port |
| `REDIS_PASSWORD` | _(empty)_ | Redis password |
| `PRODUCT_SERVICE_URL` | `http://localhost:8081` | product-service base URL |
| `JWT_PUBLIC_KEY_PATH` | `./keys/public.pem` | RS256 public key for JWT validation |
| `ENV` | `development` | Set to `production` to enable Gin release mode |

## Running

**With Docker (recommended)**

```bash
# From repo root
cp .env.example .env
docker compose up -d postgres redis
docker compose up -d cart-service
```

**Locally**

```bash
cd cart-service
go run ./cmd/server/main.go
```

Migrations run automatically on startup via `golang-migrate`.

## Testing

```bash
# Unit tests (no external deps)
go test -race ./internal/service/... ./internal/handler/...

# Integration tests (requires Postgres + Redis on localhost)
go test -tags=integration -v -race ./internal/integration/
```

Integration tests spin up a real Redis DB (index 1) and hit the actual Postgres database. A mock HTTP server stands in for product-service.

**Test coverage:**

| Package | Tests | Type |
|---|---|---|
| `internal/service` | 9 unit tests | Mock repos + product client |
| `internal/handler` | Handler-level tests | `httptest` |
| `internal/integration` | 9 integration tests | Real Redis + Postgres |

Notable integration tests:
- `TestConcurrentAdd_Integration` — 10 goroutines add the same product simultaneously; asserts exactly 1 Redis field remains (no lost updates)
- `TestRedisToPostgresSync_Integration` — verifies the background sync worker correctly flushes Redis → Postgres
- `TestDegradedMode_CircuitOpen_ReadOpsStillWork` — opens the circuit breaker, then proves get/update/remove/clear still work

## Key Files

```
cmd/server/main.go                    # wiring: DB, Redis, router, sync worker, graceful shutdown
config/config.go                      # env-based configuration
internal/handler/cart_handler.go      # HTTP layer, error mapping
internal/service/cart_service.go      # business logic, AddItem calls product-service
internal/repository/redis_cart_repository.go  # WATCH/MULTI/EXEC, TTL management
internal/repository/cart_repository.go        # Postgres upsert + replace
internal/cache/sync.go                # background worker: Redis → Postgres flush every 30s
internal/client/product_client.go     # HTTP client with retry (3 attempts) + circuit breaker
internal/client/circuit_breaker.go    # three-state circuit breaker (threshold=5, timeout=30s)
internal/middleware/auth.go           # RS256 JWT validation, sets userID in Gin context
migrations/                           # golang-migrate SQL files
```

## Database Schema

```sql
carts (
  id UUID PK, user_id UUID, status cart_status, expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ
)

cart_items (
  id UUID PK, cart_id UUID FK → carts, product_id BIGINT,
  product_name VARCHAR(200), quantity INT, unit_price DECIMAL(10,2), added_at TIMESTAMPTZ
)
-- UNIQUE (cart_id, product_id)
```

`cart_status` enum: `ACTIVE`, `CHECKED_OUT`, `ABANDONED`

The Postgres schema is append-only from cart-service's perspective. Cart items for checkout are passed directly from the frontend via router state; no cross-service Postgres access occurs at checkout time.
