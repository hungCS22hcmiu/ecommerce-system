# Phase 3 (Weeks 9–11) — Payment Service + Kafka Choreography Saga

## Context

**Why this work exists.** Phase 2 (Weeks 5–8) finished the synchronous half of the order flow: cart-service validates products, order-service creates orders and reserves stock through `ProductServiceClient`. The remaining gap is the **asynchronous payment leg** — the order is created in `PENDING`, and nothing transitions it to `CONFIRMED` (or `CANCELLED` on failure). Phase 3 closes that gap with a Kafka choreography saga.

**Current state of the saga (already built — discovered during exploration):**
- `order-service/src/main/java/.../kafka/OrderEventProducer.java` — already publishes `OrderCreatedEvent` to topic `orders.created` on order creation, keyed by `orderId`.
- `order-service/.../kafka/PaymentEventConsumer.java` — already listens on `payments.completed` (→ `CONFIRMED`) and `payments.failed` (→ `CANCELLED`), using `OrderService.updateOrderStatus(...)`.
- `order-service/.../config/KafkaConfig.java` — declares `NewTopic` beans for `orders.created`, `payments.completed`, `payments.failed` (3 partitions, 1 replica each).
- `order-service/.../OrderStateMachine.java` — guards transitions `PENDING → {CONFIRMED, CANCELLED}` etc.
- `docker-compose.yml` — Zookeeper + Kafka 7.5.0 wired with healthchecks; brokers reachable on `kafka:29092` internally and `localhost:9092` from host.
- `payment-service/migrations/000001_baseline_schema.up.sql` — `payments` table already has `idempotency_key VARCHAR(255) NOT NULL UNIQUE`, `payment_history` table for status transitions, `payment_status` and `payment_method` enums.
- `payment-service/config/config.go` — `KafkaBrokers`, `KafkaConsumerGroup`, `KafkaWorkerCount=5`, `GatewaySuccessRate=0.9`, `GatewayMin/MaxLatencyMs` already wired.
- `docs/adrs/locking-strategy.md` §5 — documents the chosen strategy: idempotency-key + DB UNIQUE, no distributed lock.

**What is missing (the entire scope of Weeks 9–11):**
- `payment-service/internal/*` is empty — no model, repository, service, handler, dto, middleware, kafka, gateway code.
- `payment-service/go.mod` has no Kafka client library.
- No `pkg/response` or `pkg/jwt` packages in payment-service yet.
- No payment endpoints in `api/openapi.yaml`.
- No DLQ topic, no retry logic, no consumer-lag logging, no graceful shutdown of consumers.

**The intended outcome at end of Week 11:**
1. A user creates an order → `OrderCreatedEvent` lands on `orders.created`.
2. payment-service consumer (5-worker pool) processes it, calls a mock gateway, persists a `Payment` row idempotently, writes a `payment_history` row.
3. payment-service producer emits either `payments.completed` (success) or `payments.failed` (gateway said no).
4. order-service confirms or cancels the order (already wired).
5. On unrecoverable error: 3 retries → DLQ (`payments.dlq`).
6. Concurrent test proves a duplicated event yields exactly one payment row.
7. Load test: 100 orders flow end-to-end without lost or duplicated payments.

**Library choice:** `github.com/segmentio/kafka-go` — pure Go, no CGO/librdkafka, simpler Dockerfile, idiomatic reader/writer API. (Alternative `confluent-kafka-go` requires librdkafka in the Alpine runtime image; not worth the friction here.)

---

## Architecture Decisions Locked In (cross-checked against ADRs)

| Decision | Choice | Source / Rationale |
|---|---|---|
| Kafka library | `segmentio/kafka-go` | Pure Go, fits Alpine multi-stage Dockerfile in `payment-service/Dockerfile`; proposal §3.7 lists `confluent-kafka-go` but accepts either |
| Idempotency mechanism | DB `UNIQUE(idempotency_key)` + INSERT-then-catch-duplicate | **ADR locking-strategy.md §5** (mandatory); proposal §4.6 code listing |
| Idempotency key value | `orderId.String()` (UUID) | Proposal §10.4 Scenario 3: "INSERT payment (idempotency_key='order-uuid')" |
| Consumer concurrency | 5 worker goroutines + **buffered channel size 100** | `KafkaWorkerCount=5` in `config/config.go`; proposal §4.6 code listing: `make(chan OrderCreatedEvent, 100)` |
| Offset commit strategy | Manual commit after successful processing (or after DLQ send for poison/exhausted) | At-least-once semantics + idempotency = the proposal's stated tradeoff (§10.4) |
| Retry policy | **3 attempts, exponential backoff 100ms → 200ms → 400ms** for transient errors only | **Proposal §12.2** (mandatory) |
| Gateway call timeout | **`context.WithTimeout(ctx, 5*time.Second)`** | **Proposal §4.6** "5s timeout prevents goroutine leaks when gateway is slow" |
| DLQ topic | `payments.dlq` | Proposal §4.6, §7.2, §12.2 all mandate DLQ after 3 retries |
| State transitions | `PENDING → COMPLETED` or `PENDING → FAILED`; both write a row to `payment_history` (old_status, new_status, reason) | Schema `payment_history` table; ADR locking-strategy §5 |
| HTTP endpoints | 3 in-scope per proposal §4.6: `POST /api/v1/payments` (Internal), `GET /api/v1/payments/{id}`, `GET /api/v1/payments/order/{orderId}` | The 4th proposal endpoint (refund) is deferred — see "Out of Scope" |
| Auth | JWT RS256 via `pkg/jwt` mirrored from user-service; admin-only check by JWT `role` claim for refund (when added) | Proposal §9.1 |
| **Correlation ID** | `X-Correlation-ID` header in HTTP, propagated into Kafka message headers, included in every log line | **Proposal §15.1** (mandatory) |
| **Structured JSON logs** | `slog` JSON handler with fields: `timestamp, level, service, correlationId, userId, method, path, status, latencyMs, message` | **Proposal §15.1** (mandatory) |
| **pprof** | Expose `net/http/pprof` on `:6060` (separate port, not public) | Proposal §11.2 |
| `/health/ready` | Checks Postgres ping + Kafka broker ping (segmentio `kafka.DialContext`) | **Proposal §12.4** (mandatory) — current main.go only checks DB |
| Mock gateway | Configurable success rate (default 0.9), latency uniform random in **50–200ms** by default | **Proposal §4.6** (sets defaults); current `config.GatewayMin/MaxLatencyMs` will be defaulted to 50/200 |

---

## Week 9 — Payment Service Core (Idempotency-First, No Kafka Yet)

**Outcome:** A working Go service with model/repo/service/handler layers, mock gateway, JWT-protected read endpoints, and a concurrent test proving the idempotency key prevents duplicate payments. Kafka comes in Week 10 — Week 9 isolates the business logic so it can be unit-tested without a broker.

### 9.1 Add dependencies (≈15 min)
- `cd payment-service && go get github.com/segmentio/kafka-go@latest` (used in Week 10; pinning now keeps go.mod stable).
- `go get github.com/golang-jwt/jwt/v5 github.com/google/uuid github.com/go-playground/validator/v10 github.com/shopspring/decimal`
- **No Redis** for payment-service (proposal §3.4 + comment in `cmd/server/main.go:59-61`).
- Verify: `go mod tidy && go build ./...`

### 9.2 `pkg/response/response.go` (≈30 min)
- Copy the envelope pattern from `user-service/pkg/response/response.go` verbatim — `Success`, `Created`, `Error`, `BadRequest`, `Unauthorized`, `Forbidden`, `NotFound`, `Conflict`, `InternalError`.
- This pkg has no service-specific logic; copy as-is.

### 9.3 `pkg/jwt/jwt.go` (≈30 min)
- Copy `cart-service/pkg/jwt/claims.go` (or the user-service equivalent if richer): RSA public key loader, `ValidateToken(tokenString, *rsa.PublicKey) (*Claims, error)`, claims struct with `UserID`, `Role`.
- Verify against `keys/public.pem` mounted via `JWT_PUBLIC_KEY_PATH`.

### 9.4 `internal/model/payment.go` (≈30 min)
- Define GORM structs matching the migration exactly:
  ```go
  type Payment struct {
      ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
      OrderID          uuid.UUID `gorm:"type:uuid;uniqueIndex"`
      UserID           uuid.UUID `gorm:"type:uuid;index"`
      Amount           decimal.Decimal `gorm:"type:numeric(12,2)"`
      Currency         string    `gorm:"type:char(3);default:USD"`
      Status           PaymentStatus `gorm:"type:payment_status"`
      Method           PaymentMethod `gorm:"type:payment_method"`
      IdempotencyKey   string    `gorm:"type:varchar(255);uniqueIndex;not null"`
      GatewayReference string    `gorm:"type:varchar(255)"`
      CreatedAt        time.Time
      UpdatedAt        time.Time
  }
  type PaymentHistory struct { ... } // BIGSERIAL id, payment_id, old_status, new_status, reason, created_at
  ```
- Use `github.com/shopspring/decimal` for money. Add to go.mod.
- Define typed enums `PaymentStatus` (PENDING/COMPLETED/FAILED/REFUNDED) and `PaymentMethod` (MOCK_CARD/MOCK_WALLET) with `Scan`/`Value` for GORM.

### 9.5 `internal/dto/payment_dto.go` (≈20 min)
- `PaymentResponse` (id, orderId, userId, amount, currency, status, method, gatewayReference, createdAt) — used by the read endpoints.
- `PaymentHistoryEntry` (oldStatus, newStatus, reason, createdAt) for audit endpoint (optional, defer if pressed).
- `ProcessPaymentRequest` for the internal POST endpoint (orderId, userId, amount, currency, idempotencyKey).

### 9.6 `internal/repository/payment_repository.go` (≈1 hr)
- Interface:
  ```go
  type PaymentRepository interface {
      Create(ctx context.Context, p *model.Payment, h *model.PaymentHistory) error
      FindByIdempotencyKey(ctx context.Context, key string) (*model.Payment, error)
      FindByID(ctx context.Context, id uuid.UUID) (*model.Payment, error)
      FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Payment, error)
      ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Payment, int64, error)
      UpdateStatus(ctx context.Context, paymentID uuid.UUID, newStatus PaymentStatus, gatewayRef, reason string) error
  }
  ```
- `Create` opens a TX → inserts payment → inserts payment_history → commits. On `pq` `unique_violation` (code `23505`) for `idempotency_key`, return a sentinel `ErrDuplicateIdempotencyKey` so the service can fetch the existing row.
- `UpdateStatus` opens a TX → updates payment row → inserts new payment_history row.
- All methods take `ctx` and use `r.db.WithContext(ctx)` (mirror `user-service/internal/repository/`).

### 9.7 `internal/gateway/mock_gateway.go` (≈45 min)
- Interface `Gateway`: `Charge(ctx, amount, currency, reference) (txnID string, err error)`.
- Implementation reads `Config.GatewaySuccessRate` (default 0.9), `GatewayMinLatencyMs` (default 50), `GatewayMaxLatencyMs` (default 200) — defaults set in `config/config.go`.
- Sleep a random duration in `[min, max]` ms (use `math/rand` seeded once), then with prob `1 - successRate` return `ErrGatewayDeclined`, else return a random tx ID like `MOCK-<uuid>`.
- Treat context cancellation correctly: respect `ctx.Done()` during the sleep (use `time.NewTimer` + `select`).

### 9.8 `internal/service/payment_service.go` (≈1.5 hr)
- Interface:
  ```go
  type PaymentService interface {
      ProcessPayment(ctx context.Context, in ProcessPaymentInput) (*model.Payment, error)
      GetByID(ctx context.Context, paymentID, userID uuid.UUID, isAdmin bool) (*dto.PaymentResponse, error)
      GetByOrderID(ctx context.Context, orderID, userID uuid.UUID, isAdmin bool) (*dto.PaymentResponse, error)
      ListByUser(ctx context.Context, userID uuid.UUID, page, size int) ([]dto.PaymentResponse, int64, error)
  }
  type ProcessPaymentInput struct {
      OrderID, UserID uuid.UUID
      Amount          decimal.Decimal
      Currency        string
      IdempotencyKey  string // MUST equal orderId.String() in saga path
  }
  ```
- `ProcessPayment` flow:
  1. Build a `Payment` row in `PENDING` state with `IdempotencyKey = in.IdempotencyKey`.
  2. Call `repo.Create(p, history{old=null, new=PENDING, reason="created from orders.created event"})`.
  3. If `ErrDuplicateIdempotencyKey`: `existing := repo.FindByIdempotencyKey(...)`; return existing (idempotent replay). **Do not re-charge the gateway.**
  4. Else: wrap the gateway call with `gwCtx, cancel := context.WithTimeout(ctx, 5*time.Second)` (proposal §4.6) → call `gateway.Charge(gwCtx, ...)`. On success → `UpdateStatus(COMPLETED, txnID, "gateway approved")`. On `ErrGatewayDeclined` → `UpdateStatus(FAILED, "", reason)`.
  5. Return the final payment row.
- `GetByID` / `GetByOrderID`: enforce `payment.UserID == userID` unless `isAdmin`.
- `ListByUser`: paginated.

### 9.9 `internal/handler/payment_handler.go` + `internal/middleware/` (≈1.5 hr)
- Middleware (port from cart-service, trim Redis-related bits):
  - `middleware/correlation.go` — **NEW**: read `X-Correlation-ID` header; if missing, generate `uuid.NewString()`. Stash on `gin.Context` and on the request `context.Context` (key type to avoid collisions). Set the same header on the response.
  - `middleware/auth.go` — RS256 JWT validation. Set `userID`, `role` on `gin.Context`.
  - `middleware/logger.go` — `slog` JSON handler. Per request, log one line with fields exactly per proposal §15.1: `timestamp, level, service="payment-service", correlationId, userId, method, path, status, latencyMs, message`.
  - `middleware/recovery.go` — panic recovery → 500 with envelope.
- Handlers (matches proposal §4.6 endpoints):
  - `POST /api/v1/payments` — **Internal** (no JWT in proposal but we still parse if present for `userId` audit). Body: `{orderId, userId, amount, currency, idempotencyKey}`. Calls `service.ProcessPayment`. Returns 201 with `PaymentResponse`. Used for direct invocation (smoke tests, manual replay) outside the saga path.
  - `GET /api/v1/payments/:id` — auth required, fetch by payment UUID, ownership check unless `role=admin`.
  - `GET /api/v1/payments/order/:orderId` — auth required, fetch by orderId, ownership check.
  - `GET /api/v1/payments` — auth required, paginated `?page=&size=`, returns `response.Paginated`.
- Each handler reads `correlationId` from context and includes it in any error logs.

### 9.10 Wire `cmd/server/main.go` (≈1 hr)
- Replace the placeholder block. Order:
  1. Load config, set up `slog.New(slog.NewJSONHandler(os.Stdout, ...))` as default. Add `service="payment-service"` as a static attr.
  2. Open Postgres + run golang-migrate (already there).
  3. Load JWT public key from `cfg.JWTPublicKeyPath`.
  4. Build `gateway` (default to 0.9 success, 50–200ms latency if env vars unset — set defaults in `config.go`), `repo`, `service`, `handler`.
  5. Build router. Middleware order: `Recovery → Correlation → Logger → (per-route) Auth`.
  6. Register routes under `/api/v1/payments` per 9.9.
  7. **`/health/live`**: trivially returns 200.
  8. **`/health/ready`**: pings Postgres (`db.Raw("SELECT 1")`), pings Kafka broker (`kafka.DialContext` with 2s timeout against `cfg.KafkaBrokers`). Returns 200 only if both succeed; otherwise 503 with envelope detailing which dep failed.
  9. Spawn a separate `http.Server` on `:6060` exposing `net/http/pprof` (proposal §11.2). Comment that this port must not be publicly exposed.
  10. **Graceful shutdown** (proposal §12.3) on SIGINT/SIGTERM:
      - `srv.Shutdown(30s ctx)` — stops accepting, drains in-flight HTTP.
      - (Week 10 will add) Cancel consumer ctx, `wg.Wait()` on workers, close producer writers.
      - `db.Close()`, log "shutdown complete".
- Leave a `// TODO Week 10: start Kafka consumer/producer here` block where Kafka wiring will go.

### 9.11 Tests (≈2 hr)
Create `internal/service/payment_service_test.go` with mocked repo/gateway:
- **Idempotent replay**: call `ProcessPayment` twice with same `IdempotencyKey`. Mock repo returns `ErrDuplicateIdempotencyKey` on second call → service returns existing row, gateway is called exactly once. Assert with `mock.AssertExpectations`.
- **Gateway success**: mock returns txnID → status moves to `COMPLETED`, history has 2 rows.
- **Gateway decline**: mock returns `ErrGatewayDeclined` → status moves to `FAILED`, history has 2 rows.
- **Gateway timeout**: mock blocks longer than 5s → service returns context-deadline error; payment remains in `PENDING` so retry/DLQ logic in Week 11 can act.

Create `internal/integration/payment_idempotency_test.go` (build tag `integration`, real Postgres):
- **Concurrent insert**: spin 10 goroutines, all calling `repo.Create` with the **same** idempotency key. Use `sync.WaitGroup` and a start-gate channel. Assert: exactly 1 row in `payments` table, exactly 1 row in `payment_history`. The other 9 must receive `ErrDuplicateIdempotencyKey`. **This is the proof of the strategy from ADR §5 and proposal §10.4 Scenario 3.**
- Run with `go test -tags=integration -race -v ./internal/integration/`.

### 9.12 Manual smoke test (≈15 min)
- `docker compose up -d postgres` → `go run ./cmd/server/main.go`
- Get a JWT from user-service (login).
- `curl -X POST localhost:8003/api/v1/payments -d '{"orderId":"...","userId":"...","amount":99.99,"currency":"USD","idempotencyKey":"test-1"}'` → verify 201 + envelope.
- `curl -H "Authorization: Bearer …" localhost:8003/api/v1/payments/order/<orderId>` → verify response envelope.

**Week 9 done when:** Concurrent idempotency test passes under `-race`. Service starts cleanly. Read endpoints return enveloped JSON. `/health/ready` reports DB+Kafka status correctly.

---

## Week 10 — Kafka Producer + Consumer (Saga Wiring)

**Outcome:** End-to-end happy path through the running stack: create order via order-service REST → see order go to `CONFIRMED` (or `CANCELLED` on simulated decline) without manual intervention.

### 10.1 Define event contracts (≈30 min)
File: `internal/kafka/event/events.go`
- `OrderCreatedEvent` — must field-match `order-service/.../OrderCreatedEvent` exactly (camelCase JSON: `orderId`, `userId`, `totalAmount`, `items[]`). Confirmed via Spring `JsonSerializer` defaults.
- `PaymentCompletedEvent { paymentId, orderId, amount }`
- `PaymentFailedEvent { paymentId (nullable), orderId, reason }`
- All UUIDs as strings on the wire; `decimal.Decimal` marshalled as JSON number.
- **Cross-language gotcha:** Spring `JsonDeserializer` with `spring.json.trusted.packages=*` accepts an extra `__TypeId__` header. We do **not** need to set it because order-service's consumer config strips type info. Verify by sending one message and watching consumer logs in 10.6.

### 10.2 Kafka writer (producer) (≈45 min)
File: `internal/kafka/producer.go`
- One `*kafka.Writer` per topic (`payments.completed`, `payments.failed`, `payments.dlq` — DLQ writer wired now even if unused until Week 11).
- Config: `RequiredAcks: kafka.RequireAll`, `BatchTimeout: 10ms`, `Async: false` (we want synchronous error feedback in the consumer worker).
- `PublishCompleted(ctx, evt, correlationId)` / `PublishFailed(ctx, evt, correlationId)` / `PublishDLQ(ctx, payload, reason, correlationId)`.
- Key = `orderId.String()` so all events for an order go to the same partition (preserves ordering with order-service consumer).
- **Headers**: every published message includes `X-Correlation-ID: <uuid>` Kafka header so order-service can pick it up and log with the same ID (proposal §15.1).

### 10.3 Kafka reader + worker pool (≈2 hr)
File: `internal/kafka/consumer.go`
- One `*kafka.Reader` for topic `orders.created`, `GroupID = cfg.KafkaConsumerGroup` (`payment-service`), `MinBytes=1`, `MaxBytes=10MB`, `CommitInterval=0` (manual commit only).
- `Consumer.Run(ctx)` loop (mirrors proposal §4.6 worker-pool listing):
  1. `msg, err := reader.FetchMessage(ctx)` — blocks until message or ctx cancel.
  2. Push msg onto **buffered channel `jobs chan kafka.Message` of size 100** (proposal §4.6).
  3. Workers (`cfg.KafkaWorkerCount = 5`) read from `jobs`. For each:
     - Read `X-Correlation-ID` from message headers; if missing, generate one. Stash on a per-message `context.Context`.
     - Deserialize JSON to `OrderCreatedEvent`.
     - Call `service.ProcessPayment(ctx, in)` with `IdempotencyKey = evt.OrderID.String()`. Internally the service wraps the gateway call with `context.WithTimeout(ctx, 5*time.Second)`.
     - On success → `producer.PublishCompleted` (or `Failed`) with correlationId → `reader.CommitMessages(ctx, msg)`.
     - On error → Week 11 retry/DLQ logic; for Week 10, log with correlationId + commit (we'll harden in 11).
  4. Each log line carries `correlationId`, `orderId`, `workerId`, `topic`, `partition`, `offset`.
- Graceful shutdown:
  - Outer `ctx` cancellation breaks the fetch loop.
  - Close `jobs` channel; workers drain remaining messages and commit each before exit.
  - `wg.Wait()` for workers (proposal §4.6 listing pattern).
  - Final `reader.Close()` and `writer.Close()`.

### 10.4 Wire consumer into `main.go` (≈30 min)
- Build `producer`, `consumer` after `service`.
- `consumerCtx, cancel := context.WithCancel(context.Background())`
- `go consumer.Run(consumerCtx)`
- In shutdown handler: `cancel()` first, then `consumer.Wait()` (a sync.WaitGroup `Wait()`), then close producer writers, then HTTP server, then DB.

### 10.5 docker-compose verification (≈15 min)
- Confirm `payment-service` block in `docker-compose.yml` already has `KAFKA_BROKERS: kafka:29092` and `depends_on: kafka: service_healthy`. ✅ already there.
- `docker compose build payment-service && docker compose up -d`.
- `docker compose logs -f payment-service` should show "consumer started, group=payment-service, topic=orders.created".

### 10.6 End-to-end happy path (≈45 min)
Script: `script/e2e-payment.sh`
1. Register + login → get JWT.
2. `POST /api/v1/orders` (through nginx or direct to order-service:8082) with one item.
3. Watch `docker compose logs payment-service` — see `OrderCreatedEvent` consumed.
4. `GET /api/v1/payments/order/<orderId>` — see status `COMPLETED` (90% of the time given `GatewaySuccessRate=0.9`).
5. `GET /api/v1/orders/<orderId>` — see status `CONFIRMED`.
6. Repeat until you see one `FAILED`/`CANCELLED` flow too.

### 10.7 Sanity test with kafka-console-consumer (≈15 min)
- `docker exec -it ecommerce-kafka kafka-console-consumer --bootstrap-server kafka:29092 --topic payments.completed --from-beginning`
- Verify JSON shape matches what order-service consumer expects.

### 10.8 Restart resilience test (≈30 min)
- Stop payment-service: `docker compose stop payment-service`.
- Create 3 orders via order-service (events queue up in `orders.created`).
- Start payment-service: `docker compose start payment-service`.
- Verify all 3 are processed (`auto-offset-reset=earliest` + manual commit means uncommitted messages are re-delivered).

### 10.9 OpenAPI docs (≈30 min)
- Append payment endpoints to `api/openapi.yaml`:
  - `POST /api/v1/payments` (internal)
  - `GET /api/v1/payments/{id}` (auth required)
  - `GET /api/v1/payments/order/{orderId}` (auth required)
  - `GET /api/v1/payments` (auth required, paginated)
- Note in description: payments are normally created via Kafka; the POST endpoint is for internal direct invocation.

**Week 10 done when:** Full happy and unhappy paths flow end-to-end. Restart resilience verified. `docker compose logs` shows clean shutdown.

---

## Week 11 — Resilience: Retry, DLQ, Lag, Graceful Shutdown

**Outcome:** Saga survives malformed messages, repeated gateway flakes, sudden shutdowns, and 100-orders-in-a-burst load. DLQ has visible failed messages with diagnostic context.

### 11.1 Classify errors at the worker boundary (≈30 min)
- In `consumer.go` worker, after deserialization + `ProcessPayment`, classify:
  - **Poison (deserialization failed)** → DLQ immediately, commit, do not retry.
  - **Transient (DB connection blip, gateway timeout, context deadline)** → retry up to 3× with backoff per 11.2.
  - **Permanent business decline (`ErrGatewayDeclined`)** → emit `payments.failed`, commit. **NOT a retry case** — the saga is healthy, the order just gets cancelled.
- Add a `classifyError(err)` helper returning `errKindPoison | errKindTransient | errKindPermanent`.

### 11.2 Implement retry loop (≈45 min)
- Inside the worker, wrap `service.ProcessPayment` in (backoff matches **proposal §12.2**: 100ms / 200ms / 400ms):
  ```go
  backoffs := []time.Duration{100*time.Millisecond, 200*time.Millisecond, 400*time.Millisecond}
  var err error
  for attempt := 0; attempt < 3; attempt++ {
      err = s.ProcessPayment(ctx, in)
      kind := classify(err)
      if err == nil || kind == permanent { break }
      if kind == poison { goto sendDLQ }
      select {
      case <-ctx.Done(): return ctx.Err()
      case <-time.After(backoffs[attempt]):
      }
  }
  if err != nil { goto sendDLQ }
  ```
- Each attempt uses the SAME idempotency key (= orderId). The DB `UNIQUE` constraint guarantees no double-charge even if attempt 1 partially succeeded — `Create` returns `ErrDuplicateIdempotencyKey` and the service replays the existing row's outcome.

### 11.3 DLQ topic + writer (≈30 min)
- Add `payments.dlq` to order-service `KafkaConfig` (or auto-create — the broker has `KAFKA_AUTO_CREATE_TOPICS_ENABLE=true`, so we can rely on first publish creating it).
- DLQ payload schema:
  ```json
  {
    "originalTopic": "orders.created",
    "originalPartition": 0,
    "originalOffset": 12345,
    "originalKey": "<orderId>",
    "originalValue": "<base64 raw bytes>",
    "errorReason": "deserialize: ...",
    "errorStage": "deserialize|process|publish",
    "attempts": 3,
    "failedAt": "2026-...",
    "correlationId": "..."
  }
  ```
- Producer writes synchronously with `RequireAll`; if DLQ publish itself fails, log loudly and **do not commit** — better to redeliver than silently lose.

### 11.4 Consumer lag logging + alert threshold (≈30 min)
- Every 30 s in a separate goroutine: read `reader.Stats()` → log `lag`, `committed offset`, `high watermark`. `slog.Info("kafka.lag", "topic", ..., "lag", stats.Lag, …)`.
- **Per proposal §15.3**: emit `slog.Warn` if lag > 10,000 messages (the documented alert threshold).
- Stop the goroutine on `ctx.Done()`.

### 11.5 Graceful shutdown of consumer (≈30 min)
- On SIGTERM: `cancel()` consumer ctx → fetch loop returns → close `jobs` channel → workers finish their current message (and commit) → `reader.Close()` flushes uncommitted offsets to broker. With manual commits we ensure each worker commits on success path before exit.
- Set a 30 s shutdown deadline (proposal §12.3); if exceeded, log "shutdown deadline exceeded" and force-close.

### 11.6 Tests for resilience (≈2 hr)

`internal/integration/payment_kafka_test.go` (build tag `integration`):

- **DLQ on poison pill:** publish a malformed JSON payload to `orders.created` directly via a `kafka.Writer` in the test → assert: a message lands in `payments.dlq` within 5 s, `errorStage="deserialize"`, no payment row created, consumer stays alive (publish a valid message right after, assert it's processed).
- **DLQ after retry exhaustion:** stub `service.ProcessPayment` to always return a transient error → publish 1 valid event → assert: payments.dlq has 1 message with `attempts=3`.
- **Permanent decline does not DLQ:** stub gateway to always decline → publish event → assert: 1 message on `payments.failed`, 0 on DLQ, payment row exists with status `FAILED`.
- **Ordering per order:** publish 3 events with the same orderId-as-key in rapid succession → assert: only 1 payment row exists (idempotency wins), exactly 1 success or 1 failure event published.

For these tests use `testcontainers-go/modules/kafka` to spin a real broker per test class. Mirror order-service's `OrderConcurrencyTest` `@EmbeddedKafka` approach but in Go.

### 11.7 Load test (≈1 hr)
Script: `script/loadtest-orders.sh`
- Bash loop creates 100 orders against order-service in 10 seconds (10 concurrent curl).
- Wait 30 s.
- Assertions (run via psql + kafka-console-consumer):
  - `SELECT COUNT(*) FROM payments` = 100
  - `SELECT COUNT(*) FROM orders WHERE status IN ('CONFIRMED', 'CANCELLED')` = 100
  - No PENDING orders left.
  - DLQ depth = 0.
  - Payment status distribution roughly 90/10 (success/fail) per `GatewaySuccessRate=0.9`.
- **NFR check (proposal §11.1)**: log p50/p99 latency for `ProcessPayment` (start at consume → publish completed). Targets are p50 < 200ms, p99 < 1s. With 50–200ms gateway latency, p99 should clear comfortably below 1s. Document numbers in `docs/load-test-results.md`.
- **NFR check (proposal §11.1)**: throughput should comfortably exceed 100 events / 30s ≈ 3.3 evt/s; the documented Kafka target is 1000 evt/s — out of scope to prove at this load, but log Kafka `reader.Stats()` to confirm no lag accumulation.

### 11.8 Documentation (≈30 min)
- Append a new section to `docs/adrs/locking-strategy.md` (or a new `docs/adrs/saga-resilience.md`) explaining: poison vs transient vs permanent, why we DLQ on poison + exhaustion only, why permanent decline emits `payments.failed` instead of DLQ.
- Update `payment-service/README.md` (or create one) with: how to run, env vars, topic list, DLQ inspection commands.

**Week 11 done when:** All resilience tests pass, load test produces clean 100/100 results, DLQ inspectable via `kafka-console-consumer`, graceful shutdown verified by `docker compose stop payment-service` showing clean log lines.

---

## Files to Create / Modify (Quick Reference)

**New (payment-service):**
- `pkg/response/response.go`
- `pkg/jwt/jwt.go`
- `internal/model/payment.go`
- `internal/dto/payment_dto.go`
- `internal/repository/payment_repository.go`
- `internal/gateway/mock_gateway.go`
- `internal/service/payment_service.go`
- `internal/handler/payment_handler.go`
- `internal/middleware/{correlation,auth,logger,recovery}.go`
- `internal/kafka/event/events.go`
- `internal/kafka/producer.go`
- `internal/kafka/consumer.go`
- `internal/service/payment_service_test.go`
- `internal/integration/payment_idempotency_test.go`
- `internal/integration/payment_kafka_test.go`

**Modify:**
- `payment-service/go.mod`, `go.sum` — add segmentio/kafka-go, golang-jwt, validator, shopspring/decimal
- `payment-service/config/config.go` — set defaults: `GatewaySuccessRate=0.9`, `GatewayMinLatencyMs=50`, `GatewayMaxLatencyMs=200`
- `payment-service/cmd/server/main.go` — full wiring (replace placeholder block, uncomment lines 107–108 area, add pprof on :6060, extend `/health/ready`)
- `api/openapi.yaml` — add payment endpoints
- `docs/adrs/locking-strategy.md` — append resilience notes (or new ADR file)

**New supporting scripts:**
- `script/e2e-payment.sh` (Week 10)
- `script/loadtest-orders.sh` (Week 11)

**No changes needed in order-service** — it's already producing and consuming the right events. (If event field names mismatch in 10.6 testing, adjust the Go event struct, not the Java side.)

---

## Verification End-to-End

After Week 11 completion, this exact sequence must succeed:

```bash
# 1. Bring up the stack
docker compose up -d postgres redis zookeeper kafka
docker compose up -d --build user-service product-service cart-service order-service payment-service

# 2. Seed: register, verify, login → JWT
TOKEN=$(curl -s -X POST localhost:8001/api/v1/auth/login -d '{"email":"customer@example.com","password":"Customer123!"}' | jq -r .data.accessToken)

# 3. Create an order (order-service publishes orders.created)
ORDER_ID=$(curl -s -X POST localhost:8082/api/v1/orders -H "Authorization: Bearer $TOKEN" \
  -d '{"cartId":"...","items":[{"productId":1,"quantity":1,"unitPrice":99.99}],"shippingAddress":{...}}' | jq -r .data.id)

# 4. Wait 2s, then check payment was created and order updated
sleep 2
curl -s localhost:8003/api/v1/payments/order/$ORDER_ID -H "Authorization: Bearer $TOKEN" | jq .data.status   # COMPLETED or FAILED
curl -s localhost:8082/api/v1/orders/$ORDER_ID -H "Authorization: Bearer $TOKEN" | jq .data.status          # CONFIRMED or CANCELLED

# 5. Verify DLQ is empty (no real failures should have leaked)
docker exec ecommerce-kafka kafka-console-consumer --bootstrap-server kafka:29092 \
  --topic payments.dlq --from-beginning --timeout-ms 3000   # should print nothing

# 6. Run integration suite
cd payment-service && go test -tags=integration -race -v ./internal/integration/

# 7. Run load test
./script/loadtest-orders.sh   # 100 orders → 100 payments, 0 PENDING orders, 0 DLQ
```

---

## Risk Register

| Risk | Likelihood | Mitigation |
|---|---|---|
| segmentio/kafka-go cross-language type mismatch with Spring JsonSerializer | Med | First-message smoke test in 10.6; if `__TypeId__` issue, add header-stripping config on order-service or matching headers on payment-service producer |
| `decimal.Decimal` JSON shape mismatch with Java `BigDecimal` (string vs number) | Med | Test in 10.7 with kafka-console-consumer; if needed, configure shopspring/decimal to marshal as string and update Spring deserializer config |
| Long-running gateway sleep blocks worker pool under load | Low | Workers=5, partitions=3 → 5 messages can be in flight; gateway max latency configurable; can raise `KafkaWorkerCount` if needed |
| Manual commit + crash mid-processing → at-least-once redelivery | Expected | This IS the design; idempotency key handles it (proven by 9.11 concurrent test) |
| DLQ writer itself fails | Low | Synchronous publish + don't commit; redelivery + alarms in real prod (out of scope here, just log loudly) |

---

## Out of Scope (for these 3 weeks)

These items appear in the proposal but are intentionally deferred. Each entry justifies why deferral is safe:

- **`POST /api/v1/payments/{id}/refund` (Admin)** — proposal §4.6 lists it. Deferred because: (a) no downstream consumer (no `payments.refunded` topic in §7.2), (b) the saga doesn't depend on it, (c) it requires gateway refund semantics not modelled in the mock. Schema already has `REFUNDED` in the `payment_status` enum, so adding the endpoint later is purely additive.
- **DLQ admin replay endpoint** — proposal §4.6 mentions "DLQ messages can be manually replayed via admin endpoint". Deferred. Week 11 documents the DLQ inspection procedure with `kafka-console-consumer`; a programmatic replay endpoint can come in Phase 4 hardening.
- **`orders.confirmed` topic** — proposal §4.5 lists this as published by order-service. Not strictly part of the saga (no consumer listed). Skip; revisit if a downstream subscriber appears.
- **Schema registry (Confluent / Apicurio)** — proposal does not require it. JSON with the `events.go` struct as the manual contract is sufficient.
- **Exactly-once Kafka semantics (transactions)** — proposal §10.4 explicitly chooses at-least-once + idempotency. Document the trade-off in the ADR addendum (§11.8).
- **Distributed tracing (OpenTelemetry)** — proposal §15 is structured logging + correlation ID only at this stage. Tracing belongs to Phase 4 (Week 14).
- **Saga compensation beyond order/payment** — the only downstream effect is stock reservation, already handled synchronously in order-service before the order enters `PENDING`.

---

## Cross-Check: Proposal & ADR Coverage

| Proposal / ADR clause | Where addressed in this plan |
|---|---|
| ADR `locking-strategy.md` §5 (idempotency-key + DB UNIQUE) | 9.6 (`Create` → catch duplicate); 9.11 concurrent insert test; 11.6 ordering test |
| Proposal §4.6 worker pool, channel buffer 100, 5 workers | 10.3 |
| Proposal §4.6 `context.WithTimeout(5s)` on gateway | 9.7 / 9.8 (service wraps gateway call); table row above |
| Proposal §4.6 mock gateway: 0.9 success, 50–200 ms latency | 9.7; defaults set in `config.go` |
| Proposal §4.6 endpoints (POST /payments, GET /payments/{id}, GET /payments/order/{orderId}) | 9.9 |
| Proposal §7.2 topics (orders.created, payments.completed, payments.failed) | 10.1, 10.2, 10.3 |
| Proposal §7.3 saga sequence | Verified end-to-end in 10.6 |
| Proposal §10.4 Scenario 3 (duplicate Kafka delivery) | 9.11 concurrent test; 11.6 ordering-per-order test |
| Proposal §11.1 SLOs (p50 < 200ms, p99 < 1s, throughput) | 11.7 NFR check |
| Proposal §11.2 pprof on :6060 | 9.10 step 9 |
| Proposal §12.2 retry 3× exp backoff 100/200/400 ms | 11.2 |
| Proposal §12.2 DLQ after max retries | 11.1, 11.3 |
| Proposal §12.3 graceful shutdown sequence | 9.10 step 10; 11.5 |
| Proposal §12.4 `/health/ready` checks DB + Kafka | 9.10 step 8 |
| Proposal §14.1 unit 70% + integration + concurrency (`-race`) | 9.11; 11.6 |
| Proposal §14.2 critical scenarios (duplicate Kafka, success E2E, failure E2E) | 9.11, 10.6, 11.6 |
| Proposal §15.1 structured JSON logs + correlation ID | 9.9 (middleware), 10.2 (Kafka headers), 10.3 (consumer reads/forwards), 9.10 (slog setup) |
| Proposal §15.3 Kafka consumer lag alert > 10k | 11.4 |
