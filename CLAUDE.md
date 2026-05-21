# CLAUDE.md

## Service Map

| Service | Language | Port | Status | Key Pattern |
|---|---|---|---|---|
| user-service | Go (Gin + GORM) | 8001 | **Implemented** | Bcrypt pool + Redis-only lockout · public seller profile |
| product-service | Java/Spring Boot | 8081 | **Implemented** | Conditional UPDATE (atomic stock) + Redis cache-aside + pgvector AI search + reviews/ratings |
| cart-service | Go (Gin + GORM) | 8002 | **Implemented** | Redis-first, WATCH/MULTI/EXEC · Redis product-validation cache (5s TTL) |
| order-service | Java/Spring Boot | 8082 | **Implemented** | Pessimistic lock · Transactional outbox → Kafka · async stock release · notifications · seller order view |
| payment-service | Go (Gin) | 8003 | **Implemented** | Idempotency key + DB UNIQUE + Kafka saga · PENDING-resume on restart |
| ai-service | Python (FastAPI) | 9000 | **Implemented** | sentence-transformers sidecar, `POST /embed` |
| frontend | React 19 + Vite → Nginx | 3001 | **Implemented** | TanStack Query, Zustand, JWT interceptor |
| nginx | nginx:alpine | 80 | **Active** | Reverse proxy, rate limiting, CORS · resolver-based dynamic upstream DNS |

## Commands

```bash
cp .env.example .env
docker compose up --build -d                                          # full stack on port 80
docker compose build <service> && docker compose up -d <service>      # rebuild one service

# Go (user/cart/payment)
go test -race ./...
go test -tags=integration -v -race ./internal/integration/

# Java (product/order)
./mvnw spring-boot:run
./mvnw test

# ai-service
uvicorn main:app --reload     # dev on :8000
pytest tests/
```

Databases auto-initialized via `script/init-databases.sql`.

## Architecture

**Sync REST:** Cart → Product (`PRODUCT_SERVICE_URL`) · Product → ai-service (`AI_SERVICE_URL`, internal `http://ai-service:9000`) for embeddings · Product → Order (`ORDER_SERVICE_URL`) for review notifications (fire-and-forget)
**Async Kafka saga:** `orders.created` → payment-service → `payments.completed/failed` → order-service. Broker: `kafka:29092`.
**Databases:** Single Postgres, 5 logical DBs (`ecommerce_users/products/carts/orders/payments`). Cross-DB refs at app level only.
**Redis:** user-service (sessions, JWT blacklist, login attempts) · cart-service (primary store + product-validation cache) · product-service (cache-aside)
**JWT:** RS256, 15 min access TTL. Keys: `./keys/private.pem` / `./keys/public.pem`.
**API envelope:** `{ success, data, meta? }` / `{ success: false, error }` — see `api/openapi.yaml`.

**Concurrency per service:**

| Service | Strategy |
|---|---|
| User | Bcrypt worker pool (`runtime.NumCPU()` workers); Redis-only login-attempt counter (no row lock on login) |
| Product | Conditional native UPDATE (`reserveStockConditional` / `releaseStockConditional`) — atomically decrements/restores stock without `@Version` retries |
| Cart | Redis `WATCH/MULTI/EXEC` |
| Order | `SELECT ... FOR UPDATE` on order row |
| Payment | Idempotency key + `UNIQUE` constraint; PENDING-resume path retries gateway on re-delivery |

## Coding Standards

**Go:** Services depend on repo interfaces. Context propagation: handler → service → repo → `db.WithContext(ctx)`. Validation via `go-playground/validator/v10`. Testing: `testify`, always `-race`. Login TX always commits for auth errors — only real DB errors rollback.

**Java:** Spring Boot 3.5, Java 21, Lombok. Flyway only (`ddl-auto: none`). Migrations in `classpath:db/migration`.

**Order-service status columns:** `orders.status`, `order_status_history.old_status/new_status` are `VARCHAR(50)` (converted from PostgreSQL enum via V6 migration). Do NOT use `@ColumnTransformer(write = "?::order_status")` — the `shippingAddress` field still uses `@ColumnTransformer(write = "?::jsonb")` which is correct.

## Key Files

- `docker-compose.yml` — full stack with health checks
- `nginx/nginx.conf` — routes, rate limiting, CORS, security headers, blocked internal routes
- `script/init-databases.sql` — all 5 DB schemas
- `script/sample_users.sql` — 1 admin / 1 customer / 1 seller (pre-verified)
- `api/openapi.yaml` — full REST API contract
- `.env.example` — all env vars
- `docs/adrs/locking-strategy.md` — concurrency rationale per service
- `docs/adrs/saga-resilience.md` — Kafka saga and DLQ design decisions
- `docs/technical/service_integration.md` — inter-service communication with Mermaid diagrams

## Nginx

Routes: `/api/v1/auth|users/*` → user-service:8001 · `/api/v1/products*|inventory*` → product-service:8081 · `/api/v1/cart*` → cart-service:8002 · `/api/v1/orders*` → order-service:8082 · `/api/v1/payments*` → payment-service:8003 · `/health/*` → payment-service:8003

Rate limiting: 10r/s general (`api_limit`) · 5r/min auth (`auth_limit`).
Blocked externally (403): `PUT /orders/:id/ship|deliver` · `POST /inventory/:id/reserve|release` · `POST /orders/notifications/internal/review`
CORS: `http://localhost:3001`
DNS: `resolver 127.0.0.11 valid=30s` + variable-based `proxy_pass` so upstream IPs re-resolve after container restarts (prevents stale-IP 502s).

Scripts (default port 80):
- `bash script/e2e-test.sh` — 14 assertions (browse → cart → order)
- `bash script/e2e-payment.sh` — 12 assertions (Kafka saga)
- `bash script/loadtest-orders.sh` — 100 orders at 10 concurrent

---

## product-service

**Endpoints:** CRUD + `/search?q=` (keyword, cached 3min) + `/ai-search?q=&limit=` (pgvector, cached 1min, fallback on `AIServiceException`) + inventory reserve/release + categories CRUD + reviews CRUD + seller shop public profile.
**List params:** `categoryId`, `status`, `ratedOnly=true` (filters `rating_count > 0`, used for "Highest Rated" seller view), standard `page`/`size`/`sort`.
**Auth:** `X-Seller-Id` header forwarded by Nginx; missing → 400.
**AI search flow:** query → `EmbeddingClient.embed()` → `SET LOCAL ivfflat.probes=10` → `findIdsBySemanticSimilarity` → `AISearchResponse{query, results, scores, mode}`. Cache write skipped on `AIServiceException`. `@Cacheable(unless = "#result.results().isEmpty()")` prevents caching empty results before async embedding completes.
**Write-through embedding:** `ProductEmbeddingService.scheduleEmbedding()` fires `@Async` on every create/update; failures logged WARN, never surface to caller.
**Cache warmup:** `CacheWarmupService` fires 3 representative AI queries on `ApplicationReadyEvent` (`@Async`) to warm pgvector planner cache and seed Redis.
**Stock mutations:** `reserveStock` / `releaseStock` use native conditional UPDATE (`WHERE stock_available >= qty`) — atomically decrements/restores without optimistic-lock retries. Returns 0 rows → diagnoses out-of-stock vs. not-found via `findById`. `StockProjection` interface used for read-only stock checks to avoid Hibernate dirty-check version bumps.
**Reviews:** `POST /products/{id}/reviews` · `GET /products/{id}/reviews` · `PUT /reviews/{reviewId}` · `DELETE /reviews/{reviewId}` · `GET /products/{id}/my-review?orderItemId=`. On create, recalculates `avg_rating` and `rating_count` on `products`, then fires `orderServiceClient.notifySellerReview()` (fire-and-forget).
**pgvector:** `embedding vector(384)` + IVFFLAT index (lists=100, cosine ops). Migration: `V3__add_product_embeddings.sql`.
**Migrations:** V1 baseline · V2 seed (200 products, 19 categories) · V3 embeddings · V4 reviews/ratings.
**Tests:** unit (Mockito) + integration (Testcontainers: Postgres + Redis + pgvector) + AI fallback.

## order-service

**Endpoints:** create order · list user orders · get order detail · cancel order · ship/deliver (internal) · order history · seller order list · get/mark-read notifications.
**Seller context:** `GET /orders?sellerId=` returns orders scoped to the seller; `PUT /orders/:id/ship` and `PUT /orders/:id/deliver` are blocked externally (nginx 403), internal only.
**Transactional outbox:** `createOrder()` writes an `OutboxEvent` row atomically with the order. `OutboxPublisher` polls every 100ms with `FOR UPDATE SKIP LOCKED`, publishes to `orders.created`, marks `published_at`. Correlation-ID stored in outbox `headers` JSON and re-applied on the Kafka `ProducerRecord` header. Reaper (`@Scheduled(fixedDelay=60_000)`) re-queues PENDING orders > 2min old with no unpublished outbox row.
**Stock release:** `releaseStockForOrder()` runs `@Async` on `order-async-*` pool with `@Retryable(3×, 100/200/400ms)` so the Kafka consumer thread returns immediately on cancellation/saga-fail.
**Notifications:** `Notification` entity stores `userId`, `orderId`, `productId` (nullable), `title`, `body`, `isRead`. Sellers are notified on order creation; buyers on payment events. Review notifications set `productId` (not `orderId`) so the frontend navigates to `/products/:id` on click.
**Internal endpoint:** `POST /orders/notifications/internal/review` — accepts `{sellerId, productId, title, body}` from product-service after a review is created. Blocked at nginx externally.
**Migrations:** V1 baseline · V2 seller context · V3 notifications · V4 product_id on notifications · V5 implicit cast attempt · V6 converts `status` columns from PostgreSQL enum to `VARCHAR(50)` · V7 `orders_outbox` table + partial index on unpublished rows.
**Status columns:** `orders.status` and `order_status_history.old_status/new_status` are VARCHAR(50). Do not add `@ColumnTransformer` for enum casting.

## user-service

**Endpoints:** register · login · refresh · verify-email · resend-verification · logout · profile · addresses · `GET /users/:id/seller-profile` (public, no auth — used by seller shop page).
**Login path:** bcrypt runs in a bounded worker pool (`pkg/password/pool.go`, `runtime.NumCPU()` workers). Pool full → HTTP 503 + `Retry-After: 1`. Login-attempt counter is Redis-only (no `SELECT ... FOR UPDATE`). `BCRYPT_COST` is env-driven (default 12; docker-compose sets 10).
**SMTP:** requires `SMTP_HOST/PORT/USERNAME/PASSWORD/FROM` in `.env`.

## payment-service

**Endpoints:** `POST /api/v1/payments` · `GET /api/v1/payments` · `GET /api/v1/payments/:id` · `GET /api/v1/payments/order/:orderId` · health live/ready (checks postgres + kafka).
**Kafka:** consumes `orders.created` (group `payment-service`) → publishes `payments.completed|failed`. Worker pool: 5 goroutines. Manual commit (`CommitInterval: 0`). `__TypeId__` header set for Spring Kafka deserialization.
**Resilience:** 3-tier — poison (DLQ immediately) · transient (3× retry 100/200/400ms → DLQ) · permanent decline (no DLQ). Lag logger every 30s (WARN if > 10k). 30s graceful shutdown.
**PENDING-resume:** `ProcessPayment` detects `ErrDuplicateIdempotencyKey` with existing status=PENDING (service was killed mid-gateway call) and retries the gateway using the existing payment ID. If `publishOutcome` still sees PENDING, the Kafka offset is NOT committed, forcing re-delivery.

## ai-service

**Endpoints:** `POST /embed` · `POST /embed/batch` (max 64) · `GET /health/live|ready`.
**Model:** `all-MiniLM-L6-v2`, 384 dims, CPU-only torch (~1.5 GB image). Loaded async via FastAPI `lifespan`; 1 warmup inference before `yield`.
**Backfill:** `scripts/embed_products.py` — batch re-embeds all products via `/embed/batch`.

## frontend

**Stack:** React 19 · TypeScript · Vite 8 · TanStack Query 5 · Zustand 5 · Axios · React Router 6 · Tailwind CSS 4.
`lib/axios.ts` — queue-based 401 interceptor (`isRefreshing` + `failedQueue[]`; replays all on refresh success). `/auth/refresh` 401 bypasses the refresh block entirely → `clearAuth()` + redirect (no deadlock).
`store/authStore.ts` — `accessToken` in memory (XSS-safe), `refreshToken` in localStorage.
`features/payment/usePaymentStatus.ts` — `refetchInterval` returns `false` on terminal status (self-stopping poll).
`features/products/useProductAISearch.ts` — TanStack Query, `enabled: q.length >= 2`, `staleTime: 60s`.
`features/products/useProductListInfinite` — `useInfiniteQuery` hook for scroll-based pagination; used on ProductDetailPage for "More from [category]" section.
`components/shared/NotificationBell.tsx` — clicks navigate: `productId` set → `/products/:id`; `orderId` set → `/orders/:id`.
`components/shared/ReviewDialog.tsx` — multi-item panel: all order items shown simultaneously, per-item rating + comment, single Submit.
`Dockerfile` — uses `npx vite build` directly (not `npm run build`) to skip `tsc -b` strict check on test files.
**Cart:** `CartDrawer` has per-item checkboxes; "Proceed to Checkout" disabled until items selected; blocks cross-seller selection with inline error. `useCartMutations.addItem.onError` shows specific toasts per error code (SELLER_CANNOT_BUY_OWN_PRODUCT, INSUFFICIENT_STOCK, etc.).
**OrderConfirmationPage:** on payment COMPLETED calls `removeItem` for each ordered product ID only (not `clearCart`), preserving items from other sellers.
**ProductDetailPage:** back link resolves to product's own category (`← Back to [categoryName]`); "More from [category]" infinite-scroll section below reviews (IntersectionObserver sentinel, 200 px lookahead, 8 per page); `window.scrollTo(0,0)` on `id` change.
**Pages added beyond plan:** CategoryBrowsePage · CategoryProductsPage · SellerShopPage · SellerOrdersPage · SellerOrderDetailPage.
