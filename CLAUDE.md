# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A distributed e-commerce platform with 5 microservices. Go services handle I/O-heavy concurrent workloads; Java/Spring Boot services handle complex business logic with transactions.

## Service Map

| Service | Language | Port | Key Pattern |
|---|---|---|---|
| user-service | Go (Gin + GORM) | 8001 | Pessimistic lock on login |
| cart-service | Go (Gin + GORM) | 8002 | Redis-first, WATCH/MULTI/EXEC |
| payment-service | Go (Gin) | 8003 | Idempotency key + DB UNIQUE |
| product-service | Java/Spring Boot | 8081 | Optimistic lock (@Version + @Retry) |
| order-service | Java/Spring Boot | 8082 | Pessimistic lock on state transitions |

## Infrastructure Commands

Copy and configure environment:
```bash
cp .env.example .env
# Edit .env with actual values
```

Start only what the current task needs (prefer minimal):
```bash
# Core infrastructure (needed by almost everything)
docker compose up -d postgres redis

# Add Kafka only when working on payment/order Kafka flows
docker compose up -d zookeeper kafka

# Start a specific service
docker compose up -d user-service

# Build images (first time or after code changes)
docker compose build             # all services
docker compose build user-service  # single service
```

Databases are initialized automatically on first Postgres start via `script/init-databases.sql`.

## Go Services (user-service, cart-service, payment-service)

Each Go service is a self-contained module. Run from its directory:

```bash
cd user-service   # or cart-service / payment-service

# Run
go run ./cmd/server/main.go

# Build
go build -o bin/server ./cmd/server/main.go

# Test
go test ./...

# Single package test
go test ./internal/handler/...

# Test with race detector (required for concurrency code)
go test -race ./...
```

Config is loaded from environment variables with fallbacks (see `config/config.go`). No config files — set env vars or use a `.env` file with Docker Compose.

Go service internal layout:
- `cmd/server/main.go` — wires dependencies (DB, Redis, router) and starts server
- `config/config.go` — env-based configuration
- `internal/handler/` — HTTP handlers (Gin)
- `internal/middleware/` — recovery + structured logger middleware
- `internal/model/` — GORM models
- `internal/repository/` — DB access layer (interface + GORM impl)
- `internal/service/` — business logic (depends only on repository interface)
- `internal/integration/` — integration tests (build tag `integration`; require real Postgres + Redis)
- `internal/dto/` — request/response structs with `validate:"..."` tags
- `pkg/response/` — shared response envelope helpers (`Success`, `Created`, `Error`, `BadRequest`, `Conflict`, `InternalError`, etc.)
- `pkg/password/` — bcrypt helpers: `Hash(plain) → hash`, `Compare(hash, plain) → bool`
- `pkg/jwt/` — JWT helpers (RS256): `GenerateAccessToken`, `GenerateRefreshToken`, `ValidateToken`, `LoadPrivateKey`/`LoadPublicKey`
- `pkg/blacklist/` — `Blacklist` interface + Redis impl; `Add(jti, ttl)` / `Contains(jti)` using `blacklist:{jti}` key pattern
- `pkg/session/` — `Cache` interface + Redis impl; `Set/Get/Delete` user profile at `session:{userID}` (30 min TTL)
- `pkg/loginattempt/` — `Counter` interface + Redis impl; `Increment/Get/Delete` at `login_attempts:{email}` (15 min sliding TTL)
- `pkg/verification/` — `Store` interface + Redis impl; `SetCode/GetCode/DeleteCode` (15 min TTL), `SetCooldown/HasCooldown` (60 s TTL), `IncrementAttempts/DeleteAttempts` — keys: `verification:{email}`, `verification_cooldown:{email}`, `verification_attempts:{email}`
- `pkg/email/` — `Sender` interface + `smtpSender` impl; `SendVerificationCode(ctx, to, code)` via `net/smtp` STARTTLS on port 587
- `api.txt` — curl-based API testing reference for the service

### Go Service Patterns

**Dependency wiring** (always in `main.go`):
```
db → repository → service → handler → router
```
Services depend only on repository *interfaces*, never on `*gorm.DB` directly — keeps them unit-testable with `testify/mock`.

**Context propagation**: every handler extracts `c.Request.Context()` and passes it through service → repository → `db.WithContext(ctx)`.

**AutoMigrate**: called in `main.go` at startup; drops and recreates tables cleanly in dev if schema drifts.

**Validation**: `github.com/go-playground/validator/v10` on DTOs; errors mapped to field→tag map in `VALIDATION_ERROR` response.

**Testing stack**: `github.com/stretchr/testify` (assert/require/mock). Run with `-race` flag always.
Coverage targets: 70%+ on service layer, 100% on auth handler.

**Integration tests**: tagged `//go:build integration` in `internal/integration/`. Require real Postgres + Redis. Use `httptest.NewServer` with a full in-memory wired stack (no mocks). Run separately:
```bash
docker compose up -d postgres redis
go test -tags=integration -v -race ./internal/integration/
```

**Login TX pattern**: The `Login` DB transaction always commits (returns `nil`) for auth-layer errors (wrong password, account locked, user not found). Auth errors are stored in an outer `loginErr` variable returned after the TX. Only real DB errors trigger a rollback. This ensures `UpdateLoginAttempts` writes persist even on failed login attempts.

## Java Services (product-service, order-service)

Each Java service uses Maven wrapper. Run from its directory:

```bash
cd product-service   # or order-service

# Run
./mvnw spring-boot:run

# Build (skip tests)
./mvnw package -DskipTests

# Test
./mvnw test

# Single test class
./mvnw test -Dtest=ProductServiceApplicationTests
```

Java version: 21. Spring Boot: 3.5. Uses Flyway for DB migrations, Lombok for boilerplate reduction.

## Architecture

### Communication
- **Synchronous REST**: Cart Service calls Product Service (`PRODUCT_SERVICE_URL`) for price/stock validation.
- **Async Kafka (Choreography Saga)**: `orders.created` → Payment Service processes → emits `payments.completed` or `payments.failed` → Order Service updates status. Inside Docker, services connect to Kafka at `kafka:29092` (internal listener), not `kafka:9092` (host port).

### Databases
Single PostgreSQL instance with 5 logical databases (one per service). Cross-DB references are enforced at the application level, not by FK constraints.

| Database | Owned by |
|---|---|
| ecommerce_users | user-service |
| ecommerce_products | product-service |
| ecommerce_carts | cart-service |
| ecommerce_orders | order-service |
| ecommerce_payments | payment-service |

### Redis Usage
- Sessions, JWT blacklist, rate limiting (user-service)
- Cart primary store — Redis is the source of truth, PostgreSQL is a background sync (cart-service)
- Cache layer (product-service)

### Concurrency Locking — Per Service (see `docs/adr/locking-strategy.md`)

| Service | Strategy | Why |
|---|---|---|
| User | `SELECT ... FOR UPDATE` | Write-heavy login row; lockout correctness critical |
| Product | `@Version` optimistic + `@Retry` | Low contention normal traffic; high throughput |
| Cart | Redis `WATCH/MULTI/EXEC` | Primary store is Redis; per-user contention is low |
| Order | `SELECT ... FOR UPDATE` | Catastrophic if two state transitions both succeed |
| Payment | Idempotency key + `UNIQUE` constraint | Handles Kafka at-least-once redelivery |

### JWT
- Algorithm: RS256
- Access token TTL: 15 minutes
- Keys: `./keys/private.pem` (sign) and `./keys/public.pem` (verify) — paths configurable via `JWT_PRIVATE_KEY_PATH` / `JWT_PUBLIC_KEY_PATH`

### API Response Envelope
All responses use a consistent shape (defined in `api/openapi.yaml`):
```json
{ "success": true, "data": { ... } }
{ "success": true, "data": [...], "meta": { "page": 0, "size": 20, "totalElements": 150, "totalPages": 8 } }
{ "success": false, "error": { ... } }
```

### Health Probes
- Go services: `GET /health/live` + `GET /health/ready` (checks Postgres + Redis)
- Java services: `GET /health/live` only (no `/ready` endpoint yet)

## Key Files
- `docker-compose.yml` — full stack (infrastructure + all 5 service containers); each service has a `Dockerfile` in its root directory
- `script/init-databases.sql` — creates all 5 databases and schemas with indexes
- `script/sample_users.sql` — inserts 1 admin / 1 customer / 1 seller with pre-verified accounts for local testing
- `Makefile` — common dev commands: `deploy-user`, `db-restart`, `db-nuke`, `db-seed`, `test-user`, etc.
- `api/openapi.yaml` — full REST API contract
- `docs/adr/locking-strategy.md` — detailed rationale for per-service concurrency decisions
- `docs/adr/proposal.md` — full technical proposal with architecture decisions
- `docs/adr/timeline.md` — 10-week day-by-day implementation plan
- `.env.example` — all required environment variables with descriptions
- `<service>/api.txt` — curl-based API testing reference per service

## Implementation Progress

### user-service (Day 11 complete)
Implemented:
- `internal/model/` — `User`, `UserProfile`, `UserAddress`, `AuthToken` (GORM + UUID PKs, soft delete on User; AuthToken stores SHA-256 hashed refresh token)
- `pkg/password/` — bcrypt cost 12
- `pkg/jwt/` — RS256 JWT: `GenerateAccessToken` (15 min TTL, jti claim), `GenerateRefreshToken` (128-char hex), `ValidateToken`, `LoadPrivateKey`/`LoadPublicKey`
- `pkg/blacklist/` — `Blacklist` interface + Redis impl (`blacklist:{jti}` key, TTL = remaining access-token lifetime)
- `pkg/session/` — `Cache` interface + Redis impl (`session:{userID}` key, 30 min TTL, JSON value); SET on login, GET on refresh, DEL on logout
- `pkg/loginattempt/` — `Counter` interface + Redis impl (`login_attempts:{email}` key, 15 min sliding TTL); INCR post-TX on bad password, pre-check before DB TX, DEL on success
- `internal/dto/` — `RegisterRequest`, `LoginRequest`, `LoginResponse`, `RefreshRequest`, `UserResponse`, `UpdateProfileRequest`, `CreateAddressRequest`, `UpdateAddressRequest`, `AddressResponse`, `ProfileResponse`, `VerifyEmailRequest`, `ResendVerificationRequest`
- `internal/repository/user_repository.go` — `Create`, `FindByEmail`, `FindByID`, `FindByEmailForUpdate` (SELECT … FOR UPDATE + Preload("Profile")), `UpdateLoginAttempts`, `FindByIDWithProfile` (Preload Profile + Addresses), `UpdateProfile`, `UpdateVerificationStatus`
- `internal/repository/auth_token_repository.go` — `Create`, `FindByHash`, `RevokeByUserID`
- `internal/repository/address_repository.go` — `Create`, `FindByID`, `Update`, `Delete`, `SetDefault` (atomic TX: clear all → set one)
- `internal/service/auth_service.go` — `Register` (creates user + sends verification code), `Login` (pessimistic lock + Redis pre-check + two-layer lockout + session SET; rejects unverified accounts with `ErrEmailNotVerified`), `Refresh` (session cache hit skips FindByID), `Logout` (blacklist jti + session DEL + RevokeByUserID), `VerifyEmail` (brute-force protected, marks `is_verified=true`), `ResendVerification` (60 s cooldown, re-sends code); error sentinels: `ErrDuplicateEmail`, `ErrUserNotFound`, `ErrInvalidCredentials`, `ErrAccountLocked`, `ErrInvalidToken`, `ErrEmailNotVerified`, `ErrInvalidCode`, `ErrAlreadyVerified`, `ErrResendCooldown`, `ErrTooManyVerifyAttempts`
- `internal/service/user_service.go` — `GetUser` (internal lookup, returns UserResponse), `GetProfile`, `UpdateProfile` (invalidates session cache), `AddAddress`, `UpdateAddress`, `DeleteAddress`, `SetDefaultAddress` (ownership check on all address ops); error sentinels: `ErrAddressNotFound`, `ErrAddressForbidden`
- `internal/handler/auth_handler.go` — register, login, refresh, logout, verify-email, resend-verification handlers; validation errors mapped to field→tag map
- `internal/handler/user_handler.go` — profile + address handlers + `GetUser` (internal, no auth); `parseUserID`/`parseAddressID` helpers; `handleAddressError` for 404/403 mapping
- `internal/handler/health_handler.go` — `/health/live` (always up) + `/health/ready` (pings Postgres + Redis)
- `internal/middleware/` — panic recovery + structured JSON logger (X-Correlation-ID) + `Auth` JWT middleware (Bearer extraction → RS256 validate → Redis blacklist check → context injection)
- `internal/integration/auth_flow_test.go` — full-stack integration tests (build tag `integration`): register → login → protected route → refresh → logout → token rejected; brute-force counter; middleware rejection
- `Dockerfile` — multi-stage production build (CGO_ENABLED=0, alpine runtime)
- `Dockerfile.dev` + `.air.toml` — Air hot reload for development (vol-mounted source)
- DB connection pool: 25 max open, 5 idle, 5 min lifetime
- Graceful shutdown on SIGTERM/SIGINT
- Unit tests: `pkg/password`, `pkg/jwt`, `pkg/session`, `pkg/loginattempt`, `internal/service`, `internal/handler`, `internal/middleware` — race-detector clean
- Integration tests: `internal/integration/` — verified against real Postgres + Redis

Active endpoints:
- `GET  /health/live`
- `GET  /health/ready`
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/verify-email`        ← public; 6-digit code + brute-force protection
- `POST /api/v1/auth/resend-verification` ← public; 60 s cooldown; 404 if email unregistered
- `POST /api/v1/auth/logout`              ← protected by Auth middleware
- `GET  /api/v1/users/:id`               ← internal, no auth; Docker network boundary only
- `GET  /api/v1/users/profile`            ← protected
- `PUT  /api/v1/users/profile`            ← protected; invalidates session cache
- `POST /api/v1/users/addresses`          ← protected
- `PUT  /api/v1/users/addresses/:id`      ← protected; ownership check → 403
- `DELETE /api/v1/users/addresses/:id`    ← protected; ownership check → 403
- `PUT  /api/v1/users/addresses/:id/default` ← protected; atomic TX sets single default

### Service constructor signatures (Day 11 + email verification)
```go
service.NewAuthService(userRepo, authTokenRepo, db, bl, sessionCache, attemptCounter, verificationStore, emailSender, privateKey, publicKey)
service.NewUserService(userRepo, addrRepo, sessionCache)
```

SMTP env vars required in `.env` and forwarded to the container via `docker-compose.yml`:
```
SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM
```

Stale-table note: if AutoMigrate fails with "constraint does not exist", drop the tables in psql and restart:
```bash
docker exec ecommerce-postgres psql -U postgres -d ecommerce_users \
  -c "DROP TABLE IF EXISTS user_addresses, user_profiles, users CASCADE;"
docker compose restart user-service
```
