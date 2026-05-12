# Architecture

## System Overview

A distributed e-commerce platform with 5 microservices + 1 AI sidecar. Go services handle I/O-heavy concurrent workloads; Java/Spring Boot services handle complex business logic with transactions. All services sit behind an Nginx reverse proxy and communicate via REST (synchronous) and Kafka (asynchronous).

## System Architecture Diagram

```
                                    ┌─────────────────────────────┐
                                    │         Client / UI         │
                                    └────────────┬────────────────┘
                                                 │ HTTP
                                    ┌────────────▼────────────────┐
                                    │      Nginx Reverse Proxy    │
                                    │   Rate Limiting · CORS · TLS│
                                    └────────────┬────────────────┘
                          ┌──────────┬───────────┼───────────┬──────────┐
                          │          │           │           │          │
                   ┌──────▼──────┐ ┌─▼─────────┐ ┌▼────────┐ ┌▼────────┐ ┌▼────────────┐
                   │   User Svc  │ │Product Svc │ │Cart Svc │ │Order Svc│ │Payment Svc  │
                   │   (Golang)  │ │(Java/Boot) │ │(Golang) │ │(Java)   │ │  (Golang)   │
                   │   :8001     │ │  :8081     │ │ :8002   │ │ :8082   │ │   :8003     │
                   └──────┬──────┘ └─┬────┬────┘ └┬───┬────┘ └┬───┬───┘ └┬────────┬───┘
                          │          │    │        │   │       │   │      │        │
                          │          │  ┌─▼──────────┐│       │   │      │        │
                          │          │  │ AI Service ││       │   │      │        │
                          │          │  │ (Python)   ││       │   │      │        │
                          │          │  │ :8004      ││       │   │      │        │
                          │          │  │ sidecar    ││       │   │      │        │
                          │          │  └────────────┘│       │   │      │        │
                   ┌──────▼──────────▼────────────────▼───┘   │   │      │        │
                   │     PostgreSQL 15+ (with pgvector)        │   │      │        │
                   │  (5 logical DBs, connection pooling)       │   │      │        │
                   └───────────────────────────────────────────┘   │      │        │
                                                                   │      │        │
                   ┌───────────────────────────────────────────────▼──────▼────────┘
                   │                Apache Kafka 3.x                              │
                   │  orders.created → payments.completed/failed → order confirm  │
                   │           (Choreography Saga with DLQ)                       │
                   └──────────────────────────────────────────────────────────────┘
                   ┌──────────────────────────────────────────────────────────────┐
                   │                    Redis 7+                                  │
                   │  Sessions · Cart (primary) · Cache · Blacklist · Rate Limit  │
                   └──────────────────────────────────────────────────────────────┘
```

## Service Decomposition

| Service | Language | Port | Bounded Context | Why This Language |
|---|---|---|---|---|
| User Service | Go (Gin + GORM) | 8001 | Auth, profiles, addresses | I/O-bound auth; goroutine-per-request handles 10K+ concurrent logins with <50MB RAM |
| Product Service | Java/Spring Boot | 8081 | Catalog, search, inventory | Complex data model + `@Version` optimistic locking + `@Transactional` stock operations |
| Cart Service | Go (Gin + GORM) | 8002 | Shopping cart lifecycle | Most latency-sensitive; Redis I/O-bound ops + background goroutine persistence |
| Order Service | Java/Spring Boot | 8082 | Order lifecycle, notifications | Complex state machine + multi-step transactions + Kafka consumer/producer |
| Payment Service | Go (Gin) | 8003 | Payment processing, refunds | I/O-bound gateway calls; explicit error handling forces every error path to be considered |
| AI Service | Python/FastAPI | 8004 | Embedding generation (sidecar) | sentence-transformers is Python-native; isolates ML model lifecycle |

**AI Service** is a lightweight sidecar — not a full microservice. Single `POST /embed` endpoint used by Product Service for query embedding. Not exposed through Nginx.

## Communication Patterns

### Synchronous (REST/HTTP)

| Caller | Target | Purpose | Failure Handling |
|---|---|---|---|
| Cart Service | Product Service | Validate product price + stock on add/checkout | Circuit breaker (`gobreaker`) + fallback to cached price |
| Order Service | Product Service | Reserve stock at order creation | Retry 3x, then fail order with clear error |
| Order Service | Product Service | Release stock on cancellation | Retry with DLQ fallback |
| Order Service | Product Service | Confirm stock deduction on payment success | Retry with DLQ fallback |
| Product Service | AI Service | Embed search query for vector similarity | Circuit breaker + fallback to keyword search |

### Asynchronous (Kafka — Choreography Saga)

| Topic | Producer | Consumer(s) | Purpose |
|---|---|---|---|
| `orders.created` | Order Service | Payment Service | Trigger payment processing |
| `orders.confirmed` | Order Service | — (audit) | Record order confirmation |
| `orders.cancelled` | Order Service | — (audit) | Record order cancellation |
| `payments.completed` | Payment Service | Order Service | Confirm order, deduct stock, send email |
| `payments.failed` | Payment Service | Order Service | Cancel order, release stock, send email |

### Saga Flow

```
Client ──POST /orders──► Order Service
                           │
                           ├─ 1. Validate cart
                           ├─ 2. Reserve stock (parallel, sync → Product Service)
                           ├─ 3. Create order (status=PENDING)
                           └─ 4. Publish "orders.created" to Kafka
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
| Payment fails | Order→CANCELLED, release reserved stock | Yes |
| Order confirmation fails | Payment stays completed, retry via Kafka | Yes (idempotency key) |
| Notification fails | Logged to DB, does NOT block order flow | Yes |

## Databases

Single PostgreSQL instance with 5 logical databases. Each service owns its database exclusively. Cross-DB references enforced at the application level, not by FK constraints.

| Database | Owner Service | Key Tables |
|---|---|---|
| `ecommerce_users` | User Service | users, user_profiles, user_addresses, auth_tokens |
| `ecommerce_products` | Product Service | products, categories, product_images, stock_movements |
| `ecommerce_carts` | Cart Service | carts, cart_items |
| `ecommerce_orders` | Order Service | orders, order_items, order_status_history, notifications |
| `ecommerce_payments` | Payment Service | payments, payment_history |

### Connection Pooling

| Service | Pool Config | Rationale |
|---|---|---|
| Go services | `MaxOpenConns=25`, `MaxIdleConns=5`, `ConnMaxLifetime=5m` | Goroutines share fewer connections efficiently |
| Java services | HikariCP: `maximumPoolSize=20`, `minimumIdle=5`, `idleTimeout=300000` | Thread-per-request model needs more connections |

## Redis Usage

| Key Pattern | Value | TTL | Service |
|---|---|---|---|
| `session:{userId}` | User profile JSON | 30 min | User Service |
| `blacklist:{jti}` | `"revoked"` | Matches JWT remaining lifetime | User Service |
| `login_attempts:{email}` | Integer counter | 15 min (sliding) | User Service |
| `verification:{email}` | 6-digit code | 15 min | User Service |
| `product:{productId}` | Product JSON (with stock) | 10 min | Product Service |
| `category:list` | All categories JSON | 30 min | Product Service |
| `cart:{userId}` | Cart JSON with items | 30 min (extended on write) | Cart Service |
| `idempotency:{key}` | Payment result JSON | 24 hours | Payment Service |

## Concurrency & Locking Strategy

| Service | Strategy | Why |
|---|---|---|
| User | `SELECT ... FOR UPDATE` | Write-heavy login row; lockout correctness critical; lock duration short |
| Product | `@Version` optimistic + `@Retry` | Low contention normal traffic; high throughput; retries cheap |
| Cart | Redis `WATCH/MULTI/EXEC` | Primary store is Redis; optimistic correct for low-contention per-user writes |
| Order | `SELECT ... FOR UPDATE` | Catastrophic if two state transitions both succeed; lock duration sub-ms |
| Payment | Idempotency key + DB `UNIQUE` | Duplicate event delivery is the threat; DB constraint lightest correct solution |

See [locking-strategy.md](adrs/locking-strategy.md) for detailed rationale per service.

## Resilience

### Circuit Breakers

| Caller → Target | Library | Failure Threshold | Timeout | Half-Open Probes |
|---|---|---|---|---|
| Cart → Product | `gobreaker` | 3 consecutive failures | 10s | 3 requests |
| Order → Product | Resilience4j | 50% failure rate (10 calls) | 15s | 5 requests |
| Product → AI Service | Resilience4j | 3 consecutive failures | 10s | 3 requests |

### Retry Strategy

| Context | Max Retries | Backoff |
|---|---|---|
| HTTP calls (service-to-service) | 3 | Exponential: 100ms, 200ms, 400ms |
| Kafka consumer (on processing failure) | 3 | Exponential: 100ms, 200ms, 400ms |
| Kafka consumer (after max retries) | — | Route to DLQ |
| Optimistic lock conflict | 3 | Immediate (re-read + retry) |

### Health Probes

| Endpoint | Probe Type | Checks |
|---|---|---|
| `GET /health/live` | Liveness | Process is running |
| `GET /health/ready` | Readiness | DB connected, Redis reachable, Kafka connected (where applicable) |

### Graceful Shutdown

All services: stop accepting new requests → finish in-flight (30s timeout) → close DB → close Kafka → close Redis.

## Nginx Reverse Proxy

Single entry point for all client traffic. Pure configuration, not a custom service.

| Path Prefix | Target Upstream |
|---|---|
| `/api/v1/auth/*` | `http://user-service:8001` |
| `/api/v1/users/*` | `http://user-service:8001` |
| `/api/v1/products/*` | `http://product-service:8081` |
| `/api/v1/inventory/*` | `http://product-service:8081` |
| `/api/v1/carts/*` | `http://cart-service:8002` |
| `/api/v1/orders/*` | `http://order-service:8082` |
| `/api/v1/payments/*` | `http://payment-service:8003` |

Capabilities: path-based routing, rate limiting (100 req/min per IP), CORS, TLS termination, request logging.

## Security Overview

| Mechanism | Details |
|---|---|
| Password storage | bcrypt, cost factor 12 |
| Access token | JWT RS256, 15-min TTL |
| Refresh token | Cryptographically random, 7-day TTL, stored hashed in DB |
| Token revocation | Redis blacklist keyed by `jti` |
| RBAC roles | `ADMIN`, `SELLER`, `CUSTOMER` |
| Account lockout | 5 consecutive failures → 15-min lockout |
| Email verification | 6-digit code, 15-min TTL, brute-force protected |
| Rate limiting | Nginx: 100 req/min per IP |
| SQL injection | Parameterized queries only (GORM, JPA) |
| Service-to-service | Docker network isolation + API key header |

## Deployment

### Containerization

- Go services: multi-stage Docker build → ~15MB alpine image
- Java services: multi-stage Docker build → ~200MB JRE alpine image

### Docker Compose Startup Order

`postgres, redis → kafka → services → nginx`

### Cloud Target (Phase 6)

AWS EC2 + RDS (managed PostgreSQL with pgvector) + ElastiCache (managed Redis). CI/CD via GitHub Actions.
