# 6-Month Execution Plan — Distributed E-Commerce Platform

**Start Date:** March 2026
**End Date:** October 2026
**Budget:** 18 hours/week × 30 weeks = **540 hours**
**Author:** Hung (with mentorship structure from senior engineer perspective)

---

## Philosophy

You already proved you can build production-grade code — the user-service is genuinely well-done. The next 6 months are about **compounding that ability** across harder problems: distributed state, async messaging, cloud ops, and AI integration.

Three rules for the whole journey:

1. **If you can't explain it on a whiteboard, you don't understand it.** Before writing code for any pattern (saga, optimistic lock, RAG), draw the sequence diagram by hand first.
2. **Write the test before asking AI for help.** If you can write the test, you understand the requirement. The implementation is the easier part.
3. **One service at a time, fully done.** Don't half-build three services. Finish one, ship it, reflect, move on.

---

## Weekly Rhythm (Every Week)

| Day | Focus | Hours | What It Looks Like |
|-----|-------|-------|--------------------|
| Day 1 | Learn | 3h | Read docs, draw diagrams, study the pattern you'll implement |
| Day 2 | Implement | 3h | Write code — models, handlers, business logic |
| Day 3 | Review + Test | 3h | Write tests, debug, refactor, reflect on what you learned |
| Day 4 | Learn | 3h | Next concept or deeper dive into current one |
| Day 5 | Implement | 3h | Continue building, wire integrations |
| Day 6 | Review + Test | 3h | Integration tests, load test, write notes, update docs |

**Rest day:** Take it. Burnout kills more projects than bad architecture.

---

## What You MUST Understand Deeply vs. What AI Can Help With

| Understand Deeply (Do Manually) | OK to Use AI Assistance |
|---|---|
| Why each locking strategy was chosen | Boilerplate CRUD handlers |
| Kafka consumer group rebalancing & offset management | Docker/Compose configuration |
| How the Saga pattern handles partial failures | Flyway migration SQL syntax |
| JWT token lifecycle (why blacklist, why refresh rotation) | OpenAPI spec writing |
| Redis data structures (why WATCH, why MULTI) | Test setup/teardown scaffolding |
| SQL transaction isolation levels | CI/CD pipeline YAML |
| How embeddings & vector similarity work | AWS CLI commands / Terraform |
| HTTP connection pooling & timeouts | CSS / frontend layout |
| What happens when a Kafka broker goes down | Monitoring dashboard config |

**The rule:** If it's a *decision* (why this approach?), understand it. If it's *syntax* (how to write this config?), AI is fine.

---

## Phase 1: Product Service (Month 1 — Weeks 1–4) ✅ DONE

### Why Product Service Next?

It's the **read-heavy, catalog service** that everything else depends on. Cart needs it for price validation. Order needs it for stock reservation. You'll also learn Spring Boot / JPA, which is different from Go — good for versatility.

### Month 1 Goals
- Fully implemented product-service with CRUD, search, inventory management
- Optimistic locking with `@Version` working under concurrent stock updates
- Redis caching for product catalog (cache-aside pattern)
- Flyway migrations managing schema evolution
- Integration tests hitting real Postgres

### Week 1 — Spring Boot + JPA Foundations ✅ DONE

**Learning Topics:**
- Spring Boot 3 project structure (controllers, services, repositories, entities)
- JPA entity lifecycle: managed → detached → merged
- Flyway migrations: why schema versioning matters (vs AutoMigrate in Go)
- Spring Data JPA: derived query methods, `@Query`, Specifications

**Implementation:**
- Write Flyway migrations for `products`, `categories`, `inventory` tables
- Create JPA entities: `Product`, `Category`, `Inventory` with proper relationships
- Implement `ProductRepository` (Spring Data JPA interface)
- Implement `ProductService` with CRUD operations
- Implement `ProductController` with REST endpoints:
  - `POST /api/v1/products` — create product (admin)
  - `GET /api/v1/products` — list with pagination + filtering
  - `GET /api/v1/products/:id` — get by ID
  - `PUT /api/v1/products/:id` — update product
  - `GET /api/v1/products/search?q=` — keyword search

**Review/Test:**
- Unit tests for service layer (mock repository)
- Test pagination edge cases (empty page, last page)
- Verify Flyway migration runs correctly on fresh DB

**Deliverable:** Product CRUD working via curl, tests passing.

### Week 2 — Inventory Management + Optimistic Locking ✅ DONE

**Learning Topics:**
- Optimistic locking: how `@Version` works in JPA (version column, `OptimisticLockException`)
- Why optimistic > pessimistic for product catalog (high read, low write contention)
- Spring `@Retryable` — automatic retry on version conflicts
- Stock reservation vs. stock deduction (two-phase approach)

**Implementation:**
- Add `@Version` to `Inventory` entity
- Implement stock reservation: `reserveStock(productId, quantity)` — checks availability, decrements with version check
- Implement stock release: `releaseStock(productId, quantity)` — for cancelled orders
- Add `@Retryable(maxAttempts=3)` on stock reservation for version conflicts
- Create `InventoryController` endpoints:
  - `POST /api/v1/inventory/:productId/reserve` — reserve stock
  - `POST /api/v1/inventory/:productId/release` — release stock
  - `GET /api/v1/inventory/:productId` — check stock level

**Review/Test:**
- **Critical test:** Write a concurrent test that fires 10 simultaneous reservation requests for a product with stock=5. Verify exactly 5 succeed, 5 fail. This is your proof that optimistic locking works.
- Test retry behavior: verify that `@Retryable` handles `OptimisticLockException`
- Understand what happens WITHOUT the `@Version` annotation (write a failing test first)

**Deliverable:** Concurrent stock reservation demo that you can explain in an interview.

### Week 3 — Redis Caching (Cache-Aside Pattern) ✅ DONE

**Learning Topics:**
- Cache-aside pattern: read → check cache → miss → load from DB → write to cache
- Cache invalidation strategies: TTL-based vs event-based
- Redis data types: String (for single product), Hash (for product fields), Sorted Set (for rankings)
- Spring Cache abstraction: `@Cacheable`, `@CacheEvict`, `@CachePut`
- When caching hurts: low-cardinality queries, write-heavy data

**Implementation:**
- Configure Spring Cache with Redis (`spring-boot-starter-data-redis`)
- Add `@Cacheable("products")` on `getProductById`
- Add `@CacheEvict` on product update/delete
- Cache product listing responses (short TTL, 2-5 minutes)
- Add cache hit/miss logging so you can observe the behavior
- Implement manual cache warming on service startup (top 100 products)

**Review/Test:**
- Verify cache hit: call getProduct twice, check Redis has the key, check DB was queried only once
- Verify cache invalidation: update product, verify next read gets fresh data
- Test cache stampede scenario: what happens when cache expires and 100 requests hit simultaneously? (understand the problem, document it — you don't need to solve it now)

**Deliverable:** Product service with Redis caching. You can demonstrate cache behavior with logs.

### Week 4 — Testing, Docs, and Hardening ✅ DONE

**Implementation:**
- Write integration tests: full Spring Boot test with `@SpringBootTest` + Testcontainers (or real Postgres)
- Add input validation on all DTOs (`@Valid`, `@NotBlank`, `@Min`, `@Max`)
- Add proper error handling with `@ControllerAdvice` (matching your response envelope)
- Update `api/openapi.yaml` with product endpoints
- Write `product-service/api.txt` with curl examples
- Health check: add `/health/ready` that checks Postgres + Redis connectivity

**Review/Test:**
- Run the full test suite, fix any flaky tests
- Verify Docker build works: `docker compose build product-service && docker compose up product-service`
- Test product service talking to Postgres in Docker
- **Reflection exercise:** Write 5 bullet points: "What I learned about Spring Boot that surprised me vs. Go"

**Milestone:** ✅ Product Service complete. All endpoints working, cached, tested, documented.

---

## Phase 2: Cart Service + Order Service (Month 2 — Weeks 5–8) ✅ DONE

### Month 2 Goals
- Cart service with Redis-first storage + PostgreSQL background sync
- Order service with state machine and pessimistic locking
- Cart → Product Service synchronous REST call working
- End-to-end flow: browse → add to cart → create order

### Week 5 — Cart Service: Redis-First Design ✅ DONE

**Learning Topics:**
- Redis as primary data store (not just cache): durability trade-offs
- Redis Hash for cart: `HSET cart:{userId} {productId} {quantity}`
- Redis WATCH/MULTI/EXEC: optimistic locking in Redis (compare to PostgreSQL @Version)
- Why Redis for cart: per-user access pattern, high write frequency, tolerance for brief inconsistency

**Implementation:**
- Cart model design in Redis: `cart:{userId}` → Hash of `productId: {quantity, price, name}`
- Implement `CartRepository` (Redis operations):
  - `AddItem(ctx, userId, item)` — WATCH key → MULTI → HSET → EXEC
  - `RemoveItem(ctx, userId, productId)` — HDEL
  - `GetCart(ctx, userId)` — HGETALL
  - `ClearCart(ctx, userId)` — DEL
  - `UpdateQuantity(ctx, userId, productId, qty)` — WATCH → MULTI → HSET → EXEC
- Implement `CartService` that calls Product Service for price validation:
  - On AddItem: HTTP GET to `product-service:8081/api/v1/products/{id}` to verify price and stock
  - Return error if product doesn't exist or is out of stock
- Implement `CartHandler` with endpoints:
  - `POST /api/v1/cart/items` — add item (protected)
  - `DELETE /api/v1/cart/items/:productId` — remove item (protected)
  - `PUT /api/v1/cart/items/:productId` — update quantity (protected)
  - `GET /api/v1/cart` — get cart (protected)
  - `DELETE /api/v1/cart` — clear cart (protected)

**Review/Test:**
- Test WATCH/MULTI/EXEC: concurrent updates to same cart item, verify no lost updates
- Test product validation: mock product service returning 404, verify cart rejects the item
- Test empty cart, cart with max items

**Deliverable:** Cart service working with Redis, validated against product service.

### Week 6 — Cart Background Sync + Auth Integration ✅ DONE

**Learning Topics:**
- Background goroutines in Go: long-running workers with graceful shutdown
- PostgreSQL as durability layer for Redis data
- Circuit breaker pattern: what to do when product-service is down
- JWT validation in a different service (sharing the public key)

**Implementation:**
- Implement PostgreSQL sync: goroutine that periodically writes cart state from Redis to Postgres
  - Frequency: every 30 seconds or on cart update (debounced)
  - Purpose: analytics, recovery if Redis loses data
- Add JWT auth middleware (reuse user-service's public key approach)
- Add HTTP client with timeout + retry for product-service calls
- Add simple circuit breaker: if product-service returns 5 errors in a row, stop calling for 30 seconds, serve from Redis cache
- Graceful shutdown: stop the sync goroutine cleanly on SIGTERM

**Review/Test:**
- Integration test: add items to cart, verify they appear in both Redis and Postgres
- Test graceful shutdown: start service, add items, send SIGTERM, verify final sync happened
- Test circuit breaker: bring down product-service, verify cart still works (degraded mode)
- Write unit tests for service layer with mocked repository

**Deliverable:** Cart service with Redis-first storage and Postgres durability.

### Week 7 — Order Service: Core + State Machine ✅ DONE

**Learning Topics:**
- Order state machine: `PENDING → CONFIRMED → SHIPPED → DELIVERED` (and `CANCELLED`, `PAYMENT_FAILED`)
- Why pessimistic locking for order state transitions (catastrophic if two transitions succeed)
- `SELECT ... FOR UPDATE` in Spring Data JPA (`@Lock(LockModeType.PESSIMISTIC_WRITE)`)
- Database constraints as safety nets (CHECK constraint on valid state transitions)

**Implementation:**
- Write Flyway migrations for `orders`, `order_items`, `order_status_history`
- Create JPA entities: `Order`, `OrderItem`, `OrderStatusHistory`
- Implement `OrderRepository` with pessimistic lock: `findByIdForUpdate(orderId)`
- Implement `OrderService`:
  - `createOrder(userId, items)` — validate stock via product-service, create order in PENDING state
  - `updateOrderStatus(orderId, newStatus)` — lock row, validate transition, update, record history
  - `cancelOrder(orderId)` — lock row, verify cancellable state, release stock
- Implement `OrderController`:
  - `POST /api/v1/orders` — create order (protected)
  - `GET /api/v1/orders` — list user's orders (protected, paginated)
  - `GET /api/v1/orders/:id` — get order detail (protected, ownership check)
  - `PUT /api/v1/orders/:id/cancel` — cancel order (protected)

**Review/Test:**
- **Critical test:** Two concurrent requests try to transition the same order from CONFIRMED → SHIPPED and CONFIRMED → CANCELLED. Verify exactly one succeeds.
- Test invalid transitions: DELIVERED → PENDING should fail
- Test order creation with insufficient stock

**Deliverable:** Order service with state machine and pessimistic locking proven by tests.

### Week 8 — End-to-End Flow: Browse → Cart → Order ✅ DONE

**Implementation:**
- Wire cart-service → product-service REST calls in Docker network
- Wire order-service → product-service stock reservation
- Test the full flow:
  1. Register user (user-service)
  2. Get JWT token (user-service)
  3. Browse products (product-service)
  4. Add to cart (cart-service, validates against product-service)
  5. Create order from cart (order-service, reserves stock)
  6. Verify stock decreased (product-service)
- Write an end-to-end test script (`script/e2e-test.sh`) that runs this flow with curl
- Fix any integration issues (network, auth, data format mismatches)

**Review/Test:**
- Run the full Docker Compose stack: `docker compose up --build`
- Execute the e2e test script
- Document any issues you found and how you fixed them
- **Reflection exercise:** "What broke during integration that unit tests didn't catch?"

**Milestone:** ✅ First end-to-end order flow working. You can demo: register → browse → add to cart → place order.

---

## Phase 3: Payment Service + Kafka Saga (Month 3 — Weeks 9–12)

### Month 3 Goals
- Payment service with idempotency pattern
- Kafka choreography saga: Order → Payment → Order status update
- Dead letter queue for failed payments
- Full async order completion flow

### Week 9 — Kafka Fundamentals + Payment Service Core ✅ DONE

**Learning Topics:**
- Kafka architecture: brokers, topics, partitions, consumer groups, offsets
- At-least-once delivery: why idempotency matters
- Consumer group rebalancing: what happens when a consumer crashes?
- Idempotency key pattern: `UNIQUE` constraint on `(idempotency_key)` column
- Draw the full saga sequence diagram by hand:
  ```
  Order Service → [orders.created] → Payment Service
  Payment Service → [payments.completed] → Order Service (→ CONFIRMED)
  Payment Service → [payments.failed] → Order Service (→ PAYMENT_FAILED, release stock)
  ```

**Implementation:**
- Write payment models: `Payment` (with `idempotency_key UNIQUE`), `PaymentEvent`
- Implement `PaymentRepository`: create payment with idempotency check
- Implement `PaymentService`:
  - `processPayment(orderId, userId, amount, idempotencyKey)` — check if already processed (by idempotency key), if not, process and record
  - Simulate payment processing (random success/failure for demo purposes)
- Implement REST endpoints:
  - `GET /api/v1/payments/:orderId` — get payment status
  - `GET /api/v1/payments` — list user's payments (protected)

**Review/Test:**
- Test idempotency: send the same payment request twice with same key, verify only one payment created
- Test concurrent submissions: two goroutines submit same idempotency key simultaneously, verify DB UNIQUE constraint catches it
- Understand the difference between application-level idempotency and database-level

**Deliverable:** Payment service core working with idempotency.

### Week 10 — Kafka Producer + Consumer Wiring ✅ DONE

**Learning Topics:**
- Go Kafka libraries: `confluent-kafka-go` or `segmentio/kafka-go` (pick one, understand trade-offs)
- Producer: serialization, partitioning, acknowledgement modes (acks=all)
- Consumer: poll loop, manual offset commit, error handling
- Message schema: what fields go in the Kafka message (orderId, userId, amount, timestamp)

**Implementation:**
- Implement Kafka producer in **order-service** (Java side):
  - On order creation, produce message to `orders.created` topic
  - Message: `{orderId, userId, totalAmount, items[], idempotencyKey, timestamp}`
- Implement Kafka consumer in **payment-service** (Go side):
  - Consumer group: `payment-service`
  - On receiving `orders.created`: call `processPayment()`
  - On success: produce to `payments.completed`
  - On failure: produce to `payments.failed`
- Implement Kafka consumer in **order-service** (Java side):
  - Listen to `payments.completed`: update order status to CONFIRMED
  - Listen to `payments.failed`: update order status to PAYMENT_FAILED, release reserved stock

**Review/Test:**
- Test happy path: create order → payment processes → order confirmed
- Test payment failure: create order → payment fails → order marked PAYMENT_FAILED → stock released
- Test Kafka consumer restart: stop payment-service, create orders, restart, verify pending orders get processed
- Monitor Kafka topics with `kafka-console-consumer`

**Deliverable:** Full saga flow working through Kafka.

### Week 11 — Error Handling, DLQ, and Resilience ✅ DONE

**Learning Topics:**
- Dead Letter Queue (DLQ): what happens when a message can't be processed after N retries
- Poison pill messages: malformed messages that crash the consumer
- Exactly-once semantics: why it's hard, why at-least-once + idempotency is the practical answer
- Consumer lag monitoring: how to know if your consumer is falling behind

**Implementation:**
- Error classification: three tiers — poison (deserialize failure → DLQ immediately), transient (retry 3× at 100/200/400ms backoffs → DLQ after exhaustion), permanent decline (`ErrGatewayDeclined` → `payments.failed`, no DLQ)
- `sendToDLQ` helper: enriches failed messages with original bytes (base64), errorStage, attempts, correlationId before routing to `payments.dlq`
- Consumer lag logger: `runLagLogger` goroutine fires every 30s, logs `lag/offset/highWaterMark`, emits `slog.Warn` if lag > 10,000
- Graceful shutdown hardening: 30-second deadline covers consumer drain; if exceeded, logs "shutdown deadline exceeded" and force-closes
- Kafka integration tests: 4 tests using `testcontainers-go/modules/kafka` — poison pill DLQ, retry exhaustion DLQ, permanent decline (no DLQ), duplicate delivery idempotency
- Load test script: `script/loadtest-orders.sh` — 100 orders at 10 concurrent, waits 30s, asserts 0 PENDING + 0 DLQ
- ADR: `docs/adrs/saga-resilience.md` — explains triage decisions, at-least-once trade-off, DLQ inspection
- Service README: `payment-service/README.md`

**Review/Test:**
- Poison pill smoke test: `THIS_IS_NOT_JSON{{{` → `payments.dlq` with `errorStage="deserialize"` ✅
- Retry exhaustion: stub service always fails → DLQ with `errorStage="process"` ✅
- E2E regression: `bash script/e2e-payment.sh` → 12/12 passing ✅
- Unit tests: `go test -race ./internal/service/` → 6/6 passing ✅

**Deliverable:** Resilient Kafka saga with DLQ, retry, lag monitoring, and graceful shutdown proven by tests.

### Week 12 — Full System Integration + Nginx

**Implementation:**
- Add Nginx reverse proxy configuration:
  - `/api/v1/auth/*` → user-service:8001
  - `/api/v1/users/*` → user-service:8001
  - `/api/v1/products/*` → product-service:8081
  - `/api/v1/cart/*` → cart-service:8002
  - `/api/v1/orders/*` → order-service:8082
  - `/api/v1/payments/*` → payment-service:8003
- Add rate limiting in Nginx (10 req/s per IP)
- Add CORS headers configuration
- Add Nginx to Docker Compose
- Update the e2e test script to go through Nginx (port 80)
- Run the complete flow through Nginx:
  1. Register → Login → Browse → Add to Cart → Create Order → Payment auto-processes → Order confirmed

**Review/Test:**
- Verify all routes work through Nginx
- Test rate limiting: send 20 rapid requests, verify throttling kicks in
- Run the complete e2e test
- **Performance baseline:** Measure response times for key operations (product list, order creation)

**Milestone:** Complete system working end-to-end through Nginx. All 5 services operational. Kafka saga proven.

---

## Phase 4: Testing, Observability, and Hardening (Month 4 — Weeks 13–16)

### Month 4 Goals
- Comprehensive test coverage across all services
- Structured logging + request tracing (correlation IDs across services)
- Load testing with k6 to find bottlenecks
- Security audit and hardening

### Week 13 — Cross-Service Testing Strategy

**Learning Topics:**
- Testing pyramid: unit → integration → contract → e2e (understand the cost/value of each layer)
- Contract testing: how to verify service-to-service API compatibility
- Test containers: disposable infrastructure for integration tests

**Implementation:**
- Add integration tests for all services (following user-service pattern):
  - product-service: Spring Boot test with real Postgres + Redis
  - cart-service: Go integration test with real Redis + mock product-service
  - order-service: Spring Boot test with real Postgres + Kafka
  - payment-service: Go integration test with real Postgres + Kafka
- Write contract tests: verify that cart-service's product-service client matches the actual API
- Ensure all tests are race-detector clean (`go test -race`)

**Deliverable:** All services have integration tests. CI-ready test suite.

### Week 14 — Observability: Logging + Tracing

**Learning Topics:**
- Structured logging: why JSON logs matter in production
- Correlation IDs: X-Correlation-ID header propagated across all services
- Distributed tracing concept (understand it — you'll add OpenTelemetry later if needed)
- Log levels and when to use each: DEBUG, INFO, WARN, ERROR

**Implementation:**
- Ensure all Go services use structured JSON logging (you already have this in user-service)
- Add X-Correlation-ID propagation:
  - Nginx generates the ID if not present
  - Each service reads it from the header, includes in all log lines
  - Each inter-service HTTP call forwards the header
  - Each Kafka message includes the correlation ID
- Add request/response logging middleware to Java services (matching Go pattern)
- Add health dashboard: simple script that polls all `/health/ready` endpoints

**Deliverable:** Correlation ID flows across all services. Logs are grep-able by request ID.

### Week 15 — Load Testing + Performance

**Learning Topics:**
- k6 load testing: write scenarios, ramp-up patterns, thresholds
- What to measure: p50, p95, p99 latency, throughput, error rate
- Connection pooling: how DB pool size affects throughput under load
- Redis pipeline: batch commands to reduce round trips

**Implementation:**
- Write k6 test scripts for key flows:
  - Product browsing (read-heavy): 100 concurrent users listing/searching products
  - Cart operations: 50 users adding/removing items simultaneously
  - Order creation: 20 concurrent orders (stresses Kafka + stock reservation)
- Run tests, collect metrics, identify bottlenecks
- Tune: adjust DB pool size, Redis pipeline usage, Gin/Spring thread pools
- Document findings: "Under 100 concurrent users, p95 for product listing is Xms"

**Deliverable:** Load test results with bottleneck analysis. Performance tuning applied.

### Week 16 — Security Hardening

**Learning Topics:**
- OWASP Top 10 for APIs: injection, broken auth, mass assignment, SSRF
- Input validation as the first line of defense
- SQL injection in JPA: why parameterized queries matter (JPA does this by default, but understand why)
- Rate limiting beyond Nginx: per-user rate limits in application layer

**Implementation:**
- Security audit checklist across all services:
  - [ ] All user input validated (DTOs with validation tags)
  - [ ] No SQL string concatenation (parameterized queries only)
  - [ ] JWT validation on all protected routes
  - [ ] Ownership checks on all user-scoped resources (order belongs to user, etc.)
  - [ ] Rate limiting on auth endpoints (login, register)
  - [ ] No secrets in code or Docker images
  - [ ] CORS properly configured (not `*` in production)
- Fix any issues found
- Add Helmet-style headers in Nginx (X-Content-Type-Options, X-Frame-Options, etc.)

**Milestone:** System is tested, observable, performant, and hardened. Production-ready (minus deployment).

---

## Phase 5: React Frontend (Month 5 — Weeks 17–20)

### Why Add a Frontend?

The backend is production-quality — but a portfolio project that no one can see without `curl` is harder to demo than one with a working UI. Four weeks of focused React work gives you a browser-accessible demo for every interview, forces you to consume your own APIs as a real client (which always reveals contract gaps), and adds a concrete full-stack story on top of the distributed backend depth you already have.

This phase is **not about becoming a frontend engineer**. It is about building enough UI to showcase the backend patterns you already know: the JWT refresh flow, the Kafka saga's async payment status, the order state machine timeline. The goal is a functional, clean UI — not a pixel-perfect design system.

### Month 5 Goals
- React 18 + TypeScript project scaffolded and dockerized
- JWT auth flow with queue-based 401 interceptor
- Product catalog with search and pagination
- Cart with optimistic updates
- Checkout → order creation → payment polling (saga visible in browser)
- Order history with status timeline
- Profile and address management

### Tech Stack

| Tool | Version | Reason |
|------|---------|--------|
| **React 18 + TypeScript** | latest | Type safety catches API contract mismatches at compile time |
| **Vite** | 5.x | Fast HMR dev server; faster Docker builds than CRA |
| **TanStack Query** | 5.x | Server state, caching, polling (`refetchInterval`) — eliminates manual useEffect+useState for every API call |
| **Zustand** | 4.x | accessToken in memory (XSS-safe), cart item count badge — two stores, zero boilerplate |
| **Axios** | 1.x | Interceptor API for 401→refresh→retry; cleaner than native fetch for this pattern |
| **React Router** | v6.4+ | `<Outlet>` for protected route wrappers; `useSearchParams` for URL-driven pagination |
| **Tailwind CSS** | 3.x | Utility-first; faster to write for a solo dev than maintaining CSS files |
| **shadcn/ui** | latest | Copies component source into repo; Tailwind-native; accessible. Use: Button, Input, Badge, Skeleton, Dialog, Table, Sheet, Toast |

**Not adding:** Redux, GraphQL, Next.js, react-hook-form (too much ceremony for this project size).

### Project Structure

```
frontend/
├── src/
│   ├── lib/
│   │   ├── axios.ts          # Axios instance + queue-based 401 interceptor
│   │   ├── queryClient.ts    # TanStack Query global config
│   │   └── utils.ts          # cn(), formatCurrency, formatDate
│   ├── store/
│   │   ├── authStore.ts      # Zustand: accessToken (memory), userId, email
│   │   └── cartStore.ts      # Zustand: itemCount for Navbar badge
│   ├── types/                # api.ts, auth.ts, product.ts, cart.ts, order.ts, payment.ts
│   ├── components/
│   │   ├── ui/               # shadcn/ui copies
│   │   ├── layout/           # Navbar, Footer, PageLayout
│   │   └── shared/           # LoadingSpinner, ErrorMessage, EmptyState, Pagination
│   ├── features/
│   │   ├── auth/             # LoginForm, RegisterForm, useAuth, authApi, ProtectedRoute
│   │   ├── products/         # ProductCard, ProductGrid, SearchBar, useProducts, productApi
│   │   ├── cart/             # CartDrawer, CartItem, useCart, useCartMutations, cartApi
│   │   ├── orders/           # OrderList, OrderDetail, OrderTimeline, CheckoutForm, orderApi
│   │   ├── payment/          # PaymentStatusPoller, usePaymentStatus, paymentApi
│   │   └── profile/          # ProfileForm, AddressManager, profileApi
│   └── pages/                # One file per route (see routes table below)
├── vite.config.ts            # proxy /api → localhost:80
└── .env.local                # VITE_API_BASE_URL=http://localhost/api/v1
```

### Pages / Routes

| Route | Page | Auth | Data Sources |
|-------|------|------|-------------|
| `/` | HomePage | No | GET /products (first 8) |
| `/login` | LoginPage | No (redirect if authed) | — |
| `/register` | RegisterPage | No | — |
| `/products` | ProductListPage | No | GET /products?page=&q= |
| `/products/:id` | ProductDetailPage | No | GET /products/:id, GET /inventory/:id |
| `/cart` | CartPage | Yes | GET /cart |
| `/checkout` | CheckoutPage | Yes | GET /cart, GET /users/profile |
| `/orders/:id/confirmation` | OrderConfirmationPage | Yes | GET /orders/:id, GET /payments/order/:id (polled) |
| `/orders` | OrderHistoryPage | Yes | GET /orders?page= |
| `/orders/:id` | OrderDetailPage | Yes | GET /orders/:id, GET /orders/:id/history |
| `/profile` | ProfilePage | Yes | GET /users/profile |

### Week 17 — Project Setup + Auth Flow

**Learning Topics:**
- React component model vs Go handlers: where does business logic live?
- TypeScript generics for `ApiResponse<T>` — one type covers every envelope shape
- Why accessToken in memory (Zustand), not localStorage: XSS threat model
- Queue-based Axios interceptor: how to hold concurrent requests during a token refresh
- React Router v6 `<Outlet>` pattern for protected route wrappers

**Implementation:**
- Scaffold: `npm create vite@latest frontend -- --template react-ts`, add all deps, configure Tailwind + shadcn/ui
- Add Vite proxy in `vite.config.ts`: `/api` → `http://localhost:80` (eliminates CORS in dev)
- Define all shared types in `src/types/` first — TypeScript guides everything else
- Implement `src/lib/axios.ts`:
  - Request interceptor: attach `Authorization: Bearer <token>` from Zustand
  - Response interceptor: on 401, set `isRefreshing = true`, call `POST /auth/refresh`, store new token, flush `failedQueue` with new token; on second 401, clear auth + redirect to `/login`
  - `failedQueue`: array of `{ resolve, reject }` — queues requests that arrive while refresh is in flight, replays them all once the new token is ready
- Implement `authStore.ts`: `accessToken` (memory only), `refreshToken` (localStorage), `userId`, `email`, `setToken()`, `clearToken()`
- Implement `authApi.ts`, `LoginForm.tsx`, `RegisterForm.tsx`
- Implement `ProtectedRoute.tsx`: check `accessToken`; if null, attempt silent refresh; if that fails, redirect to `/login?from=<path>`
- Wire `LoginPage`, `RegisterPage`, router, `Navbar` with auth-aware state

**Review/Test:**
- Register → login → navigate to `/profile` → page loads without redirect
- Token expiry: clear access token from Zustand devtools → navigate → silent refresh fires → page loads
- Network tab: `Authorization` header on every protected request
- Two simultaneous requests on stale page: both succeed (queue mechanism works)

**Deliverable:** Login, register, protected routes working. JWT interceptor with queue-based refresh proven by manual test.

### Week 18 — Product Catalog

**Learning Topics:**
- TanStack Query: `useQuery` lifecycle, `queryKey` design, `staleTime` vs `gcTime`
- URL-driven pagination: `page` and `size` in query params, `useSearchParams`
- Debounce: 300ms delay before updating search query key — avoid request-per-keystroke
- Smart (data-fetching) vs presentational (props-only) component split

**Implementation:**
- `productApi.ts`: list (paginated, filterable), get by ID, search
- `useProducts(params)`: `queryKey: ['products', params]`, `staleTime: 30_000`
- `useProduct(id)`: `staleTime: 30 * 60 * 1000` (matches backend's 30-min cache)
- `useProductSearch(query)`: `enabled: query.length >= 2`, debounced query key
- `ProductCard.tsx`: name, price, stock badge (In Stock / Low Stock / Out of Stock), Add to Cart (disabled if stock=0)
- `ProductGrid.tsx`: maps to `ProductCard`, shows 8 `<Skeleton>` cards while loading
- `Pagination.tsx`: reads `PaginationMeta` from response, renders page controls
- `ProductListPage.tsx`: `SearchBar` + `ProductGrid` + `Pagination`, URL-driven state via `useSearchParams`
- `ProductDetailPage.tsx`: description, price, stock, quantity selector, Add to Cart button

**Review/Test:**
- Navigate product list → view detail → back → no network request (TanStack Query cache hit)
- Search: 1 character → no request; 2 characters → request fires after 300ms
- Skeleton loading appears before data; never a blank page
- Out of Stock product: Add to Cart disabled
- Copy page 2 URL → open new tab → page 2 loads directly (URL-driven state)

**Deliverable:** Full product catalog browsable with search, pagination, and detail page.

### Week 19 — Cart + Checkout + Order + Payment Polling

This is the most technically complex week. Three features connect in sequence; the payment polling pattern is unique to this system.

**Learning Topics:**
- TanStack Query optimistic updates: `onMutate` snapshot → optimistic cache write → `onError` rollback → `onSettled` invalidate
- `refetchInterval` as a function: return `false` to stop, a number to continue — stop when payment status is terminal
- The order saga from the browser's perspective: POST /orders returns immediately PENDING; frontend polls GET /payments/order/:id until terminal
- `useMutation` (imperative, user action) vs `useQuery` (declarative, always-up-to-date)

**Implementation:**

*Cart (Days 1–2):*
- `cartApi.ts`: GET cart, POST add item, PUT update qty, DELETE item, DELETE cart
- `useCart()`: `queryKey: ['cart']`, `enabled: !!accessToken`
- `useCartMutations()`:
  - `addItem`: optimistic add to cached cart in `onMutate`; snapshot returned for rollback; `cartStore.setItemCount` updated immediately for Navbar badge; `onError` rolls back; `onSettled` invalidates `['cart']`
  - `updateQuantity`, `removeItem`: same optimistic pattern
- `CartDrawer.tsx`: shadcn/ui Sheet (slide-over), triggered by Navbar cart icon, shows `CartItem` list + "Proceed to Checkout"
- Wire "Add to Cart" on `ProductCard` and `ProductDetailPage` to `useCartMutations().addItem`

*Checkout + Order Creation (Days 3–4):*
- `orderApi.ts`: POST create, GET list, GET by ID, PUT cancel, GET history
- `CheckoutPage.tsx`: cart summary, address selector from `GET /users/profile` (radio group of saved addresses), "Place Order" calls `POST /orders`
- `useCreateOrder()`: `useMutation`; on `onSuccess`, invalidate `['cart']`, navigate to `/orders/:id/confirmation`

*Payment Polling + Confirmation (Days 5–6):*
- `paymentApi.ts`: GET by order ID, GET list
- `usePaymentStatus(orderId)`:
  ```typescript
  const TERMINAL = ['CONFIRMED', 'PAYMENT_FAILED', 'CANCELLED'];
  useQuery({
    queryKey: ['payment', 'order', orderId],
    queryFn: () => paymentApi.getByOrderId(orderId),
    refetchInterval: (query) => {
      const status = query.state.data?.data?.status;
      return TERMINAL.includes(status) ? false : 2000;
    },
    refetchIntervalInBackground: true,
  })
  ```
- `PaymentStatusPoller.tsx`: spinner while PENDING, success state on CONFIRMED, error state on PAYMENT_FAILED
- `OrderConfirmationPage.tsx`: order summary + `PaymentStatusPoller`; auto-redirects to order detail 3s after terminal status

**Review/Test:**
- Optimistic update: click Add to Cart → Navbar badge increments before network response
- Optimistic rollback: disable network → click Add to Cart → cart reverts on error
- Polling: place order → watch confirmation page → status changes PENDING → CONFIRMED without page refresh
- Polling stop: once terminal, Network tab shows no more GET /payments/order/:id requests

**Deliverable:** Complete purchase flow in browser. Add to cart → checkout → place order → payment polled → confirmation shown. The Kafka saga is visible.

### Week 20 — Order History + Profile + Polish

**Learning Topics:**
- React error boundaries: query `error` state vs `<ErrorBoundary>` — when to use each
- Loading state hierarchy: page Suspense vs component Skeleton vs inline spinner
- Accessible UI: focus management after form submit, `aria-label` on icon buttons
- Production build: `npm run build` must have zero TypeScript errors

**Implementation:**

*Order History + Detail (Days 1–2):*
- `useOrders(params)`: paginated, `queryKey: ['orders', params]`
- `useOrder(id)`: single order with history
- `OrderList.tsx`: shadcn/ui Table — ID (truncated), status Badge, total, date, View link
- `OrderTimeline.tsx`: vertical timeline from `order_status_history`; status colors: PENDING=gray, CONFIRMED=blue, SHIPPED=orange, DELIVERED=green, CANCELLED/PAYMENT_FAILED=red
- `OrderDetailPage.tsx`: order items table + `OrderTimeline` + cancel button (only if PENDING or CONFIRMED)
- Cancel mutation: `PUT /orders/:id/cancel`; on success, invalidate `['orders']` + `['orders', id]`

*Profile + Address Management (Days 3–4):*
- `profileApi.ts`: GET/PUT profile, POST/PUT/DELETE/default address
- `ProfileForm.tsx`: update firstName/lastName; Toast on success
- `AddressManager.tsx`: list addresses with Edit/Delete/Set Default; "Add New" opens a Dialog; default address highlighted with Badge; default pre-selected on checkout

*Polish (Days 5–6):*
- Audit every `useQuery`: every failure state renders `<ErrorMessage>` with envelope's `error.message`
- Audit every page: skeleton on first load, never blank page
- Audit for empty states: empty cart, empty order history, no search results
- Axios catch-all: network errors show a global Toast
- Dockerize: `frontend/Dockerfile` (multi-stage: `npm run build` → serve with Nginx); add to `docker-compose.yml`; update root Nginx to serve the Vite build at `/`
- `frontend/README.md`: dev mode, Docker build

**Review/Test:**
- Full demo: register → browse → add to cart → checkout → payment polls to CONFIRMED → order history shows new order → OrderTimeline shows full progression
- Cancel flow: place order → cancel → OrderTimeline shows CANCELLED, cancel button disappears
- Address: add two addresses → set second as default → pre-selected on checkout
- Error state: stop a backend service → product page shows `<ErrorMessage>`, app does not crash
- `npm run build` → zero TypeScript errors

**Milestone:** A fully functional e-commerce frontend connected to the real backend. Every feature visible in a browser with zero curl commands. The Kafka saga is transparent to the user.

---

### Key Technical Patterns — Reference

#### Queue-Based JWT Refresh

```
accessToken  → Zustand (memory only) — cleared on page refresh by design
refreshToken → localStorage — cleared on explicit logout
```

On 401:
1. First request sets `isRefreshing = true`, calls `POST /auth/refresh`
2. All subsequent 401s during the refresh push onto `failedQueue: { resolve, reject }[]`
3. On refresh success: store new token → replay all queued requests with new token
4. On refresh failure: reject all queued → clear auth state → redirect to `/login`

Without the queue, 5 simultaneous components on a stale page each trigger a refresh — 4 fail because the refresh token is already rotated after the first use.

#### Payment Polling Pattern

`refetchInterval` as a function: the query stops polling itself when it sees terminal data — no `clearInterval`, no `useEffect` cleanup, no memory leaks.

#### Optimistic Cart Updates

`onMutate` snapshots the cache → applies the change locally → `onError` restores the snapshot → `onSettled` always re-syncs with server. This gives instant UI response while remaining correct under failure.

---

### What You MUST Understand Deeply (Frontend Addition)

| Understand Deeply (Do Manually) | OK to Use AI Assistance |
|---|---|
| Why accessToken in memory and not localStorage (XSS threat model) | Tailwind class names and layout utilities |
| How the queue-based 401 interceptor prevents duplicate refresh requests | shadcn/ui component setup and variants |
| Why `refetchInterval` returning `false` stops polling (query lifecycle) | TypeScript generic type definitions |
| How optimistic updates roll back on failure (snapshot/context pattern) | React Router v6 nested route syntax |

---

### Interview Questions This Frontend Adds

**"Why do you store the access token in memory instead of localStorage?"**
Tokens in localStorage are readable by any JavaScript on the page — including injected scripts from XSS. In-memory Zustand storage means a compromised script can't exfiltrate the token. The trade-off: the token disappears on page refresh, requiring a silent refresh on every page load. In production you'd use HttpOnly cookies for the refresh token so the browser manages it entirely and JavaScript can't touch it.

**"How does your frontend handle concurrent API calls when the access token expires?"**
Without a queue, five components mounting simultaneously on a stale page each get a 401 and each try to refresh — four fail because the refresh token is already rotated after the first use. My interceptor sets `isRefreshing = true` on the first 401, queues all subsequent requests as `{ resolve, reject }` promises, and replays them all once the single refresh succeeds. This is the same concurrency problem solved in the backend with idempotency keys — expressed in JavaScript promise queues.

**"Walk me through what happens from 'Place Order' to 'Order Confirmed.'"**
POST /orders validates cart, reserves stock, creates PENDING order, publishes `orders.created` to Kafka, returns 201 with orderId. Frontend redirects to confirmation page where `usePaymentStatus` polls `GET /payments/order/:id` every 2 seconds. Payment-service consumer processes the charge asynchronously and publishes `payments.completed` or `payments.failed`. Order-service consumer transitions the order. Next poll returns the terminal status, `refetchInterval` returns `false`, polling stops, UI shows success or failure. The Kafka saga is transparent to the user.

**"Why TanStack Query over useEffect + useState?"**
Manual `useEffect` for data fetching requires you to handle: loading state, error state, deduplication, cache invalidation after mutations, background refetch, and retry. TanStack Query handles all of that. For this project: `refetchInterval` with a conditional stop function is 3 lines; building it correctly with `useEffect` requires a cleanup function, a ref for the interval, and careful unmount handling — most implementations get the cleanup wrong and leak.

---

## Phase 6: AI Product Search with RAG (Month 6 — Weeks 21–24)

### Month 6 Goals
- Understand embeddings and vector similarity search
- Build a realistic RAG-powered product search
- Integrate with real product data from your database
- Keep it simple but genuinely functional

### Week 21 — Embeddings + Vector DB Fundamentals

**Learning Topics:**
- What are embeddings? How text → vector works
- Cosine similarity vs Euclidean distance
- Vector databases: FAISS (local, simple) vs pgvector (PostgreSQL extension)
- RAG architecture: query → embed → vector search → retrieve → (optional) LLM augment
- **Decision: Use pgvector** — it's a PostgreSQL extension, so no new infrastructure. Your product-service already uses PostgreSQL.

**Study exercises (no code yet):**
- Read the pgvector README and understand `vector(384)` column type
- Understand how `<=>` (cosine distance) operator works in SQL
- Try the OpenAI embeddings API or a free alternative (sentence-transformers) in a scratch script
- Draw the RAG flow for product search on paper:
  ```
  User query "red running shoes"
    → embed query → vector(384)
    → SELECT * FROM products ORDER BY embedding <=> query_vec LIMIT 10
    → return results
  ```

**Deliverable:** Hand-drawn RAG architecture. Understanding of embeddings proven by explaining it in your own words.

### Week 22 — pgvector Setup + Product Embedding Pipeline

**Learning Topics:**
- pgvector installation (Docker: `ankane/pgvector` image or enable extension in Postgres)
- Embedding model options:
  - **Free/local:** `all-MiniLM-L6-v2` via sentence-transformers (384 dimensions) — Python script
  - **API-based:** OpenAI `text-embedding-3-small` (1536 dimensions) — costs money but easy
- Batch embedding: how to embed all existing products
- **Recommendation: Use a small Python script** for the embedding pipeline. The search itself stays in product-service (Java/SQL). This is realistic — ML pipelines are often separate from serving.

**Implementation:**
- Add pgvector extension to PostgreSQL (`CREATE EXTENSION vector`)
- Add `embedding vector(384)` column to products table (Flyway migration)
- Create `scripts/embed_products.py`:
  - Connect to PostgreSQL
  - For each product: concatenate `name + description + category` → embed → store vector
  - Use `sentence-transformers` library (free, runs locally)
- Run the script against seed data
- Verify: `SELECT name, embedding <=> '[query_vector]' AS distance FROM products ORDER BY distance LIMIT 5`

**Deliverable:** All products have embeddings. Raw SQL similarity search works.

### Week 23 — Search API Endpoint in Product Service

**Implementation:**
- Add new endpoint to product-service:
  - `GET /api/v1/products/ai-search?q=comfortable+lightweight+shoes`
  - Flow: receive query → call embedding service → pgvector similarity search → return products
- Embedding service options (pick one):
  - **Option A:** Small Python Flask/FastAPI sidecar that exposes `POST /embed` → returns vector. Product-service calls it.
  - **Option B:** Call an external embedding API directly from Java (OpenAI or similar).
  - **Recommended: Option A** — it's more realistic (ML model served separately) and free.
- Create `ai-service/` — tiny Python service:
  - `POST /embed` — accepts `{"text": "..."}`, returns `{"embedding": [0.1, 0.2, ...]}`
  - Uses `sentence-transformers` with `all-MiniLM-L6-v2`
  - Dockerized, added to docker-compose
- Wire product-service → ai-service for query embedding
- Combine AI search with traditional filters (price range, category) as a fallback

**Review/Test:**
- Test: search for "comfortable shoes for running" returns running shoes (not just keyword match)
- Test: search for "affordable laptop" returns budget laptops
- Compare: AI search vs `LIKE '%running%'` — show cases where AI search is better
- Test: empty embedding service (circuit breaker: fall back to keyword search)

**Deliverable:** AI-powered product search working. Demonstrably better than keyword search for natural language queries.

### Week 24 — RAG Polish + Embedding Refresh Pipeline

**Implementation:**
- Add embedding refresh: when a product is created/updated, re-embed it
  - Option: product-service publishes event, Python script listens (or simple cron job)
  - Simple approach: re-run `embed_products.py` nightly (good enough for demo)
- Add search result ranking: combine vector similarity score with product rating/popularity
- Add search analytics: log queries and which results were returned (for demo purposes)
- Add the AI search to the OpenAPI spec
- Write tests for the search endpoint
- Create a demo script that shows 5 natural language queries and their results

**Review/Test:**
- Test the full flow: add new product → re-embed → search finds it
- Benchmark: how long does a similarity search take with 1000 products? 10000?
- **Reflection exercise:** "What would I need to change to handle 1 million products?" (answer: approximate nearest neighbors, HNSW index in pgvector)

**Milestone:** AI product search working end-to-end with real data. You can demo natural language search in interviews.

---

## Phase 7: AWS Deployment + CI/CD + Interview Prep (Month 7 — Weeks 25–30)

### Month 7 Goals
- Deploy the system to AWS
- CI/CD pipeline with GitHub Actions
- Documentation and demo preparation
- Interview-ready project presentation

### Week 25 — AWS Fundamentals

**Learning Topics:**
- AWS core services for your stack:
  - **EC2** — run your Docker Compose (simplest deployment)
  - **RDS** — managed PostgreSQL (with pgvector support)
  - **ElastiCache** — managed Redis
  - **S3** — product images (if you want)
  - **ECR** — Docker image registry
- VPC basics: public subnet, security groups, why not expose DB to internet
- AWS free tier: what you can run without paying

**Implementation:**
- Set up AWS account (if not already done)
- Create a VPC with public and private subnets
- Launch an EC2 instance (t3.medium or t3.small)
- Install Docker + Docker Compose on EC2
- Push Docker images to ECR (or build on EC2 directly)
- Run `docker compose up` on EC2 — verify it works

**Deliverable:** System running on AWS EC2.

### Week 26 — Managed Services: RDS + ElastiCache

**Learning Topics:**
- Why managed services: backups, patching, scaling, monitoring — handled by AWS
- RDS PostgreSQL: multi-AZ, automated backups, parameter groups
- ElastiCache Redis: cluster mode, persistence configuration
- Connection strings: how to point your services at RDS/ElastiCache instead of local containers

**Implementation:**
- Create RDS PostgreSQL instance (db.t3.micro, free tier eligible)
  - Enable pgvector extension
  - Run `init-databases.sql` to create schemas
- Create ElastiCache Redis instance (cache.t3.micro)
- Update `.env` / docker-compose to point at managed services
- Remove Postgres and Redis containers from production compose
- Test: verify all services connect to RDS + ElastiCache
- Set up security groups: only EC2 can reach RDS/ElastiCache

**Deliverable:** System running on EC2 with managed RDS and ElastiCache.

### Week 27 — CI/CD with GitHub Actions

**Learning Topics:**
- GitHub Actions: workflows, triggers, jobs, steps
- CI pipeline: lint → test → build → push image
- CD pipeline: deploy to EC2 (SSH or AWS CodeDeploy)
- Environment secrets: how to store DB passwords in GitHub

**Implementation:**
- Create `.github/workflows/ci.yml`:
  - Trigger: on push to main, on PR
  - Jobs:
    - `test-go`: run Go tests for user/cart/payment services
    - `test-java`: run Maven tests for product/order services
    - `build`: build Docker images
    - `push`: push images to ECR (only on main branch)
- Create `.github/workflows/deploy.yml`:
  - Trigger: on push to main (after CI passes)
  - SSH into EC2, pull latest images, `docker compose up -d`
- Add branch protection: require CI to pass before merging to main

**Deliverable:** Push to main → tests run → images built → deployed to AWS.

### Week 28 — Monitoring + Final Hardening

**Implementation:**
- Add CloudWatch basics: EC2 CPU/memory monitoring, RDS metrics
- Add application health monitoring: script that checks all `/health/ready` endpoints every minute
- Set up CloudWatch alarm: alert if any service health check fails
- Final security review:
  - [ ] No hardcoded secrets (all from env vars)
  - [ ] Security groups are tight (no 0.0.0.0/0 on DB ports)
  - [ ] HTTPS via Nginx + Let's Encrypt (or AWS ALB)
  - [ ] API rate limiting active
- Load test on AWS: run k6 against the deployed system, compare with local results

**Deliverable:** Monitored, secured AWS deployment.

### Week 29 — Documentation + README

**Implementation:**
- Update root `README.md`:
  - Project overview (what, why, how)
  - Architecture diagram
  - Tech stack table
  - How to run locally (Docker Compose)
  - How to deploy (AWS)
  - API overview with links to OpenAPI spec
  - Key technical decisions (link to ADRs)
- Write per-service README (following user-service pattern)
- Create a demo video script: 3-minute walkthrough of the system
- Update `docs/adrs/proposal.md` with lessons learned

**Deliverable:** Professional documentation. Someone new can clone the repo and understand the project in 10 minutes.

### Week 30 — Interview Preparation

**No new code.** This week is about preparing to talk about what you built.

**Prepare answers for these questions:**

1. **"Walk me through your architecture."**
   - Start with the problem (e-commerce at scale), explain bounded contexts, why Go vs Java, how services communicate (sync REST + async Kafka saga).

2. **"How do you handle race conditions?"**
   - Per-service answer: optimistic locking for products (high read), pessimistic for orders (correctness critical), Redis WATCH for cart (per-user), idempotency for payments (at-least-once Kafka).
   - Have the concurrent test results ready as proof.

3. **"Explain your Kafka saga."**
   - Draw the sequence diagram. Explain what happens when payment fails. Explain DLQ. Explain idempotency.

4. **"How does your AI search work?"**
   - Explain embeddings in plain language. Show the pgvector query. Explain why vector search is better than LIKE for natural language.

5. **"What would you change for production scale?"**
   - Kubernetes for orchestration, service mesh for observability, CQRS for read/write separation, Elasticsearch for full-text search + vector search, horizontal scaling of stateless services.

6. **"What was the hardest bug you encountered?"**
   - Have 2-3 real stories ready from your development experience.

**Practice:**
- Do a mock interview with a friend or record yourself explaining the system
- Time yourself: system walkthrough should be under 5 minutes
- Prepare a 1-slide system diagram you can draw on a whiteboard in 2 minutes

**Milestone:** You can confidently explain every architectural decision, every concurrency pattern, and the AI feature.

---

## Key Milestones Summary

| When | Milestone | Why It Matters |
|------|-----------|----------------|
| End of Month 1 | ✅ Product Service complete | Second service done, you know both Go and Spring Boot |
| End of Month 2 | ✅ End-to-end order flow | Core business logic works, services talk to each other |
| End of Month 3 | Kafka saga working | Async distributed transaction — the hardest pattern |
| End of Month 4 | Tested + hardened | Production-quality code, not just "it works on my machine" |
| **End of Week 20** | **React frontend live** | **System is demoable in a browser — no curl required** |
| End of Month 6 | AI search working | Differentiating feature that shows breadth |
| End of Month 7 | Deployed + interview-ready | Live system you can demo, stories you can tell |

---

## 3 Features That Make This Project Stand Out for Internships

### 1. Concurrency Control Showcase
Most student projects have zero concurrency handling. Yours has **four different strategies** (optimistic, pessimistic, Redis atomic, idempotency key), each chosen for a specific reason. This shows you understand the trade-offs, not just the syntax.

**Interview line:** "I implemented four different concurrency strategies across five services, each chosen based on the specific contention pattern — high-read catalog uses optimistic locking, critical order state transitions use pessimistic locks."

### 2. RAG-Powered Product Search with pgvector
AI integration that isn't just "I called the ChatGPT API." You built an embedding pipeline, stored vectors in PostgreSQL, and implemented similarity search. This shows you can integrate ML into backend systems practically.

**Interview line:** "I added semantic product search using RAG — products are embedded with sentence-transformers, stored in pgvector, and searched by cosine similarity. It handles natural language queries like 'comfortable shoes for long walks' that keyword search can't."

### 3. Kafka Choreography Saga with Idempotency
Event-driven architecture with proper failure handling. Most students use synchronous HTTP for everything. You have a genuine async saga with DLQ, retry, and idempotency.

**Interview line:** "Order-to-payment uses a Kafka choreography saga — the payment service is idempotent by design, so Kafka's at-least-once delivery is handled gracefully. Failed payments go to a dead letter queue for manual inspection."

---

## How to Explain This Project in Interviews

### The 30-Second Pitch
"I built a distributed e-commerce platform with five microservices — three in Go for high-concurrency I/O and two in Java/Spring Boot for complex business transactions. The services communicate through REST and Kafka, with a choreography saga for the order-payment flow. Each service uses a different concurrency control strategy matched to its contention pattern. I also added AI-powered product search using RAG with pgvector. The whole system is deployed on AWS with CI/CD."

### The Follow-Up Framework
When they ask deeper questions, follow this structure:
1. **What** — the specific technical decision
2. **Why** — the trade-off you evaluated
3. **Proof** — the test or metric that validates it

Example: "I used pessimistic locking for order state transitions *because* if two transitions succeed simultaneously — say, CONFIRMED→SHIPPED and CONFIRMED→CANCELLED — we'd have an inconsistent order. I wrote a concurrent integration test that proves exactly one transition succeeds."

### Red Flags to Avoid
- Don't say "I used AI to write it." Say "I used AI as a coding assistant for boilerplate, but I designed the architecture and wrote the concurrency logic myself."
- Don't claim expertise you don't have. It's better to say "I chose pgvector because it was simpler than standing up a separate vector DB — for production scale, I'd evaluate Pinecone or Weaviate."
- Don't undersell it. This is a legitimate distributed system, not a toy project. Own it.

---

## Final Note

You've already done the hardest thing: you started, and you finished one service properly. The user-service proves you can write production-grade code. Now it's about doing that four more times, adding the infrastructure glue, and learning to talk about it.

The plan is ambitious but achievable at 18 hours/week. If you fall behind, **cut scope, not quality.** It's better to have 4 polished services than 5 half-done ones.

Good luck, Hung. You're building something real.
