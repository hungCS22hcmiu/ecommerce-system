# CLAUDE.md

## Service Map

| Service | Language | Port | Status | Key Pattern |
|---|---|---|---|---|
| user-service | Go (Gin + GORM) | 8001 | **Implemented** | Pessimistic lock on login |
| product-service | Java/Spring Boot | 8081 | **Implemented** | Optimistic lock + Redis cache-aside + pgvector AI search |
| cart-service | Go (Gin + GORM) | 8002 | **Implemented** | Redis-first, WATCH/MULTI/EXEC |
| order-service | Java/Spring Boot | 8082 | **Implemented** | Pessimistic lock on state transitions |
| payment-service | Go (Gin) | 8003 | **Implemented** | Idempotency key + DB UNIQUE + Kafka saga |
| ai-service | Python (FastAPI) | 8000 | **Implemented** | sentence-transformers sidecar, `POST /embed` |
| frontend | React 19 + Vite → Nginx | 3001 | **Implemented** | TanStack Query, Zustand, JWT interceptor |
| nginx | nginx:alpine | 80 | **Active** | Reverse proxy, rate limiting, CORS |

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

**Sync REST:** Cart → Product (`PRODUCT_SERVICE_URL`) · Product → ai-service (`AI_SERVICE_URL`) for embeddings  
**Async Kafka saga:** `orders.created` → payment-service → `payments.completed/failed` → order-service. Broker: `kafka:29092`.  
**Databases:** Single Postgres, 5 logical DBs (`ecommerce_users/products/carts/orders/payments`). Cross-DB refs at app level only.  
**Redis:** user-service (sessions, JWT blacklist, login attempts) · cart-service (primary store) · product-service (cache-aside)  
**JWT:** RS256, 15 min access TTL. Keys: `./keys/private.pem` / `./keys/public.pem`.  
**API envelope:** `{ success, data, meta? }` / `{ success: false, error }` — see `api/openapi.yaml`.

**Concurrency per service:**

| Service | Strategy |
|---|---|
| User | `SELECT ... FOR UPDATE` |
| Product | `@Version` optimistic + `@Retryable` |
| Cart | Redis `WATCH/MULTI/EXEC` |
| Order | `SELECT ... FOR UPDATE` |
| Payment | Idempotency key + `UNIQUE` constraint |

## Coding Standards

**Go:** Services depend on repo interfaces. Context propagation: handler → service → repo → `db.WithContext(ctx)`. Validation via `go-playground/validator/v10`. Testing: `testify`, always `-race`. Login TX always commits for auth errors — only real DB errors rollback.

**Java:** Spring Boot 3.5, Java 21, Lombok. Flyway only (`ddl-auto: none`). Migrations in `classpath:db/migration`.

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
Blocked externally (403): `PUT /orders/:id/ship|deliver` · `POST /inventory/:id/reserve|release`  
CORS: `http://localhost:3001`

Scripts (default port 80):
- `bash script/e2e-test.sh` — 14 assertions (browse → cart → order)
- `bash script/e2e-payment.sh` — 12 assertions (Kafka saga)
- `bash script/loadtest-orders.sh` — 100 orders at 10 concurrent

---

## product-service

**Endpoints:** CRUD + `/search?q=` (keyword, cached 3min) + `/ai-search?q=&limit=` (pgvector, cached 1min, fallback on `AIServiceException`) + inventory reserve/release + categories CRUD.  
**Auth:** `X-Seller-Id` header forwarded by Nginx; missing → 400.  
**AI search flow:** query → `EmbeddingClient.embed()` → `SET LOCAL ivfflat.probes=10` → `findIdsBySemanticSimilarity` → `AISearchResponse{query, results, scores, mode}`. Cache write skipped on `AIServiceException`.  
**Write-through embedding:** `ProductEmbeddingService.scheduleEmbedding()` fires `@Async` on every create/update; failures logged WARN, never surface to caller. Toggle: `ai-service.write-through-enabled`.  
**pgvector:** `embedding vector(384)` + IVFFLAT index (lists=100, cosine ops). Migration: `V3__add_product_embeddings.sql`.  
**Tests:** 76 total — unit (Mockito) + integration (Testcontainers: Postgres + Redis + pgvector) + AI fallback.

## user-service

**Endpoints:** register · login · refresh · verify-email · resend-verification · logout · profile · addresses.  
**SMTP:** requires `SMTP_HOST/PORT/USERNAME/PASSWORD/FROM` in `.env`.

## payment-service

**Endpoints:** `POST /api/v1/payments` · `GET /api/v1/payments` · `GET /api/v1/payments/:id` · `GET /api/v1/payments/order/:orderId` · health live/ready (checks postgres + kafka).  
**Kafka:** consumes `orders.created` (group `payment-service`) → publishes `payments.completed|failed`. Worker pool: 5 goroutines. `__TypeId__` header set for Spring Kafka deserialization.  
**Resilience:** 3-tier — poison (DLQ immediately) · transient (3× retry 100/200/400ms → DLQ) · permanent decline (no DLQ). Lag logger every 30s (WARN if > 10k). 30s graceful shutdown.

## ai-service

**Endpoints:** `POST /embed` · `POST /embed/batch` (max 64) · `GET /health/live|ready`.  
**Model:** `all-MiniLM-L6-v2`, 384 dims, CPU-only torch (~1.5 GB image). Loaded async via FastAPI `lifespan`.  
**Backfill:** `scripts/embed_products.py` — batch re-embeds all products via `/embed/batch`.

## frontend

**Stack:** React 19 · TypeScript · Vite 8 · TanStack Query 5 · Zustand 5 · Axios · React Router 6 · Tailwind CSS 4.  
`lib/axios.ts` — queue-based 401 interceptor (`isRefreshing` + `failedQueue[]`; replays all on refresh success).  
`store/authStore.ts` — `accessToken` in memory (XSS-safe), `refreshToken` in localStorage.  
`features/payment/usePaymentStatus.ts` — `refetchInterval` returns `false` on terminal status (self-stopping poll).  
`features/products/useProductAISearch.ts` — TanStack Query, `enabled: q.length >= 2`, `staleTime: 60s`.  
`components/shared/AISearchBadge.tsx` — badge shown on `ProductListPage` when AI search mode is active.
