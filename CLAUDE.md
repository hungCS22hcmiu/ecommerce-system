# CLAUDE.md

## Project Overview

Distributed e-commerce platform — 5 microservices. Go services handle I/O-heavy concurrent workloads; Java/Spring Boot services handle complex business logic with transactions.

## Service Map

| Service | Language | Port | Status | Key Pattern |
|---|---|---|---|---|
| user-service | Go (Gin + GORM) | 8001 | **Implemented** | Pessimistic lock on login |
| product-service | Java/Spring Boot | 8081 | **In Progress** | Optimistic lock (@Version + @Retry) |
| cart-service | Go (Gin + GORM) | 8002 | Scaffolded | Redis-first, WATCH/MULTI/EXEC |
| order-service | Java/Spring Boot | 8082 | Scaffolded | Pessimistic lock on state transitions |
| payment-service | Go (Gin) | 8003 | Scaffolded | Idempotency key + DB UNIQUE |

"Scaffolded" = `cmd/server/main.go` + `config/config.go` + health probe only; internal packages are empty directories.

## Infrastructure Commands

```bash
cp .env.example .env              # then edit with actual values

docker compose up -d postgres redis          # core infra
docker compose up -d zookeeper kafka         # only for payment/order Kafka flows
docker compose up -d user-service            # single service
docker compose build user-service            # rebuild after code changes
```

Databases auto-initialized on first Postgres start via `script/init-databases.sql` (5 logical DBs, full schemas with indexes).

## Go Services (user-service, cart-service, payment-service)

```bash
cd user-service  # or cart-service / payment-service
go run ./cmd/server/main.go       # run
go test -race ./...               # test (always use -race)
go test ./internal/handler/...    # single package
go test -tags=integration -v -race ./internal/integration/  # requires real Postgres + Redis
```

Internal layout:
- `cmd/server/main.go` — dependency wiring: `db → repo → service → handler → router`
- `config/config.go` — env-based config (no config files)
- `internal/handler/` — Gin HTTP handlers
- `internal/middleware/` — recovery, structured logger, JWT auth
- `internal/model/` — GORM models
- `internal/repository/` — DB layer (interface + GORM impl)
- `internal/service/` — business logic (depends only on repo interfaces)
- `internal/dto/` — request/response structs with `validate:"..."` tags
- `internal/integration/` — integration tests (`//go:build integration`)
- `pkg/response/` — response envelope helpers
- `pkg/password/` — bcrypt Hash/Compare
- `pkg/jwt/` — RS256: GenerateAccessToken, GenerateRefreshToken, ValidateToken
- `pkg/blacklist/` — Redis `blacklist:{jti}` with TTL
- `pkg/session/` — Redis `session:{userID}` (30 min TTL)
- `pkg/loginattempt/` — Redis `login_attempts:{email}` (15 min sliding TTL)
- `pkg/verification/` — Redis verification code + cooldown + attempt tracking
- `pkg/email/` — SMTP sender (STARTTLS port 587)

### Go Patterns

- Services depend only on repository **interfaces** — unit-testable with `testify/mock`
- Context propagation: `handler → service → repository → db.WithContext(ctx)`
- AutoMigrate at startup in `main.go`
- Validation: `go-playground/validator/v10` on DTOs → field→tag error map
- Testing: `testify` (assert/require/mock), `-race` always. 70%+ service coverage, 100% auth handler.
- **Login TX pattern**: TX always commits for auth errors (wrong password, locked, not found). Auth errors stored in outer `loginErr` returned after TX. Only real DB errors rollback. Ensures `UpdateLoginAttempts` persists on failed logins.

## Java Services (product-service, order-service)

```bash
cd product-service  # or order-service
./mvnw spring-boot:run            # run
./mvnw test                       # test
./mvnw package -DskipTests        # build jar
```

Java 21, Spring Boot 3.5, Lombok. Flyway present but **disabled** (no migrations yet, `ddl-auto: none`).

## Architecture

### Communication
- **Sync REST**: Cart → Product (`PRODUCT_SERVICE_URL`) for price/stock validation
- **Async Kafka (Choreography Saga)**: `orders.created` → Payment processes → `payments.completed`/`payments.failed` → Order updates. Internal broker: `kafka:29092`

### Databases
Single Postgres, 5 logical databases: `ecommerce_users`, `ecommerce_products`, `ecommerce_carts`, `ecommerce_orders`, `ecommerce_payments`. Cross-DB refs enforced at app level, not FK constraints.

### Redis
- user-service: sessions, JWT blacklist, login attempts, verification codes
- cart-service: primary cart store (Redis source of truth, Postgres background sync)
- product-service: cache layer

### Concurrency (see `docs/adr/locking-strategy.md`)

| Service | Strategy | Rationale |
|---|---|---|
| User | `SELECT ... FOR UPDATE` | Write-heavy login; lockout correctness critical |
| Product | `@Version` optimistic + `@Retry` | Low contention, high throughput |
| Cart | Redis `WATCH/MULTI/EXEC` | Primary store is Redis; per-user contention low |
| Order | `SELECT ... FOR UPDATE` | Two concurrent state transitions must not both succeed |
| Payment | Idempotency key + `UNIQUE` | Handles Kafka at-least-once redelivery |

### JWT
RS256, 15 min access TTL. Keys: `./keys/private.pem` / `./keys/public.pem` (configurable via `JWT_PRIVATE_KEY_PATH` / `JWT_PUBLIC_KEY_PATH`).

### API Response Envelope (defined in `api/openapi.yaml`)
```json
{ "success": true, "data": { ... } }
{ "success": true, "data": [...], "meta": { "page": 0, "size": 20, "totalElements": 150, "totalPages": 8 } }
{ "success": false, "error": { ... } }
```

## Key Files
- `docker-compose.yml` — full stack (infra + 5 services, health checks, volumes, backend network)
- `script/init-databases.sql` — all 5 DB schemas with indexes (307 lines)
- `script/sample_users.sql` — 1 admin / 1 customer / 1 seller (pre-verified)
- `Makefile` — dev commands (currently user-service targets only)
- `api/openapi.yaml` — full REST API contract (all services)
- `docs/adr/locking-strategy.md` — per-service concurrency rationale
- `docs/adr/proposal.md` — full technical proposal
- `.env.example` — all 43 env vars with descriptions

## product-service (In Progress)

### Active Endpoints
```
GET  /health/live
POST /api/v1/products                        # seller only (X-Seller-Id header)
GET  /api/v1/products/{id}                   # public
GET  /api/v1/products?categoryId=&status=    # public; paginated
GET  /api/v1/products/search?q=              # public; full-text, paginated
PUT  /api/v1/products/{id}                   # seller only; ownership check → 403
DELETE /api/v1/products/{id}                 # seller only; soft delete → DELETED
```

### Auth Convention
No JWT validation yet. Gateway pre-validates and forwards `X-Seller-Id: <UUID>` header.
Missing header on write endpoints → 400.

### Key Classes
- `ProductServiceImpl` — business logic, optimistic lock via `@Version`
- `ProductController` — REST layer, `@PageableDefault(size=20, sort=createdAt DESC)`
- `GlobalExceptionHandler` — maps domain exceptions to envelope errors
- `ApiResponse<T>` — `{ success, data, meta?, error? }` envelope

### Tests
`ProductServiceImplTest` — 30 unit tests, all mocked (no Spring context):
`CreateProduct`(4) · `GetProduct`(3) · `ListProducts`(9) · `SearchProducts`(5) · `UpdateProduct`(6) · `DeleteProduct`(4)

---

## user-service (Implemented)

### Active Endpoints
```
GET  /health/live
GET  /health/ready
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/verify-email         # public; 6-digit code + brute-force protection
POST /api/v1/auth/resend-verification  # public; 60 s cooldown
POST /api/v1/auth/logout               # protected
GET  /api/v1/users/:id                 # internal only (Docker network boundary)
GET  /api/v1/users/profile             # protected
PUT  /api/v1/users/profile             # protected; invalidates session cache
POST /api/v1/users/addresses           # protected
PUT  /api/v1/users/addresses/:id       # protected; ownership check → 403
DELETE /api/v1/users/addresses/:id     # protected; ownership check → 403
PUT  /api/v1/users/addresses/:id/default  # protected; atomic TX
```

### Constructor Signatures
```go
service.NewAuthService(userRepo, authTokenRepo, db, bl, sessionCache, attemptCounter, verificationStore, emailSender, privateKey, publicKey)
service.NewUserService(userRepo, addrRepo, sessionCache)
```

### SMTP Config
Requires in `.env`: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`

### Stale Table Fix
```bash
docker exec ecommerce-postgres psql -U postgres -d ecommerce_users \
  -c "DROP TABLE IF EXISTS user_addresses, user_profiles, users CASCADE;"
docker compose restart user-service
```
