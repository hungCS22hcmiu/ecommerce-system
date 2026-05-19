# payment-service

Go microservice that processes payments via a Kafka choreography saga. Consumes `orders.created`, charges the mock gateway, and publishes `payments.completed` or `payments.failed`. Built with Gin, GORM, and `confluent-kafka-go`.

- **Port:** 8003
- **Database:** PostgreSQL (`ecommerce_payments`)
- **Idempotency:** `UNIQUE(idempotency_key)` on `payments` table prevents double-charging on Kafka redelivery

## Running

```bash
# Dependencies: postgres + kafka must be running
docker compose up -d postgres zookeeper kafka

cd payment-service
go run ./cmd/server/main.go
```

Or via Docker Compose:
```bash
docker compose build payment-service
docker compose up -d payment-service
```

## Kafka Topics

| Topic | Direction | Description |
|---|---|---|
| `orders.created` | **Consumed** | Triggers payment processing for a new order |
| `payments.completed` | Produced | Payment succeeded → order-service confirms the order |
| `payments.failed` | Produced | Payment failed (gateway declined) → order-service cancels the order |
| `payments.dlq` | Produced | Dead-letter queue — poison pills and retry-exhausted messages |

## HTTP Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/payments` | No | Internal: direct payment trigger |
| `GET` | `/api/v1/payments` | Bearer JWT | List current user's payments |
| `GET` | `/api/v1/payments/:id` | Bearer JWT | Get payment by ID (ownership enforced) |
| `GET` | `/api/v1/payments/order/:orderId` | Bearer JWT | Get payment by order ID |
| `GET` | `/health/live` | No | Liveness probe |
| `GET` | `/health/ready` | No | Readiness probe (checks Postgres + Kafka) |

pprof is exposed on `:6060` (internal only — must not be publicly exposed).

## Resilience

**3-tier error classification:**
- **Poison** (deserialize failure) → DLQ immediately, no retry
- **Transient** (network, DB errors) → 3× retry with 100/200/400ms backoff → DLQ after exhaustion
- **Permanent decline** (`ErrGatewayDeclined`) → `payments.failed` published, no DLQ

**Idempotency:** `UNIQUE(idempotency_key)` constraint on `payments` table. Kafka's at-least-once delivery is safe — redelivered messages produce a DB unique violation, which is treated as "already processed."

**Consumer lag:** logged every 30s. `slog.Warn` if lag exceeds 10,000 messages.

**Graceful shutdown:** 30-second deadline covers consumer drain + HTTP server drain. If exceeded, logs "shutdown deadline exceeded."

See `docs/adrs/saga-resilience.md` for the full rationale.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8003` | HTTP listen port |
| `DB_HOST` | `localhost` | Postgres host |
| `DB_PORT` | `5432` | Postgres port |
| `DB_USER` | `postgres` | Postgres user |
| `DB_PASSWORD` | `postgres` | Postgres password |
| `DB_NAME` | `ecommerce_payments` | Postgres database name |
| `KAFKA_BROKERS` | `localhost:9092` | Kafka broker address (CSV) |
| `KAFKA_CONSUMER_GROUP` | `payment-service` | Consumer group ID |
| `GATEWAY_SUCCESS_RATE` | `0.9` | Mock gateway success probability (0.0–1.0) |
| `GATEWAY_MIN_LATENCY_MS` | `50` | Mock gateway min latency |
| `GATEWAY_MAX_LATENCY_MS` | `200` | Mock gateway max latency |
| `JWT_PUBLIC_KEY_PATH` | `./keys/public.pem` | RS256 public key for JWT validation |
| `ENV` | `development` | Set to `production` for Gin release mode |

## Testing

```bash
# Unit tests (no external deps)
go test -race ./internal/service/

# Integration tests (requires Docker — Testcontainers spins up Kafka + Postgres)
go test -tags=integration -race -v -timeout=120s ./internal/integration/
```

Integration test suite:
- `TestPoisonPill_RoutesToDLQ` — malformed bytes → DLQ with `errorStage="deserialize"`
- `TestRetryExhaustion_RoutesToDLQ` — always-failing service → DLQ after 3 attempts
- `TestPermanentDecline_NosDLQ` — gateway decline → `payments.failed`, not DLQ
- `TestDuplicateDelivery_Idempotency` — same message twice → exactly one payment row

## E2E and Load Tests

```bash
# End-to-end saga test (full stack must be running)
bash script/e2e-payment.sh       # 12 assertions: PENDING → CONFIRMED and PENDING → CANCELLED paths

# Load test: 100 orders, verify 0 PENDING and 0 DLQ messages
bash script/loadtest-orders.sh
```

## DLQ Inspection

```bash
# Read all DLQ messages
docker exec ecommerce-kafka kafka-console-consumer \
  --bootstrap-server localhost:29092 --topic payments.dlq \
  --from-beginning --timeout-ms 5000

# Check consumer lag
docker exec ecommerce-kafka kafka-consumer-groups \
  --bootstrap-server localhost:29092 --group payment-service --describe
```

To replay a DLQ message: base64-decode `originalValue` from the DLQ envelope and publish it back to `orders.created`. The idempotency key prevents double-charging if a payment row already exists.

## Key Files

```
cmd/server/main.go                 # wiring: DB, Kafka, router, graceful shutdown
internal/
  handler/payment_handler.go       # HTTP endpoints
  service/payment_service.go       # ProcessPayment with idempotency check
  kafka/
    consumer.go                    # poll loop, 3-tier error classification, DLQ routing
    producer.go                    # publishes completed/failed/dlq
  model/payment.go                 # GORM model with UNIQUE idempotency_key
  client/gateway_client.go         # mock payment gateway (configurable success rate)
  middleware/auth.go               # RS256 JWT validation
```
