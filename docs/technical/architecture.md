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

## Trade-offs

This section documents the key architectural decisions and what was given up to get them.

### Microservices vs Monolith

**Chosen:** 5 separate services (Go + Java + Python), each deployed independently.

| What we gain | What we give up |
|---|---|
| Services can scale independently (cart is more latency-sensitive than user) | Network hop overhead on every cross-service call |
| Language fit per problem: Go for I/O-bound, Java for complex transactions | Distributed debugging — a single request may span 3+ services |
| Failures are contained — a payment outage doesn't kill product browsing | Eventual consistency between services; no cross-service ACID transactions |
| Teams can deploy independently | Operational burden: 5 Dockerfiles, 5 migration systems, 5 health checks |

**When a monolith would be better:** Early-stage startup with <5 engineers or <10k users/day. The operational cost here is justified only once the services have genuinely independent scaling or change rates.

---

### Go vs Java Language Split

**Rule of thumb used:** Go for I/O-bound services with high concurrency (user, cart, payment); Java/Spring Boot for services with complex domain logic and transactional state machines (product, order).

| Service | Why Go | Why Java would hurt |
|---|---|---|
| Cart | Redis WATCH/MULTI/EXEC — goroutine-per-request maps cleanly onto Redis pipelining | Spring's thread-per-request model adds thread overhead for what is essentially a Redis CRUD service |
| Payment | Gateway I/O + Kafka consume/produce — goroutines are cheap when blocked on network | JVM startup time and memory footprint for a lightweight gateway-bridge service |
| User | bcrypt worker pool — explicit goroutine management for CPU-bound work | Thread pool sizing in Spring would achieve the same, but with more boilerplate |

| Service | Why Java | Why Go would hurt |
|---|---|---|
| Order | Saga state machine + outbox + `@Async` + `@Retryable` — Spring annotations compose naturally | Go would require manually wiring retry logic, async pools, and transaction scoping |
| Product | Flyway migrations + conditional native UPDATE + Testcontainers integration tests — mature Spring Data ecosystem | Go's GORM doesn't support Testcontainers + pgvector as cleanly; pgvector Java driver is more mature |

---

### Single PostgreSQL Instance vs One DB per Service

**Chosen:** One Postgres process, 5 logical databases (separate schemas/DB names).

| What we gain | What we give up |
|---|---|
| One `docker-compose` volume, one backup target, one connection pool to configure | A single Postgres crash takes all 5 services down |
| Simpler local development — `docker compose up` gives the full stack | Can't tune Postgres per-service (e.g., different `max_connections` for cart vs product) |
| Cross-DB joins are still possible for debugging (psql admin) | Application-level FK enforcement only — no DB-enforced referential integrity across services |

**How we mitigate:** Each service's GORM/Flyway migration only touches its own DB. Cross-service references are string UUIDs in application code, never foreign keys to another DB.

**Cloud path:** Each logical DB maps cleanly to one RDS instance when the system needs to scale — the application code doesn't change, only the `DB_HOST` env var per service.

---

### Choreography Saga vs Orchestration

**Chosen:** Kafka choreography — services react to events (`orders.created` → payment-service → `payments.completed/failed` → order-service) with no central saga orchestrator.

| What we gain | What we give up |
|---|---|
| No single point of failure (no orchestrator service to go down) | The saga flow is implicit — to understand the full flow, you must trace topics across services |
| Services remain loosely coupled; adding a new step (e.g., warehouse notification) requires no change to existing services | Harder to implement complex compensations (e.g., partial rollback across 4 services) |
| Kafka handles durability and redelivery natively | Debugging a stuck saga requires correlating logs across 2+ services by `correlation-id` |

**Why not orchestration here:** The saga only has 2 hops (order → payment → order). Orchestration is worth the complexity when sagas have 4+ steps or need dynamic routing; at 2 hops it's unnecessary overhead.

---

### Transactional Outbox vs Direct Kafka Publish

**Chosen:** Order Service writes an `orders_outbox` row atomically with the order, then a separate `OutboxPublisher` polls and publishes to Kafka.

| What we gain | What we give up |
|---|---|
| Atomic: order + event commit or both roll back — no "order created but event never sent" | Extra polling loop adds ~100ms latency to event delivery |
| At-least-once delivery guaranteed even if Kafka is down at order creation time | `orders_outbox` table grows until reaped — needs `published_at` cleanup job |
| Reaper job recovers PENDING orders older than 2 min with no unpublished row | Two processes (OutboxPublisher + Reaper) must not both publish the same row — `FOR UPDATE SKIP LOCKED` prevents this |

**Alternative considered:** Direct Kafka publish inside the order TX. Rejected because the Kafka client cannot participate in a Postgres transaction — a Kafka failure after DB commit would lose the event silently.

---

### Pessimistic Lock (Order) vs Optimistic Lock vs Redis

**Each service chose its own strategy based on its contention profile:**

| Service | Strategy | Rationale |
|---|---|---|
| Order | `SELECT … FOR UPDATE` | State transitions (PENDING→CONFIRMED→SHIPPED) are catastrophic if duplicated. Lock duration is sub-ms (no external I/O inside the lock). |
| Cart | Redis `WATCH/MULTI/EXEC` | Primary store is Redis; optimistic is correct for low-contention per-user writes. No DB row to lock. |
| Product (stock) | Conditional UPDATE (`WHERE stock_available >= qty`) | Single SQL statement; concurrent updates self-serialize at the DB engine level without explicit locks or version retries. |
| User (login) | Redis INCR (no DB lock) | Bcrypt takes 100ms+; holding a row lock for that duration serializes all logins for the same email. Redis counter is best-effort but sufficient given the 5-attempt threshold. |
| Payment | DB UNIQUE constraint on `idempotency_key` | The threat is duplicate Kafka delivery, not concurrent users. A UNIQUE constraint is the cheapest correct solution. |

See [locking-strategy.md](adrs/locking-strategy.md) for deeper rationale.

---

### Redis-First Cart vs DB-Backed Cart

**Chosen:** Cart data lives primarily in Redis (`cart:{userId}`). There is no `carts` DB table used as the source of truth.

| What we gain | What we give up |
|---|---|
| Sub-millisecond reads/writes — cart is the most latency-sensitive user interaction | Cart data is lost if Redis goes down without a backup (or if the TTL expires) |
| WATCH/MULTI/EXEC gives atomic add/remove without a DB transaction | No durable audit trail of cart mutations |
| 30-min TTL cleans up abandoned carts automatically | Harder to query "all users who have product X in their cart" (no SQL) |

**Mitigation:** The 30-min TTL resets on every write (`EXPIRE` called on each mutation), so active carts stay alive. Losing an abandoned cart is an acceptable UX tradeoff.

---

### Stateless JWT (RS256) vs Opaque Session Tokens

**Chosen:** RS256 JWTs for access tokens; opaque random hex for refresh tokens.

| What we gain | What we give up |
|---|---|
| Any service can verify an access token using only the public key — no round-trip to user-service | Revocation requires a Redis blacklist check per request (O(1), but an extra hop) |
| 15-min TTL limits blast radius if a token is stolen | Short TTL means clients must implement silent refresh (handled by `lib/axios.ts` queue-based interceptor) |
| Refresh tokens stored as SHA-256 hashes — DB breach doesn't expose usable tokens | Two-token model (access + refresh) adds implementation complexity on the client |

**Why RS256 over HS256:** RS256 uses an asymmetric key pair. Only user-service holds the private key (signs tokens). All other services hold only the public key (verify tokens). A compromise of product-service cannot be used to forge tokens.

---

### pgvector (Embedded) vs Dedicated Vector Database

**Chosen:** pgvector extension inside the existing PostgreSQL instance.

| What we gain | What we give up |
|---|---|
| No additional infrastructure to operate (no Pinecone, Weaviate, Qdrant) | IVFFLAT index is approximate and requires `SET LOCAL ivfflat.probes=10` tuning for recall |
| Joins between product metadata and embeddings are a single query | Recall degrades as product count grows beyond ~1M rows without re-tuning `lists` |
| Embedding generation isolated in a Python sidecar; storage + search stay in Postgres | AI Service is ~1.5 GB Docker image (CPU torch) — slow cold start |

**When to migrate:** If the product catalog exceeds ~500k rows and recall quality drops, move embeddings to a dedicated ANN service (Weaviate/Qdrant) while keeping metadata in Postgres.

---

### Nginx as API Gateway vs Dedicated API Gateway

**Chosen:** Nginx handles routing, rate limiting, CORS, security headers, and blocking of internal routes.

| What we gain | What we give up |
|---|---|
| Zero additional services to operate — Nginx is already serving the React SPA | No built-in auth middleware, tracing plugins, or dynamic routing without custom Lua |
| Variable-based `proxy_pass` with `resolver 127.0.0.11` re-resolves upstream IPs after container restarts | Rate limiting is IP-based only — no per-user or per-token rate limiting |
| All security headers centralized in one config file | Adding a new route (e.g., a new microservice) requires an Nginx config change and reload |

**Cloud path:** Replace with AWS API Gateway or Kong when per-user rate limiting, auth delegation, or dynamic service discovery becomes a requirement.

---

### Denormalized `avg_rating` / `rating_count` vs Real-Time Aggregation

**Chosen:** Product rows store pre-computed `avg_rating` and `rating_count`, updated on every review write.

| What we gain | What we give up |
|---|---|
| `O(1)` product list query — no `GROUP BY` / subquery needed for rating | Rating data can be stale briefly during concurrent review writes |
| Supports `ratedOnly=true` filter (`WHERE rating_count > 0`) as a simple column predicate | Recalculation logic must run inside the review write transaction — review create/update/delete are slightly heavier |

**Alternative considered:** Computing on read with a materialized view. Rejected because the materialized view refresh interval would need to match user expectations (near-real-time), which is operationally complex for marginal benefit.

---

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
