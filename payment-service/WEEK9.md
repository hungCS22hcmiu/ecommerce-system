# Week 9 Implementation — Payment Service Core

This document summarises everything built in sections 9.1–9.11 of `payment-plan.md`. Kafka (Week 10) and resilience (Week 11) are not included here.

---

## What Was Built

### Layer map

```
payment-service/
├── pkg/
│   ├── jwt/jwt.go                          RSA token validation (9.3)
│   └── response/response.go                JSON envelope helpers (9.2)
├── internal/
│   ├── model/payment.go                    GORM structs + PG enum types (9.4)
│   ├── dto/payment_dto.go                  Request/response shapes + mapper (9.5)
│   ├── repository/payment_repository.go   DB access + idempotency sentinel (9.6)
│   ├── gateway/mock_gateway.go             Simulated payment provider (9.7)
│   ├── service/
│   │   ├── payment_service.go              Business logic (9.8)
│   │   └── payment_service_test.go         Unit tests (9.11a)
│   ├── middleware/
│   │   ├── correlation.go                  X-Correlation-ID propagation (9.9)
│   │   ├── auth.go                         RS256 JWT validation (9.9)
│   │   ├── logger.go                       Structured JSON request logging (9.9)
│   │   └── recovery.go                     Panic → 500 (9.9)
│   ├── handler/payment_handler.go          HTTP handlers (9.9)
│   └── integration/
│       └── payment_idempotency_test.go     Concurrent DB proof (9.11b)
└── cmd/server/main.go                      Full service wiring (9.10)
```

---

## Section-by-Section Summary

### 9.1 — Dependencies

Added to `go.mod`:

| Package | Purpose |
|---|---|
| `github.com/segmentio/kafka-go v0.4.51` | Kafka client (used in Week 10; pinned now) |
| `github.com/golang-jwt/jwt/v5 v5.3.1` | RS256 JWT validation |
| `github.com/google/uuid v1.6.0` | UUID generation |
| `github.com/shopspring/decimal v1.4.0` | Precise monetary arithmetic |
| `github.com/stretchr/testify v1.11.1` | Test assertions + mocks |
| `github.com/testcontainers/testcontainers-go/modules/postgres v0.42.0` | Real Postgres in integration tests |

`validator/v10`, `gorm`, `golang-migrate`, `jackc/pgx/v5` were already present.

---

### 9.2 — `pkg/response/response.go`

Standard JSON envelope used by every handler. Shapes:

```json
// Success
{ "success": true, "data": { ... } }

// Paginated
{ "success": true, "data": [...], "meta": { "page": 1, "size": 20, "totalElements": 42, "totalPages": 3 } }

// Error
{ "success": false, "error": { "code": "NOT_FOUND", "message": "...", "timestamp": "...", "path": "..." } }
```

Helpers: `Success`, `Created`, `NoContent`, `Paginated`, `Error`, `BadRequest`, `Unauthorized`, `Forbidden`, `NotFound`, `Conflict`, `InternalError`, `TooManyRequests`.

---

### 9.3 — `pkg/jwt/jwt.go`

Two exported functions used by the auth middleware and `main.go`:

```go
func LoadPublicKey(path string) (*rsa.PublicKey, error)
func ValidateToken(tokenString string, publicKey *rsa.PublicKey) (*Claims, error)
```

`Claims` carries `UserID`, `Email`, `Role` — the same fields signed by user-service. Validation enforces RS256; any other algorithm is rejected.

---

### 9.4 — `internal/model/payment.go`

Two GORM structs mirroring the `000001_baseline_schema.up.sql` migration exactly.

**`Payment`**
| Field | Type | Notes |
|---|---|---|
| `ID` | `uuid.UUID` | Primary key |
| `OrderID` | `uuid.UUID` | Unique — one payment per order |
| `UserID` | `uuid.UUID` | Indexed |
| `Amount` | `decimal.Decimal` | `numeric(10,2)` |
| `Currency` | `string` | `char(3)`, default `USD` |
| `Status` | `PaymentStatus` | PG enum: `PENDING / COMPLETED / FAILED / REFUNDED` |
| `Method` | `PaymentMethod` | PG enum: `MOCK_CARD / MOCK_WALLET` |
| `IdempotencyKey` | `string` | Unique index — the duplicate-prevention key |
| `GatewayReference` | `string` | TX ID returned by the mock gateway |

**`PaymentHistory`** — append-only audit log: `id (BIGSERIAL)`, `payment_id`, `old_status` (nullable), `new_status`, `reason`, `created_at`.

Both enum types implement `driver.Valuer` and `sql.Scanner` so GORM can read/write PostgreSQL custom enum columns without casting.

---

### 9.5 — `internal/dto/payment_dto.go`

| Type | Used for |
|---|---|
| `PaymentResponse` | All read responses (GET endpoints, POST 201 body) |
| `ProcessPaymentRequest` | POST `/api/v1/payments` body |
| `PaymentHistoryEntry` | Audit queries (future) |

`ToPaymentResponse(p *model.Payment) PaymentResponse` — converts a model to its wire shape. All handlers call this instead of exposing GORM structs.

---

### 9.6 — `internal/repository/payment_repository.go`

**Interface**

```go
type PaymentRepository interface {
    Create(ctx, p *model.Payment, h *model.PaymentHistory) error
    FindByIdempotencyKey(ctx, key string) (*model.Payment, error)
    FindByID(ctx, id uuid.UUID) (*model.Payment, error)
    FindByOrderID(ctx, orderID uuid.UUID) (*model.Payment, error)
    ListByUserID(ctx, userID uuid.UUID, limit, offset int) ([]model.Payment, int64, error)
    UpdateStatus(ctx, paymentID uuid.UUID, newStatus, gatewayRef, reason string) error
}
```

**Sentinel errors**
```go
var ErrDuplicateIdempotencyKey = errors.New("payment: duplicate idempotency key")
var ErrNotFound                = errors.New("payment: not found")
```

**`Create`** — runs a single DB transaction:
1. `INSERT INTO payments` — on `pgconn.PgError` code `23505` (unique violation), returns `ErrDuplicateIdempotencyKey`.
2. `INSERT INTO payment_history` with `old_status = NULL`, `new_status = PENDING`.

**`UpdateStatus`** — runs a single DB transaction:
1. Fetches current status for the `old_status` history column.
2. `UPDATE payments SET status, gateway_reference, updated_at`.
3. `INSERT INTO payment_history`.

All methods propagate context via `db.WithContext(ctx)`.

> **Why `pgconn.PgError` not `pq.Error`:** `gorm.io/driver/postgres v1.6.0` uses `jackc/pgx/v5/stdlib` as the underlying driver. Unique-violation errors arrive as `*pgconn.PgError`, not `*pq.Error`.

---

### 9.7 — `internal/gateway/mock_gateway.go`

Simulates a real payment provider with configurable behaviour.

```go
type Gateway interface {
    Charge(ctx context.Context, amount decimal.Decimal, currency, reference string) (txnID string, err error)
}
var ErrGatewayDeclined = errors.New("gateway: payment declined")
```

**`mockGateway`** constructor reads from `config.Config`:
- `GatewaySuccessRate` (default 0.9) — probability of approval.
- `GatewayMinLatencyMs` / `GatewayMaxLatencyMs` (default 50 / 200 ms) — uniform random sleep.

Context cancellation is respected during the sleep:
```go
select {
case <-ctx.Done(): return "", ctx.Err()  // honours 5 s timeout from service layer
case <-timer.C:                          // normal path
}
```

---

### 9.8 — `internal/service/payment_service.go`

**`ProcessPayment` flow**

```
1. Build Payment{Status: PENDING} + PaymentHistory{new=PENDING}
2. repo.Create(p, h)
   └─ ErrDuplicateIdempotencyKey? → repo.FindByIdempotencyKey → return existing (no gateway call)
3. gwCtx = context.WithTimeout(ctx, 5s)   ← mandatory per proposal §4.6
   gateway.Charge(gwCtx, ...)
   ├─ success       → repo.UpdateStatus(COMPLETED, txnID, "gateway approved")
   ├─ ErrDeclined   → repo.UpdateStatus(FAILED, "", "gateway declined")
   └─ other error   → return error; payment stays PENDING (Week 11 retry/DLQ)
4. repo.FindByID → return final row
```

**Read methods** — `GetByID` / `GetByOrderID` enforce ownership: `payment.UserID != callerUserID && !isAdmin` → `ErrForbidden`. Admins bypass ownership.

**`ListByUser`** — paginated: `repo.ListByUserID(ctx, userID, size, (page-1)*size)`.

---

### 9.9 — Middleware + Handler

**Middleware order in `main.go`:**
```
Recovery → Correlation → Logger → (per-route group) Auth
```

| File | Responsibility |
|---|---|
| `correlation.go` | Reads `X-Correlation-ID` header (from Nginx), generates UUID if absent. Stores in gin context **and** in `c.Request.Context()` (Kafka workers need it in Week 10). Sets the header on the response. |
| `auth.go` | Validates Bearer RS256 JWT; sets `"userID"` (`uuid.UUID`) and `"role"` (`string`) in gin context. Applied per route group, not globally. |
| `logger.go` | One structured JSON line per request: `service`, `correlationId`, `userId`, `method`, `path`, `status`, `latencyMs`. Reads values already set by Correlation and Auth middlewares. |
| `recovery.go` | `defer recover()` → logs panic with correlation ID → returns `500` envelope. |

**Handler — `internal/handler/payment_handler.go`**

| Method | Route | Auth |
|---|---|---|
| `CreatePayment` | `POST /api/v1/payments` | None (internal) |
| `GetByID` | `GET /api/v1/payments/:id` | JWT required |
| `GetByOrderID` | `GET /api/v1/payments/order/:orderId` | JWT required |
| `ListByUser` | `GET /api/v1/payments` | JWT required |

Private helpers mirror cart-service:
- `getUserID(c) (uuid.UUID, bool)` — reads from gin context, aborts with 401 if missing.
- `isAdmin(c) bool` — checks `role == "admin"`.
- `handleError(c, err)` — maps sentinel errors to HTTP status codes.

**Error → HTTP mapping**

| Error | Status | Code |
|---|---|---|
| `service.ErrNotFound` | 404 | `NOT_FOUND` |
| `service.ErrForbidden` | 403 | `FORBIDDEN` |
| `repository.ErrDuplicateIdempotencyKey` | 409 | `DUPLICATE_PAYMENT` |
| `context.DeadlineExceeded` | 504 | `GATEWAY_TIMEOUT` |
| anything else | 500 | `INTERNAL_ERROR` |

---

### 9.10 — `cmd/server/main.go` (full wiring)

Start-up sequence:
1. `slog.SetDefault(logger.With("service", "payment-service"))` — every log line carries the service name.
2. PostgreSQL connect + `golang-migrate` run migrations.
3. Load RSA public key from `cfg.JWTPublicKeyPath`.
4. Construct: `gateway → repository → service → handler`.
5. Build Gin router with middleware stack (Recovery → Correlation → Logger).
6. Register routes:
   - `POST /api/v1/payments` — no auth
   - `GET /api/v1/payments`, `GET /api/v1/payments/:id`, `GET /api/v1/payments/order/:orderId` — auth group
7. `/health/live` — always 200.
8. `/health/ready` — pings Postgres (`sqlDB.PingContext`) and Kafka (`kafka.DialContext` with 2 s timeout). Returns 503 with per-dependency status if either is down.
9. pprof HTTP server on `:6060` in a background goroutine (must not be publicly exposed).
10. Graceful shutdown on `SIGINT`/`SIGTERM`: 30 s deadline → `srv.Shutdown` → `sqlDB.Close`.

A `// TODO Week 10` block marks where the Kafka consumer goroutine will be started.

---

### 9.11 — Tests

#### Unit tests — `internal/service/payment_service_test.go`

Run with: `go test -race ./internal/service/...`

Inline `mock.Mock` structs implement the `PaymentRepository` and `Gateway` interfaces (no code generation). Six test cases:

| Test | What it proves |
|---|---|
| `TestProcessPayment_IdempotentReplay` | On duplicate idempotency key the gateway is **never called** (`AssertNotCalled`); existing row is returned. |
| `TestProcessPayment_GatewaySuccess` | `repo.UpdateStatus` is called with `COMPLETED` + the txn ID. |
| `TestProcessPayment_GatewayDecline` | `repo.UpdateStatus` is called with `FAILED`; no error returned to caller. |
| `TestProcessPayment_GatewayTimeout` | `context.DeadlineExceeded` propagates; `UpdateStatus` is **not called** (payment stays `PENDING`). |
| `TestGetByID_OwnershipEnforced` | Non-owner, non-admin caller receives `ErrForbidden`. |
| `TestGetByID_AdminBypassesOwnership` | Admin can access any payment regardless of owner. |

All six pass under `-race`.

#### Integration test — `internal/integration/payment_idempotency_test.go`

Run with: `go test -tags=integration -race -v ./internal/integration/`

Requires Docker (testcontainers spins a `postgres:16-alpine` container per run).

**`TestConcurrentIdempotency`** — proves the idempotency strategy from ADR `locking-strategy.md §5`:

1. Spins 10 goroutines, all blocked on a start-gate channel.
2. Releases all goroutines simultaneously with `close(gate)`.
3. Each goroutine calls `repo.Create` with the **same** `idempotency_key` and the same `order_id`.
4. Asserts:
   - Exactly **1** goroutine returns `nil` (success).
   - The other **9** return `ErrDuplicateIdempotencyKey`.
   - `SELECT COUNT(*) FROM payments WHERE idempotency_key = ?` = **1**.
   - `SELECT COUNT(*) FROM payment_history JOIN payments ...` = **1**.

---

## How to Run

```bash
# 1. Normal build + vet
go build ./... && go vet ./...

# 2. Unit tests (no infrastructure needed)
go test -race ./internal/service/...

# 3. Start locally (needs Postgres)
docker compose up -d postgres
go run ./cmd/server/main.go

# 4. Smoke tests
curl localhost:8003/health/live
curl localhost:8003/health/ready

# POST (internal — no JWT)
curl -s -X POST localhost:8003/api/v1/payments \
  -H "Content-Type: application/json" \
  -d '{
    "orderId":        "00000000-0000-0000-0000-000000000001",
    "userId":         "00000000-0000-0000-0000-000000000002",
    "amount":         "99.99",
    "currency":       "USD",
    "idempotencyKey": "test-smoke-1"
  }' | jq .

# GET (JWT from user-service login)
TOKEN="<bearer token>"
curl -s -H "Authorization: Bearer $TOKEN" \
  localhost:8003/api/v1/payments/order/00000000-0000-0000-0000-000000000001 | jq .

# 5. Integration test (Docker required)
go test -tags=integration -race -v ./internal/integration/
```

---

## Deferred to Week 10 / 11

- Kafka consumer + producer wiring (`internal/kafka/`)
- Retry loop (3× exponential backoff: 100 ms → 200 ms → 400 ms)
- Dead-letter queue (`payments.dlq`)
- Consumer-lag logging + alert threshold (> 10 000 messages)
- End-to-end saga verification (`orders.created` → `payments.completed/failed` → order status)
