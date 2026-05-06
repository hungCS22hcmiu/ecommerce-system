# payment-service

Go microservice that handles payments via a Kafka choreography saga.

## How to run

```bash
# Dependencies: postgres + kafka must be running
docker compose up -d postgres zookeeper kafka

# Run from repo root
cd payment-service
go run ./cmd/server/main.go
```

Or via Docker Compose (production mode):

```bash
docker compose build payment-service
docker compose up -d payment-service
```

## Required environment variables

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
| `GATEWAY_SUCCESS_RATE` | `0.9` | Mock gateway success probability |
| `GATEWAY_MIN_LATENCY_MS` | `50` | Mock gateway min latency |
| `GATEWAY_MAX_LATENCY_MS` | `200` | Mock gateway max latency |
| `JWT_PUBLIC_KEY_PATH` | `./keys/public.pem` | RS256 public key for JWT validation |
| `ENV` | `development` | Set to `production` to enable Gin release mode |

## Kafka topics

| Topic | Direction | Description |
|---|---|---|
| `orders.created` | **Consumed** | Triggers payment processing for a new order |
| `payments.completed` | Produced | Payment succeeded; order-service confirms the order |
| `payments.failed` | Produced | Payment failed (gateway declined); order-service cancels the order |
| `payments.dlq` | Produced | Dead-letter queue — poison pills and retry-exhausted messages |

## HTTP endpoints

```
POST   /api/v1/payments                  # Internal: direct payment trigger (no JWT required)
GET    /api/v1/payments                  # List authenticated user's payments (JWT required)
GET    /api/v1/payments/:id              # Get payment by ID (JWT required, ownership enforced)
GET    /api/v1/payments/order/:orderId   # Get payment by order ID (JWT required)
GET    /health/live                      # Liveness probe
GET    /health/ready                     # Readiness probe (checks Postgres + Kafka)
```

pprof is exposed on `:6060` (internal use only, must not be publicly exposed).

## Resilience

- **Retry**: transient errors on `ProcessPayment` are retried 3× with 100 ms / 200 ms / 400 ms backoff.
- **DLQ**: malformed messages and retry-exhausted messages are routed to `payments.dlq`.
- **Idempotency**: `UNIQUE(idempotency_key)` on the `payments` table prevents double-charging on Kafka redelivery.
- **Lag alert**: consumer lag is logged every 30 s; `slog.Warn` fires if lag exceeds 10,000 messages.
- **Graceful shutdown**: 30-second deadline covers consumer drain + HTTP server drain.

See `docs/adrs/saga-resilience.md` for the full rationale.

## Running tests

```bash
# Unit tests
go test -race ./internal/service/

# Integration tests (requires Docker)
go test -tags=integration -race -v -timeout=120s ./internal/integration/
```

## E2E and load tests

```bash
# End-to-end saga test (full stack must be running)
bash script/e2e-payment.sh

# Load test: 100 orders, verify 0 PENDING and 0 DLQ messages
bash script/loadtest-orders.sh
```

## DLQ inspection

```bash
# Read all DLQ messages
docker exec ecommerce-kafka kafka-console-consumer \
  --bootstrap-server localhost:29092 --topic payments.dlq \
  --from-beginning --timeout-ms 5000

# Check consumer lag
docker exec ecommerce-kafka kafka-consumer-groups \
  --bootstrap-server localhost:29092 --group payment-service --describe
```

To replay a DLQ message: base64-decode `originalValue` from the DLQ envelope and publish it
back to `orders.created`. The idempotency key prevents double-charging if a payment row
already exists for that order.
