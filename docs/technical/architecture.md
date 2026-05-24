# Architecture

## System Overview

A distributed e-commerce platform with 5 microservices + 1 AI sidecar + 1 React frontend. Go services handle I/O-heavy concurrent workloads; Java/Spring Boot services handle complex business logic with transactions. All external traffic enters through an Nginx reverse proxy; services communicate via REST (synchronous) and Kafka (asynchronous).

## System Architecture Diagram

```
                                    ┌─────────────────────────────┐
                                    │      Browser (React SPA)     │
                                    └────────────┬────────────────┘
                                                 │ HTTP :80
                                    ┌────────────▼────────────────┐
                                    │      Nginx Reverse Proxy    │
                                    │  Rate Limit · CORS · TLS    │
                                    │  Security Headers · DNS      │
                                    └──┬──────┬────┬────┬────┬────┘
                                       │      │    │    │    │
                          ┌────────────▼┐ ┌───▼──┐ │ ┌─▼──┐ │
                          │  User Svc   │ │Product│ │ │Cart│ │
                          │  (Go/Gin)   │ │(Java) │ │ │(Go)│ │
                          │  :8001      │ │:8081  │ │ │:8002│ │
                          └──────┬──────┘ └──┬───┘ │ └─┬──┘ │
                                 │           │     │   │     │
                                 │           │ ┌───▼──┐│   ┌─▼────────┐
                                 │           │ │Order ││   │Payment   │
                                 │           │ │(Java)││   │Svc (Go)  │
                                 │           │ │:8082 ││   │:8003     │
                                 │           │ └──────┘│   └──────────┘
                                 │           │         │
                                 │       ┌───▼──────┐  │
                                 │       │AI Service│  │
                                 │       │(Python)  │  │
                                 │       │:9000     │  │
                                 │       │(internal)│  │
                                 │       └──────────┘  │
                   ┌─────────────▼─────────────────────▼──────┐
                   │     PostgreSQL 15+ (with pgvector)         │
                   │  (5 logical DBs, connection pooling)        │
                   └────────────────────────────────────────────┘
                   ┌────────────────────────────────────────────┐
                   │                Apache Kafka 3.x             │
                   │  orders.created → payments.completed/failed │
                   │  payments.dlq  (Choreography Saga + DLQ)   │
                   └────────────────────────────────────────────┘
                   ┌────────────────────────────────────────────┐
                   │                    Redis 7+                 │
                   │  Sessions · Cart · Cache · Blacklist · OTP  │
                   └────────────────────────────────────────────┘
                   ┌────────────────────────────────────────────┐
                   │          Frontend (React 19 + Vite)         │
                   │       Served by Nginx (SPA catch-all)       │
                   │       :3000 internal / :3001 dev port       │
                   └────────────────────────────────────────────┘
```

## Service Decomposition

| Service | Language | Port | Bounded Context | Why This Language |
|---|---|---|---|---|
| User Service | Go (Gin + GORM) | 8001 | Auth, profiles, addresses | I/O-bound auth; bcrypt worker pool handles concurrent logins without blocking goroutines |
| Product Service | Java/Spring Boot | 8081 | Catalog, search, inventory, reviews | Complex data model + conditional native UPDATE for atomic stock + pgvector AI search |
| Cart Service | Go (Gin + GORM) | 8002 | Shopping cart lifecycle | Most latency-sensitive; Redis-first with WATCH/MULTI/EXEC + short-lived product-validation cache |
| Order Service | Java/Spring Boot | 8082 | Order lifecycle, notifications | Complex state machine + transactional outbox + Kafka consumer/producer + async stock release |
| Payment Service | Go (Gin) | 8003 | Payment processing | I/O-bound gateway calls; idempotency key + DB UNIQUE prevents duplicate charges |
| AI Service | Python/FastAPI | 9000 | Embedding generation (sidecar) | sentence-transformers is Python-native; isolates ML model lifecycle from Java service |
| Frontend | React 19 + Vite | 3001 | UI / SPA | TypeScript + TanStack Query + Zustand; served as static assets via Nginx |

**AI Service** is a lightweight internal sidecar — no external Nginx exposure. Single `POST /embed` (and `POST /embed/batch`) endpoint used by Product Service for query and product embedding.

## Communication Patterns

### Synchronous (REST/HTTP)

| Caller | Target | Endpoint | Purpose | Failure Handling |
|---|---|---|---|---|
| Cart Service | Product Service | `GET /api/v1/products/{id}` | Validate product price + stock on add | Circuit breaker (custom 3-state) + Redis product-validation cache (5s TTL) |
| Order Service | Product Service | `POST /api/v1/inventory/{id}/reserve` | Reserve stock at order creation | Retry 3×, then fail order with 409 |
| Order Service | Product Service | `POST /api/v1/inventory/{id}/release` | Release reserved stock on cancellation | `@Async` + `@Retryable(3×, 100/200/400ms)` |
| Order Service | Product Service | (confirm deduction) | Stock deduction confirmed on payment success | Retry with DLQ fallback |
| Product Service | AI Service | `POST /embed` | Embed search query for vector similarity | Circuit breaker (Resilience4j) + fallback to keyword search |
| Product Service | Order Service | `POST /orders/notifications/internal/review` | Notify seller on new review | Fire-and-forget (blocked externally by Nginx) |

### Asynchronous (Kafka — Choreography Saga)

| Topic | Partitions | Producer | Consumer | Purpose |
|---|---|---|---|---|
| `orders.created` | 3 | Order Service (via outbox) | Payment Service | Trigger payment processing |
| `payments.completed` | 3 | Payment Service | Order Service | Confirm order, deduct stock, notify buyer |
| `payments.failed` | 3 | Payment Service | Order Service | Cancel order, release stock, notify buyer |
| `payments.dlq` | 1 | Payment Service | (monitoring) | Dead-letter queue for poison/unrecoverable events |

**Consumer groups:** `payment-service` (on `orders.created`) · `order-service` (on `payments.completed`, `payments.failed`).

### Transactional Outbox

Order Service writes `orders_outbox` rows atomically with each order creation. A dedicated `OutboxPublisher` polls every 100ms with `SELECT ... FOR UPDATE SKIP LOCKED`, publishes to `orders.created`, and marks `published_at`. A reaper job re-queues PENDING orders older than 2 min with no unpublished outbox row, ensuring at-least-once delivery without dual writes.

### Saga Flow

```
Client ──POST /orders──► Order Service
                           │
                           ├─ 1. Validate cart
                           ├─ 2. Reserve stock (sync → Product Service)
                           ├─ 3. Create order (status=PENDING) + outbox row
                           └─ 4. OutboxPublisher → "orders.created" → Kafka
                                    │
                              ┌─────▼──────┐
                              │   Kafka     │
                              └─────┬──────┘
                                    │
                           ┌────────▼─────────┐
                           │ Payment Service   │
                           │  1. Consume event │
                           │  2. Idempotency   │
                           │  3. Process pay   │
                           │  4. Publish result│
                           └────────┬─────────┘
                                    │
                              ┌─────▼──────┐
                              │   Kafka     │
                              └─────┬──────┘
                                    │
                           ┌────────▼──────────┐
                           │  Order Service     │
                           │  1. Consume result │
                           │  2. Lock order row │
                           │  3. Transition     │
                           │  4. Confirm/release│
                           │  5. Notify (@Async)│
                           └───────────────────┘
```

**Compensation on failure:**

| Failure Point | Compensation | Idempotent? |
|---|---|---|
| Stock reservation fails | Return 409 to client (order not created) | N/A |
| Payment fails | Order→CANCELLED, release reserved stock (@Async + @Retryable) | Yes |
| Order confirmation fails | Payment stays COMPLETED, retry via Kafka re-delivery | Yes (idempotency key) |
| Kafka poison event | Route to `payments.dlq` immediately | N/A |
| Notification fails | Logged to DB, does NOT block order flow | Yes |

## Databases

Single PostgreSQL 15+ instance with pgvector extension and 5 logical databases. Each service owns its database exclusively; cross-DB references are enforced at the application level only.

| Database | Owner Service | Key Tables |
|---|---|---|
| `ecommerce_users` | User Service | users, user_profiles, user_addresses, auth_tokens |
| `ecommerce_products` | Product Service | categories, products (+ embedding vector), product_images, product_reviews, stock_movements |
| `ecommerce_carts` | Cart Service | carts, cart_items |
| `ecommerce_orders` | Order Service | orders, order_items, order_status_history, notifications, orders_outbox |
| `ecommerce_payments` | Payment Service | payments, payment_history |

### Notable Schema Details

- `products.embedding` — `vector(384)` column with IVFFLAT index (`lists=100`, cosine ops) for pgvector similarity search
- `products.avg_rating` / `rating_count` — denormalized, recalculated on every review write
- `orders.status` / `order_status_history.old_status/new_status` — `VARCHAR(50)` (migrated from PostgreSQL enum in V6)
- `orders_outbox` — transactional outbox table with partial index on unpublished rows
- `payments.idempotency_key` — UNIQUE constraint (DB-enforced idempotency)

### Connection Pooling

| Service | Pool Config | Rationale |
|---|---|---|
| Go services | `MaxOpenConns=25`, `MaxIdleConns=5`, `ConnMaxLifetime=5m` | Goroutines share fewer connections efficiently |
| Java services | HikariCP: `maximumPoolSize=20`, `minimumIdle=5`, `idleTimeout=300000ms` | Thread-per-request model needs dedicated connections |

## Redis Usage

| Key Pattern | Value | TTL | Service |
|---|---|---|---|
| `session:{userId}` | User profile JSON | 30 min | User Service |
| `login_attempts:{email}` | Integer counter | 15 min (sliding) | User Service |
| `verification_code:{email}` | 6-digit OTP | 15 min | User Service |
| `reset_code:{email}` | Reset token | 15 min | User Service |
| `reset_cooldown:{email}` | Rate-limit marker | Configurable | User Service |
| `cart:{userId}` | Cart JSON with items | 30 min (extended on write) | Cart Service |
| `product:v:{productId}` | Product validation JSON | 5s | Cart Service |
| `product-service::product::{productId}` | Full product JSON | 10 min | Product Service |
| `product-service::productList::{categoryId}` | Product list JSON | 10 min | Product Service |

## Concurrency & Locking Strategy

| Service | Strategy | Why |
|---|---|---|
| User | Bcrypt worker pool (`runtime.NumCPU()` workers); Redis-only login-attempt counter | Pool prevents goroutine explosion under load; Redis counter avoids row lock on login — lockout correctness only requires atomicity, not durability of the counter |
| Product | Conditional native `UPDATE … WHERE stock_available >= qty` | Single atomic SQL statement; no version retries needed; `StockProjection` used for read-only checks to avoid dirty-check version bumps |
| Cart | Redis `WATCH/MULTI/EXEC` | Primary store is Redis; optimistic is correct for low-contention per-user writes |
| Order | `SELECT … FOR UPDATE` on order row | Catastrophic if two state transitions both succeed; lock duration is sub-ms |
| Payment | Idempotency key + DB `UNIQUE` constraint; PENDING-resume path retries gateway on re-delivery | Duplicate Kafka event delivery is the primary threat; DB constraint is the lightest correct solution |

See [locking-strategy.md](adrs/locking-strategy.md) for detailed rationale per service.

## Resilience

### Circuit Breakers

| Caller → Target | Implementation | Failure Threshold | Cool-down | Fallback |
|---|---|---|---|---|
| Cart → Product | Custom 3-state (Go) | 5 consecutive failures | 30s | Redis product-validation cache (5s TTL) |
| Product → AI Service | Resilience4j | 3 consecutive failures | 10s | Keyword search |
| Order → Product | Spring `@Retryable` | 3 retries | 100/200/400ms backoff | Fail order with 409 |

### Retry Strategy

| Context | Max Retries | Backoff |
|---|---|---|
| HTTP calls (service-to-service) | 3 | Exponential: 100ms, 200ms, 400ms |
| Kafka consumer (on processing failure) | 3 | Exponential: 100ms, 200ms, 400ms |
| Kafka consumer (after max retries) | — | Route to `payments.dlq` |
| Async stock release (@Retryable) | 3 | 100/200/400ms |

### Health Probes

| Endpoint | Probe Type | Checks |
|---|---|---|
| `GET /health/live` | Liveness | Process is running |
| `GET /health/ready` | Readiness | DB connected; Redis reachable (where applicable); Kafka connected (where applicable); ML model loaded (AI Service) |

### Graceful Shutdown

All services: stop accepting new requests → finish in-flight (30s timeout) → close DB → close Kafka → close Redis.

Payment Service holds Kafka offset uncommitted if `publishOutcome` sees PENDING status after a gateway call, forcing re-delivery on restart.

## Nginx Reverse Proxy

Single entry point for all client traffic. Pure configuration — not a custom service.

| Path Prefix | Target Upstream |
|---|---|
| `/api/v1/auth/*` | `http://user-service:8001` |
| `/api/v1/users/*` | `http://user-service:8001` |
| `/api/v1/products/*` | `http://product-service:8081` |
| `/api/v1/categories/*` | `http://product-service:8081` |
| `/api/v1/inventory/*` | `http://product-service:8081` |
| `/api/v1/cart/*` | `http://cart-service:8002` |
| `/api/v1/orders/*` | `http://order-service:8082` |
| `/api/v1/payments/*` | `http://payment-service:8003` |
| `/health/*` | `http://payment-service:8003` |
| `/` (catch-all) | `http://frontend:3000` (React SPA) |

**Rate limiting zones:**
- `api_limit`: 10 req/s per IP (general API)
- `auth_limit`: 5 req/min per IP, burst=3 (auth endpoints)

**Blocked externally (returns 403):**
- `PUT /api/v1/orders/:id/ship` and `PUT /api/v1/orders/:id/deliver` — internal only
- `POST /api/v1/inventory/:id/reserve` and `POST /api/v1/inventory/:id/release` — internal only
- `POST /api/v1/orders/notifications/internal/review` — internal only

**CORS:** `Access-Control-Allow-Origin: http://localhost:3001` — methods GET, POST, PUT, DELETE, OPTIONS — headers Authorization, Content-Type, X-Seller-Id, X-User-Id, X-Correlation-ID.

**Security headers:** `X-Content-Type-Options: nosniff` · `X-Frame-Options: DENY` · `X-XSS-Protection: 1; mode=block` · `Referrer-Policy: no-referrer` · `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`.

**DNS:** `resolver 127.0.0.11 valid=30s` + variable-based `proxy_pass` to re-resolve upstream IPs after container restarts and prevent stale-IP 502s.

## Security Overview

| Mechanism | Details |
|---|---|
| Password storage | bcrypt, cost factor 10 (docker-compose default; env-configurable via `BCRYPT_COST`) |
| Bcrypt worker pool | `runtime.NumCPU()` workers; pool full → HTTP 503 + `Retry-After: 1` |
| Access token | JWT RS256, 15-min TTL; private key in User Service only |
| Refresh token | Cryptographically random, 7-day TTL, stored hashed (SHA-256) in DB |
| Token revocation | Redis blacklist keyed by `jti` |
| RBAC roles | `ADMIN`, `SELLER`, `CUSTOMER` — enforced via JWT claim in middleware |
| Account lockout | 5 consecutive failures → Redis counter → lock; email verification required to unlock |
| Email OTP | 6-digit code, 15-min TTL, rate-limit cooldown per email |
| Rate limiting | Nginx: 10 req/s general / 5 req/min auth |
| SQL injection | Parameterized queries only (GORM, Spring Data JPA) |
| Service-to-service | Docker bridge network isolation; blocked routes via Nginx 403 |
| Correlation IDs | `X-Correlation-ID` header propagated across service calls for request tracing |

## Frontend

**Stack:** React 19 · TypeScript · Vite 8 · TanStack Query 5 · Zustand 5 · Axios · React Router 6 · Tailwind CSS 4.

| File | Role |
|---|---|
| `lib/axios.ts` | Queue-based 401 interceptor (`isRefreshing` + `failedQueue[]`); replays queued requests on token refresh; `/auth/refresh` 401 bypasses refresh → `clearAuth()` + redirect |
| `store/authStore.ts` | `accessToken` in memory (XSS-safe); `refreshToken` in localStorage |
| `features/payment/usePaymentStatus.ts` | `refetchInterval` returns `false` on terminal status (self-stopping poll) |
| `features/products/useProductAISearch.ts` | TanStack Query; `enabled: q.length >= 2`; `staleTime: 60s` |
| `features/products/useProductListInfinite` | `useInfiniteQuery` for scroll-based pagination (ProductDetailPage "More from category") |
| `components/shared/NotificationBell.tsx` | Clicks navigate: `productId` set → `/products/:id`; `orderId` set → `/orders/:id` |
| `components/shared/ReviewDialog.tsx` | Multi-item panel: all order items shown simultaneously; per-item rating + comment; single Submit |

**Key behaviors:**
- Cart has per-item checkboxes; cross-seller selection blocked with inline error
- OrderConfirmationPage removes only ordered product IDs from cart (not full `clearCart`)
- ProductDetailPage scrolls to top on `id` change; back link resolves to product's own category
- `Dockerfile` uses `npx vite build` directly (bypasses `tsc -b` strict check on test files)

## Deployment

### Containerization

- Go services: multi-stage Docker build → ~15MB alpine image
- Java services: multi-stage Docker build → ~200MB JRE alpine image
- Frontend: Node.js build stage → Nginx alpine serving static assets
- AI Service: Python slim image with sentence-transformers (~1.5 GB due to torch CPU)

### Docker Compose Startup Order

`postgres, redis, zookeeper → kafka → ai-service → product-service, order-service, user-service, cart-service, payment-service → frontend → nginx`

### Volumes

`postgres_data` · `redis_data` · `zookeeper_data` / `zookeeper_logs` · `kafka_data` · RSA keys mounted read-only from `./user-service/keys/` into services that verify JWTs.

### Cloud Target (Phase 6)

AWS EC2 + RDS (managed PostgreSQL with pgvector) + ElastiCache (managed Redis). CI/CD via GitHub Actions.
