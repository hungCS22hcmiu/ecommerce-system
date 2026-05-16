# E-Commerce Microservices Platform

Distributed e-commerce backend built with Go, Java/Spring Boot, and Python. Six services communicate over synchronous REST and an asynchronous Kafka choreography saga, fronted by an Nginx gateway and a React 19 SPA. Includes semantic product search powered by pgvector and a sentence-transformer embedding sidecar.

## Service Map

| Service | Language | Port | Key Pattern |
|---|---|---|---|
| user-service | Go (Gin + GORM) | 8001 | Pessimistic lock · Redis sessions · RS256 JWT |
| product-service | Java/Spring Boot | 8081 | Optimistic lock · Redis cache-aside · pgvector AI search |
| cart-service | Go (Gin + GORM) | 8002 | Redis-first · WATCH/MULTI/EXEC |
| order-service | Java/Spring Boot | 8082 | Pessimistic lock · Kafka publisher |
| payment-service | Go (Gin) | 8003 | Idempotency key · Kafka saga · DLQ |
| ai-service | Python (FastAPI) | 8000 | sentence-transformers sidecar · `POST /embed` |
| frontend | React 19 + Vite → Nginx | 3001 | TanStack Query · Zustand · JWT interceptor |
| nginx | nginx:alpine | 80 | Reverse proxy · rate limiting · CORS |

All external traffic enters through **port 80** (Nginx). Services are not directly exposed in production.

## Quick Start

```bash
# 1. Configure environment
cp .env.example .env          # fill in SMTP credentials if you want email verification

# 2. Generate JWT keys (first time only)
mkdir -p user-service/keys
openssl genrsa -out user-service/keys/private.pem 2048
openssl rsa -in user-service/keys/private.pem -pubout -out user-service/keys/public.pem

# 3. Start the full stack
docker compose up --build -d

# 4. Seed sample users (admin / customer / seller — all pre-verified)
make db-seed
```

The React frontend is available at **http://localhost:3001** and the API at **http://localhost/api/v1/...**.

Sample credentials (from `script/sample_users.sql`):
- Customer: `customer@example.com` / `password123`
- Seller: `seller@example.com` / `password123`
- Admin: `admin@example.com` / `password123`

## Architecture

```
Browser → Nginx :80 → user-service   :8001  (auth, profiles)
                    → product-service:8081  (catalog, inventory, AI search)
                    → cart-service   :8002  (Redis-first cart)
                    → order-service  :8082  (orders, Kafka publisher)
                    → payment-service:8003  (Kafka consumer, saga)

product-service ──REST──▶  ai-service:8000  (embed query / write-through re-embed)

order-service   ──kafka──▶  orders.created          ──▶  payment-service
payment-service ──kafka──▶  payments.completed/failed ──▶  order-service
```

**Databases:** Single PostgreSQL instance, 5 logical databases (`ecommerce_users/products/carts/orders/payments`). Schemas auto-applied from `script/init-databases.sql` at container start.

**Redis:** sessions + JWT blacklist (user-service) · primary cart store (cart-service) · cache-aside (product-service).

**AI Search:** `ai-service` runs `all-MiniLM-L6-v2` (384 dims) locally. Products are embedded on create/update (write-through) and searchable via cosine similarity with pgvector.

## Common Commands

```bash
# Infrastructure only
make infra-up          # postgres + redis
docker compose up -d zookeeper kafka   # add Kafka for order/payment flows

# Full stack
make up                # docker compose up -d
make down              # docker compose down
make ps                # container status

# Database
make db-shell          # psql shell
make db-seed           # insert sample users
make db-nuke           # wipe all volumes and reinitialise

# Rebuild a single service after code changes
docker compose build cart-service && docker compose up -d cart-service

# Backfill product embeddings (run once after seeding)
docker compose run --rm ai-service python scripts/embed_products.py
```

## Testing

```bash
# Go services — unit tests (race detector)
cd user-service    && go test -race ./...
cd cart-service    && go test -race ./...
cd payment-service && go test -race ./...

# Go services — integration tests (requires postgres + redis running)
cd user-service    && go test -tags=integration -v -race ./internal/integration/
cd cart-service    && go test -tags=integration -v -race ./internal/integration/
cd payment-service && go test -tags=integration -v -race ./internal/integration/

# Java services (Testcontainers — no external deps needed)
cd product-service && ./mvnw test
cd order-service   && ./mvnw test

# ai-service
cd ai-service && pytest tests/

# End-to-end (full stack must be running on port 80)
bash script/e2e-test.sh          # browse → cart → order (14 assertions)
bash script/e2e-payment.sh       # Kafka saga: order → payment → confirm/cancel (12 assertions)
bash script/loadtest-orders.sh   # 100 orders at 10 concurrent, asserts 0 PENDING + 0 DLQ
bash script/perf-baseline.sh     # single-threaded latency baseline
```

## Documentation

| Document | Description |
|---|---|
| [`docs/technical/service_integration.md`](docs/technical/service_integration.md) | How all services communicate (HTTP, Kafka, JWT) — with Mermaid diagrams |
| [`docs/technical/architecture.md`](docs/technical/architecture.md) | Overall system design and data flow |
| [`docs/technical/development.md`](docs/technical/development.md) | Development environment setup guide |
| [`docs/technical/databaseMigration.md`](docs/technical/databaseMigration.md) | Migration strategy (Flyway + golang-migrate) |
| [`docs/technical/security-checklist.md`](docs/technical/security-checklist.md) | OWASP API Top 10 audit results per service |
| [`docs/technical/testing.md`](docs/technical/testing.md) | Testing strategy and coverage targets |
| [`docs/technical/convention.md`](docs/technical/convention.md) | Code style and API conventions |
| [`docs/adrs/locking-strategy.md`](docs/adrs/locking-strategy.md) | Concurrency strategy rationale per service |
| [`docs/adrs/saga-resilience.md`](docs/adrs/saga-resilience.md) | Kafka saga and DLQ design decisions |
| [`cart-service/README.md`](cart-service/README.md) | cart-service deep dive (Redis WATCH, circuit breaker, sync worker) |
| [`ai-service/README.md`](ai-service/README.md) | ai-service deep dive (model, endpoints, backfill script) |
| [`api/openapi.yaml`](api/openapi.yaml) | Full REST API contract |
| [`CLAUDE.md`](CLAUDE.md) | AI assistant context (service internals, key files, commands) |

## Environment Variables

Key variables (all 43 documented in [`.env.example`](.env.example)):

| Variable | Default | Used by |
|---|---|---|
| `PRODUCT_SERVICE_URL` | `http://product-service:8081` | cart-service, order-service |
| `AI_SERVICE_URL` | `http://ai-service:8000` | product-service |
| `KAFKA_BROKERS` | `kafka:29092` | order-service, payment-service |
| `JWT_PRIVATE_KEY_PATH` | `./keys/private.pem` | user-service |
| `JWT_PUBLIC_KEY_PATH` | `./keys/public.pem` | cart-service, payment-service, order-service |
| `SMTP_HOST/PORT/USERNAME/PASSWORD` | — | user-service (email verification) |
