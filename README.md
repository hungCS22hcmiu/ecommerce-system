# E-Commerce Microservices Platform

Distributed e-commerce backend built with Go, Java/Spring Boot, and Python. Six services communicate over synchronous REST and an asynchronous Kafka choreography saga, fronted by an Nginx gateway and a React 19 SPA. Includes semantic product search powered by pgvector, a reviews/ratings system, in-app notifications, and a seller management portal.

## Service Map

| Service | Language | Port | Key Pattern |
|---|---|---|---|
| user-service | Go (Gin + GORM) | 8001 | Bcrypt worker pool · Redis-only lockout · RS256 JWT |
| product-service | Java/Spring Boot | 8081 | Conditional UPDATE (atomic stock) · Redis cache-aside · pgvector AI search · reviews/ratings |
| cart-service | Go (Gin + GORM) | 8002 | Redis-first · WATCH/MULTI/EXEC · product-validation cache (5s TTL) |
| order-service | Java/Spring Boot | 8082 | Transactional outbox → Kafka · async stock release · notifications |
| payment-service | Go (Gin) | 8003 | Idempotency key · PENDING-resume · Kafka saga · DLQ |
| ai-service | Python (FastAPI) | 9000 | sentence-transformers sidecar · `POST /embed` |
| frontend | React 19 + Vite → Nginx | 3001 | TanStack Query · Zustand · JWT interceptor |
| nginx | nginx:alpine | 80 | Reverse proxy · rate limiting · CORS · dynamic DNS resolver |

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
Browser → Nginx :80 → user-service   :8001  (auth, profiles, public seller profile)
                    → product-service:8081  (catalog, inventory, AI search, reviews)
                    → cart-service   :8002  (Redis-first cart)
                    → order-service  :8082  (orders, notifications, transactional outbox)
                    → payment-service:8003  (Kafka consumer, saga, PENDING-resume)

product-service ──REST──▶  ai-service:9000          (embed query / write-through re-embed)
product-service ──REST──▶  order-service:8082        (review notification, fire-and-forget)

order-service   ──outbox──▶  orders.created            ──▶  payment-service
payment-service ──kafka───▶  payments.completed/failed  ──▶  order-service
```

**Databases:** Single PostgreSQL instance, 5 logical databases (`ecommerce_users/products/carts/orders/payments`). Schemas auto-applied from `script/init-databases.sql` at container start.

**Redis:** sessions + JWT blacklist (user-service) · primary cart store (cart-service) · cache-aside (product-service). **AOF (dev):** `appendfsync everysec` + `no-appendfsync-on-rewrite yes` (eliminates fsync stalls during AOF rewrite). In production tune `appendfsync` to `always` for max durability or `no` for max throughput.

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

## Pages & Routes

| Route | Description | Auth |
|---|---|---|
| `/` | Home — featured products | No |
| `/products` | Product list with keyword + AI search, pagination | No |
| `/products/:id` | Product detail — gallery, reviews, ratings | No |
| `/categories` | Category browse grid | No |
| `/categories/:slug` | Products filtered by category | No |
| `/sellers/:id` | Public seller shop page | No |
| `/cart` | Cart with item images and seller grouping | Yes |
| `/checkout` | Checkout — address selection, order summary | Yes |
| `/orders/:id/confirmation` | Order confirmation + payment polling | Yes |
| `/orders` | Order history | Yes |
| `/orders/:id` | Order detail — items, timeline, product thumbnails | Yes |
| `/profile` | Profile and address management | Yes |
| `/seller/products` | My Products — list, sort, filter, "Highest Rated" | Seller |
| `/seller/products/new` | Create product | Seller |
| `/seller/products/:id/edit` | Edit product | Seller |
| `/seller/orders` | Orders received — filter by status | Seller |
| `/seller/orders/:id` | Seller order detail | Seller |

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
| [`order-service/README.md`](order-service/README.md) | order-service deep dive (state machine, notifications, seller view) |
| [`ai-service/README.md`](ai-service/README.md) | ai-service deep dive (model, endpoints, backfill script) |
| [`api/openapi.yaml`](api/openapi.yaml) | Full REST API contract |
| [`CLAUDE.md`](CLAUDE.md) | AI assistant context (service internals, key files, commands) |

## Environment Variables

Key variables (all documented in [`.env.example`](.env.example)):

| Variable | Default | Used by |
|---|---|---|
| `PRODUCT_SERVICE_URL` | `http://product-service:8081` | cart-service, order-service |
| `ORDER_SERVICE_URL` | `http://order-service:8082` | product-service (review notifications) |
| `AI_SERVICE_URL` | `http://ai-service:9000` | product-service |
| `KAFKA_BROKERS` | `kafka:29092` | order-service, payment-service |
| `JWT_PRIVATE_KEY_PATH` | `./keys/private.pem` | user-service |
| `JWT_PUBLIC_KEY_PATH` | `./keys/public.pem` | cart-service, payment-service, order-service |
| `SMTP_HOST/PORT/USERNAME/PASSWORD` | — | user-service (email verification) |
