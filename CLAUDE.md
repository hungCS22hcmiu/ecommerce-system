# CLAUDE.md

## Project Overview

Distributed e-commerce platform — 5 microservices. Go for I/O-heavy concurrent workloads; Java/Spring Boot for complex business logic with transactions.

## Service Map

| Service | Language | Port | Status | Key Pattern |
|---|---|---|---|---|
| user-service | Go (Gin + GORM) | 8001 | **Implemented** | Pessimistic lock on login |
| product-service | Java/Spring Boot | 8081 | **In Progress** | Optimistic lock + Redis cache-aside |
| cart-service | Go (Gin + GORM) | 8002 | Scaffolded | Redis-first, WATCH/MULTI/EXEC |
| order-service | Java/Spring Boot | 8082 | Scaffolded | Pessimistic lock on state transitions |
| payment-service | Go (Gin) | 8003 | **Implemented** | Idempotency key + DB UNIQUE + Kafka saga |

"Scaffolded" = `cmd/server/main.go` + `config/config.go` + health probe only.

## Infrastructure Commands

```bash
cp .env.example .env
docker compose up -d postgres redis          # core infra
docker compose up -d zookeeper kafka         # only for payment/order Kafka flows
docker compose up -d product-service         # single service
docker compose build product-service         # rebuild after code changes
```

Databases auto-initialized via `script/init-databases.sql` (5 logical DBs, full schemas).

## Go Services (user-service, cart-service, payment-service)

```bash
cd user-service
go run ./cmd/server/main.go
go test -race ./...
go test -tags=integration -v -race ./internal/integration/  # requires real Postgres + Redis
```

Layout: `cmd/server/main.go` (wiring) · `config/` · `internal/handler/` · `internal/middleware/` · `internal/model/` · `internal/repository/` · `internal/service/` · `internal/dto/` · `internal/integration/` · `pkg/`

Key pkgs: `response` · `password` (bcrypt) · `jwt` (RS256) · `blacklist` · `session` · `loginattempt` · `verification` · `email`

**Patterns:** Services depend on repo interfaces. Context propagation: handler → service → repo → `db.WithContext(ctx)`. Validation via `go-playground/validator/v10`. Testing: `testify`, always `-race`, 70%+ service coverage. Login TX always commits for auth errors (wrong password/locked/not found) — only real DB errors rollback.

## Java Services (product-service, order-service)

```bash
cd product-service
./mvnw spring-boot:run
./mvnw test
./mvnw package -DskipTests
```

Java 21, Spring Boot 3.5, Lombok. Flyway enabled (`ddl-auto: none`, migrations in `classpath:db/migration`).

## Architecture

**Sync REST:** Cart → Product (`PRODUCT_SERVICE_URL`) for price/stock validation.  
**Async Kafka (Choreography Saga):** `orders.created` → Payment → `payments.completed/failed` → Order. Broker: `kafka:29092`.

**Databases:** Single Postgres, 5 logical DBs (`ecommerce_users/products/carts/orders/payments`). Cross-DB refs at app level, no FK constraints.

**Redis:** user-service (sessions, JWT blacklist, login attempts, verification) · cart-service (primary store) · product-service (cache-aside layer)

**Concurrency:**

| Service | Strategy |
|---|---|
| User | `SELECT ... FOR UPDATE` |
| Product | `@Version` optimistic + `@Retryable` |
| Cart | Redis `WATCH/MULTI/EXEC` |
| Order | `SELECT ... FOR UPDATE` |
| Payment | Idempotency key + `UNIQUE` |

**JWT:** RS256, 15 min access TTL. Keys: `./keys/private.pem` / `./keys/public.pem`.

**API envelope:** `{ success, data, meta? }` / `{ success: false, error }` — see `api/openapi.yaml`.

## Key Files
- `docker-compose.yml` — full stack with health checks
- `script/init-databases.sql` — all 5 DB schemas (307 lines)
- `script/sample_users.sql` — 1 admin / 1 customer / 1 seller (pre-verified)
- `api/openapi.yaml` — full REST API contract
- `docs/adr/locking-strategy.md` — concurrency rationale per service
- `.env.example` — all 43 env vars

## product-service (In Progress)

**Endpoints:**
```
GET  /health/live
POST /api/v1/products                     # seller only (X-Seller-Id header)
GET  /api/v1/products/{id}                # public; cached 30 min
GET  /api/v1/products?categoryId=&status= # public; paginated; cached 3 min
GET  /api/v1/products/search?q=           # public; full-text; cached 3 min
PUT  /api/v1/products/{id}                # seller only; ownership → 403
DELETE /api/v1/products/{id}              # seller only; soft delete → DELETED
```

**Auth:** No JWT yet. Gateway forwards `X-Seller-Id: <UUID>`; missing header → 400.

**Key classes:**
- `ProductServiceImpl` — business logic, `@Version` optimistic lock, cache annotations
- `RedisConfig` — `RedisCacheManager`, Jackson JSON serializer, TTLs: product=30min / productList=3min / default=10min, prefix `product-service::`
- `CacheWarmupService` — async warmup on startup, loads top 100 active products
- `ProductController` — REST layer, `@PageableDefault(size=20, sort=createdAt DESC)`
- `GlobalExceptionHandler` — maps domain exceptions to envelope errors

**Cache strategy (Cache-Aside):**
- `@Cacheable("product")` on `getProduct` — key = product ID
- `@CachePut("product")` + `@CacheEvict("productList")` on `updateProduct`
- `@CacheEvict` both caches on `deleteProduct` and `createProduct`
- `@Cacheable("productList")` on `listProducts` / `searchProducts` — composite key

**Tests (62 total):**
- `ProductServiceImplTest` — 30 unit tests (Mockito, no Spring context)
- `InventoryServiceImplTest` — 9 unit tests
- `ProductServiceCacheTest` — 5 cache AOP tests (in-memory cache, mocked repos)
- `ProductCacheIntegrationTest` — 15 integration tests (real Redis + Postgres via Testcontainers): key format, TTL, serialization round-trip, invalidation, not-found not cached
- `InventoryConcurrencyTest` — 2 concurrency tests (Testcontainers Postgres)
- `ProductServiceApplicationTests` — context load smoke test

## user-service (Implemented)

**Endpoints:** register · login · refresh · verify-email · resend-verification · logout · GET/PUT profile · POST/PUT/DELETE/default address

**Constructor signatures:**
```go
service.NewAuthService(userRepo, authTokenRepo, db, bl, sessionCache, attemptCounter, verificationStore, emailSender, privateKey, publicKey)
service.NewUserService(userRepo, addrRepo, sessionCache)
```

**SMTP:** requires `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM` in `.env`

**Stale table fix:**
```bash
docker exec ecommerce-postgres psql -U postgres -d ecommerce_users \
  -c "DROP TABLE IF EXISTS user_addresses, user_profiles, users CASCADE;"
docker compose restart user-service
```

## payment-service (Implemented)

**Endpoints:**
```
POST /api/v1/payments                   # manual payment (direct trigger)
GET  /api/v1/payments                   # list payments for authenticated user
GET  /api/v1/payments/:id               # get payment by ID
GET  /api/v1/payments/order/:orderId    # get payment by order ID
GET  /health/live
GET  /health/ready                      # checks postgres + kafka
```

**Auth:** RS256 JWT (`Authorization: Bearer <token>`). Public key loaded from `JWT_PUBLIC_KEY_PATH`.

**Kafka saga (choreography):**
- Consumes: `orders.created` (group `payment-service`, manual commit, `StartOffset=earliest`)
- Publishes: `payments.completed` → `payments.failed` on terminal outcome
- Worker pool: 5 goroutines (`KAFKA_WORKER_COUNT`), buffered channel size 100
- `__TypeId__` header set on outbound messages so Spring Kafka `JsonDeserializer` resolves the correct class without requiring config changes on order-service

**Key packages:**
- `internal/kafka/event/events.go` — `OrderCreatedEvent`, `PaymentCompletedEvent`, `PaymentFailedEvent`
- `internal/kafka/producer.go` — one `*kafka.Writer` per topic (sync, `RequireAll`)
- `internal/kafka/consumer.go` — fetch loop + worker pool, graceful drain on shutdown
- `internal/service/payment_service.go` — `ProcessPayment` with idempotency key dedup
- `internal/gateway/mock_gateway.go` — mock payment gateway (90% success rate)
- `internal/model/payment.go` — `Payment` + `PaymentHistory` (manual `TableName()` override)

**Resilience:** `StartOffset: kafka.FirstOffset` + manual commit → at-least-once delivery; idempotency key (`orderId`) prevents duplicate processing on redelivery.

**E2E test:** `bash script/e2e-payment.sh` — full saga: login → create order → poll payment → verify CONFIRMED order → health check (12 assertions).
