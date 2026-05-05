# Payment Service — Implementation Plan

## Current State

- Scaffold: health check, DB connection, migrations, config — all in place
- Schema: `payments` (with `idempotency_key UNIQUE`, `order_id UNIQUE`) + `payment_history`
- order-service already produces `orders.created` and consumes `payments.completed/failed`
- **Missing from go.mod:** Kafka library (need to add `segmentio/kafka-go`)

---

## Phase 1 — Core Models, Repository, Service (Week 9)

**Goal:** Idempotent payment processing proven by tests.

| File | What |
|---|---|
| `internal/model/payment.go` | `Payment` + `PaymentHistory` GORM structs matching the migration schema |
| `internal/repository/payment_repository.go` | Interface + impl: `Create`, `FindByIdempotencyKey`, `FindByOrderID`, `FindByUserID` (paginated), `UpdateStatus` |
| `internal/service/payment_service.go` | `ProcessPayment(ctx, orderID, userID, amount, idempotencyKey)` — check idempotency key → if exists return existing → else call mock gateway → save + record history |
| `internal/service/mock_gateway.go` | Simulates payment with configurable success rate + random latency (uses `GatewaySuccessRate` from config) |
| `pkg/response/response.go` | Standard `{success, data}` / `{success: false, error}` envelope (same pattern as user-service) |

**Critical test:** Two goroutines submit identical idempotency key simultaneously → DB `UNIQUE` constraint ensures only one payment is created.

---

## Phase 2 — REST Handler (still Week 9)

| File | What |
|---|---|
| `internal/dto/payment_dto.go` | `PaymentResponse`, `PaymentListResponse` |
| `internal/handler/payment_handler.go` | `GET /api/v1/payments/:orderId` (get by order), `GET /api/v1/payments?userId=` (list, paginated) |
| `main.go` update | Uncomment `v1` group, register handler routes |

Auth: no JWT on these endpoints yet (same "gateway forwards X-User-Id header" pattern as cart-service) — JWT middleware comes in Phase 4 hardening.

---

## Phase 3 — Kafka Integration (Week 10)

Add `github.com/segmentio/kafka-go` to go.mod (simpler API than confluent for this use case).

| File | What |
|---|---|
| `internal/kafka/events.go` | `OrderCreatedEvent` + `PaymentCompletedEvent` + `PaymentFailedEvent` structs — must match order-service JSON field names exactly |
| `internal/kafka/producer.go` | `Producer` struct with `PublishPaymentCompleted(ctx, event)`, `PublishPaymentFailed(ctx, event)`, `PublishToDLQ(ctx, raw)` |
| `internal/kafka/consumer.go` | Reader loop on `orders.created` topic, manual commit after processing |
| `internal/kafka/worker.go` | Worker pool — consumer sends raw messages to a `chan kafka.Message`, N goroutines (from `cfg.KafkaWorkerCount=5`) pick up and call `PaymentService.ProcessPayment` |
| `main.go` update | Start consumer goroutine, pass cancel context, shutdown waits for consumer to stop + commit offsets |

**Saga flow:**
```
orders.created → [consumer] → processPayment()
                                ├─ success → publish payments.completed
                                └─ failure → publish payments.failed
```

---

## Phase 4 — Resilience + DLQ (Week 11)

| What | Detail |
|---|---|
| Retry loop | Worker retries `processPayment` up to 3 times on transient errors (network, DB timeout) with exponential backoff |
| Poison pill | If JSON unmarshal fails → skip retry, send raw bytes to `payments.dlq` immediately |
| DLQ | `payments.dlq` topic — dead messages land here for manual inspection |
| Consumer lag logging | On each poll cycle, log `HighWaterMark - CommittedOffset` so you can detect a falling-behind consumer |
| Graceful shutdown | SIGTERM → cancel context → worker pool drains → final manual offset commit → close reader/writer |

---

## Phase 5 — Tests

| Test | What it proves |
|---|---|
| `PaymentServiceTest` (unit) | idempotency: same key twice → first creates, second returns existing; mock gateway returns success/failure |
| `PaymentRepositoryIdempotencyTest` (integration) | 10 concurrent goroutines submit same idempotency key → exactly 1 row in DB |
| `KafkaWorkerTest` (unit) | malformed message → DLQ, no panic; retry exhaustion → DLQ after 3 attempts |
| `KafkaFlowTest` (integration, optional) | Full saga: publish `orders.created` → consumer processes → `payments.completed` appears on topic |

---

## File Layout (final)

```
payment-service/
├── cmd/server/main.go               ← update: wire Kafka + routes
├── config/config.go                 ← already done
├── migrations/                      ← already done
├── internal/
│   ├── model/payment.go
│   ├── repository/payment_repository.go
│   ├── service/
│   │   ├── payment_service.go
│   │   └── mock_gateway.go
│   ├── handler/payment_handler.go
│   ├── dto/payment_dto.go
│   └── kafka/
│       ├── events.go
│       ├── producer.go
│       ├── consumer.go
│       └── worker.go
└── pkg/response/response.go
```

---

## Build Sequence

1. **Phase 1** first — get `ProcessPayment` + idempotency working with tests before touching Kafka
2. **Phase 2** — add REST handler, smoke test with curl
3. **Phase 3** — add Kafka after the core is solid (the worker just calls the same `ProcessPayment`)
4. **Phase 4** — layer resilience on top of working Kafka
5. **Phase 5** — tests throughout, not at the end

## Key Decision: Kafka Library

Use `segmentio/kafka-go` (pure Go, simpler API) over `confluent-kafka-go` (requires CGO, heavier setup).
Fits the Go philosophy better and avoids CGO compilation issues in Docker.
