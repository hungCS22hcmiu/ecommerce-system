# Performance & Quality Improvement Plan

> Maps the 18 entries from [`docs/testing/improvement performance plan.md`](../testing/improvement%20performance%20plan.md) into an execution sequence of 9 development steps. Each step lists what to explore first, the key changes to make, and the verification command(s) that prove the fix landed.

## Reading order

1. The latest test results: [`docs/testing/test_result.md`](../testing/test_result.md).
2. The triaged findings: [`docs/testing/improvement performance plan.md`](../testing/improvement%20performance%20plan.md).
3. This document for the order of work.

## Why this order

The 18 IMP entries are not independent. Three clusters dominate the prioritization:

- **The login chokepoint (IMP-6)** is the single biggest gate on Phase 2 throughput. Fixing it lets every downstream load test produce realistic numbers.
- **The order/inventory retry-and-outbox cluster (IMP-2, IMP-8, IMP-11, IMP-13)** all stem from one root: `ProductRepository` optimistic-lock retries exhaust under sustained concurrency, and order-service has no transactional outbox. One coordinated fix dissolves four entries.
- **Frontend correctness (IMP-18, IMP-16, IMP-17)** is cheap, visible to users, and includes one **P1 silent-deadlock** bug that should not wait.

The remaining items — observability (IMP-4), latency tails (IMP-7, IMP-3, IMP-5, IMP-9, IMP-10), and infra hygiene (IMP-14, IMP-15) — clean up cleanly after the big rocks land. **IMP-12 is direction, not work** — already encoded in this sequencing.

---

## ~~Step 1 — Unblock the front door: `/auth/login` throughput  (P0, IMP-6)~~ ✅ DONE 2026-05-19

**Goal:** sustain 100 RPS on `POST /api/v1/auth/login` with P95 < 300ms, error rate < 1%.

### Changes landed
1. **`user-service/pkg/password/pool.go`** (new) — bounded bcrypt worker pool (`runtime.NumCPU()` workers, queue 256). `ErrBcryptOverload` sentinel → handler returns HTTP 503 + `Retry-After: 1`.
2. **`user-service/pkg/password/password.go`** — `cost` is now env-driven via `BCRYPT_COST` (default 12; dev stack defaults to 10 via docker-compose).
3. **`user-service/internal/service/auth_service.go`** — `Login()` rewritten: bcrypt moved outside any DB transaction; `SELECT ... FOR UPDATE` replaced by plain `FindByEmailWithProfile`; redundant `UpdateLoginAttempts` DB writes removed; lockout is Redis-counter-only.
4. **`user-service/internal/repository/user_repository.go`** — added `FindByEmailWithProfile` (plain SELECT + Profile preload, no row lock).
5. **`user-service/internal/handler/auth_handler.go`** — added `ErrBcryptOverload` → 503 case.
6. **`user-service/cmd/server/main.go`** — pool wired + stopped on shutdown; HTTP timeouts tuned (`ReadHeaderTimeout: 5s, ReadTimeout: 15s, WriteTimeout: 30s, IdleTimeout: 30s`); DB pool increased to MaxOpenConns=50/MaxIdleConns=10.
7. **`docker-compose.yml`** — `BCRYPT_COST: ${BCRYPT_COST:-10}` added to user-service environment.

### Verification (run to confirm)
- `cd user-service && go test -race -count=1 ./pkg/password/... ./internal/service/...` — **PASS** (verified 2026-05-19)
- `bash script/test/phase2_run.sh` — re-run §1-User: target `auth_login.json` P95 < 300ms, throughput ≥ 100/s, http_req_failed < 1%.
- Look for **503 with Retry-After** in `auth_login.log` under overload — confirms load shedding works.

---

## ~~Step 2 — Order/inventory retry strategy  (P1, IMP-2 + IMP-8 + IMP-11)~~ ✅ DONE 2026-05-19

**Goal:** `POST /api/v1/orders` P95 < 400ms at 50 RPS, error rate < 0.1%; 50 VU composite checkout error rate < 0.1%.

### Changes landed
1. **`product-service/src/main/java/com/ecommerce/product_service/repository/ProductRepository.java`** — added `reserveStockConditional` and `releaseStockConditional` native `@Modifying(clearAutomatically=true)` queries; replaced the old `@Version`-based SELECT + save pattern entirely.
2. **`product-service/src/main/java/com/ecommerce/product_service/service/serviceImpl/InventoryServiceImpl.java`** — rewrote `reserveStock` and `releaseStock`: removed `@Retryable`; conditional UPDATE returns 0 → `findById` diagnoses OOS vs. not-found; returns 1 → `findById` fetches post-UPDATE state, inserts StockMovement. `@CacheEvict(value="product")` retained.
3. **`product-service/src/main/java/com/ecommerce/product_service/exception/StockContentionException.java`** (new) — safety-belt exception → HTTP 503 `STOCK_CONTENTION`. Theoretically unreachable with atomic conditional UPDATE, but wired up defensively.
4. **`product-service/src/main/java/com/ecommerce/product_service/exception/GlobalExceptionHandler.java`** — added `StockContentionException` → 503 handler. `ObjectOptimisticLockingFailureException` handler retained (still used by product CRUD `save()`).
5. **`order-service/src/main/java/com/ecommerce/order_service/client/ProductServiceClient.java`** — added `HttpServerErrorException` catch in `reserveStock()` to handle the 503 defensive path cleanly.
6. **`product-service/src/test/java/com/ecommerce/product_service/integration/InventoryConcurrencyTest.java`** — updated for conditional UPDATE semantics: removed `ObjectOptimisticLockingFailureException` from catch clause; renamed test 2; updated Javadoc.
7. **`product-service/src/test/java/com/ecommerce/product_service/service/InventoryServiceImplTest.java`** — updated all mocks: `save()` replaced by `reserveStockConditional`/`releaseStockConditional` stubs returning 0/1; `findById` stubs return post-update product state.

### Verification (run to confirm)
- `cd product-service && ./mvnw test -Dtest=InventoryConcurrencyTest` — **PASS** (verified 2026-05-19, 78/78 unit tests pass).
- `bash script/test/phase2_run.sh` — re-run `order_create.json` + `checkout_50vu.json` rows: target P95 < 400ms, throughput ≥ 50/s, http_req_failed < 0.1%.
- Direct race test: `bash script/k6/race_inventory.js` — expect 1×201 + 9×409, no 503s.

---

## ~~Step 3 — Transactional outbox in order-service  (P1, IMP-13)~~ ✅ DONE 2026-05-20

**Goal:** zero PENDING orders left behind across payment-service restart, even at sustained load.

### Changes landed
1. **`order-service/src/main/resources/db/migration/V7__orders_outbox.sql`** (new) — `orders_outbox` table (id, order_id UUID, payload JSONB, headers JSONB, created_at, published_at) + partial index on unpublished rows. `@ColumnTransformer(write = "?::jsonb")` required on payload/headers to avoid Postgres type mismatch.
2. **`order-service/src/main/java/com/ecommerce/order_service/model/OutboxEvent.java`** (new) — JPA entity mapping `orders_outbox`; uses `@ColumnTransformer(write = "?::jsonb")` on both JSONB columns.
3. **`order-service/src/main/java/com/ecommerce/order_service/repository/OutboxEventRepository.java`** (new) — JPA repo with native `FOR UPDATE SKIP LOCKED` query for safe concurrent polling.
4. **`order-service/src/main/java/com/ecommerce/order_service/repository/OrderRepository.java`** — added `findStuckPendingOrderIds(@Param("threshold") OffsetDateTime threshold)` native query for reaper.
5. **`order-service/src/main/java/com/ecommerce/order_service/config/AsyncConfig.java`** — added `@EnableScheduling` (was `@EnableAsync` only).
6. **`order-service/src/main/java/com/ecommerce/order_service/service/impl/OrderServiceImpl.java`** — replaced `eventProducer.publishOrderCreated(event)` with `outboxEventRepository.save(OutboxEvent...)` inside the same `@Transactional` scope; injected `OutboxEventRepository` and `ObjectMapper`.
7. **`order-service/src/main/java/com/ecommerce/order_service/kafka/OutboxPublisher.java`** (new) — `@Scheduled(fixedDelay=100)` polls up to 100 unpublished rows with `FOR UPDATE SKIP LOCKED`, publishes via `kafkaTemplate.send().get(5s)`, marks `published_at`; `@Scheduled(fixedDelay=60_000)` reaper re-queues PENDING orders > 2 min old with no unpublished outbox row.
8. **`order-service/src/test/java/com/ecommerce/order_service/integration/OrderOutboxIT.java`** (new) — 2 IT tests (Testcontainers PG + EmbeddedKafka): atomic write assertion + publisher delivery.
9. **`order-service/src/test/java/com/ecommerce/order_service/service/OrderServiceImplTest.java`** — added `@Mock OutboxEventRepository` + `@Mock ObjectMapper`; updated assertions from `eventProducer.publishOrderCreated` to `outboxEventRepository.save`.

### Verification (run to confirm)
- `cd order-service && ./mvnw test -Dtest="OrderOutboxIT,OrderServiceImplTest,OrderConcurrencyTest"` — **PASS 31/31** (verified 2026-05-20)
- `bash script/test/chaos_saga_kill.sh` — must report **`pending_count=0`** across N runs.
- Sanity: under 100-order load, total Kafka messages on `orders.created` == total orders created.

---

## ~~Step 4 — Frontend axios `/auth/refresh` deadlock  (P1, IMP-18)~~ ✅ DONE 2026-05-20

**Goal:** an expired refresh token must cleanly redirect to `/login`, not silently hang.

### Changes landed
1. **`frontend/src/lib/axios.ts` line 26** — added `|| original.url?.includes('/auth/refresh')` to `isAuthEndpoint`. When `/auth/refresh` itself returns 401, the interceptor now bypasses the refresh block entirely (falls through to `Promise.reject`), so the outer `catch` runs `processQueue(err, null)` + `clearAuth()` + redirect. No more deadlock.
2. **`frontend/src/lib/__tests__/axios.test.ts` lines 143–156** — updated the "refresh failure" test adapter to return 401 (not 500) for all endpoints including `/auth/refresh`. Updated comment to document the fix. `cd frontend && npx vitest run` — **11/11 PASS** (verified 2026-05-20).

### Verification (run to confirm)
- `cd frontend && npx vitest run` — **PASS 11/11** (verified 2026-05-20)
- Manual smoke: revoke a refresh token in Redis (`DEL refresh_token:<userId>`), refresh the page, verify a single redirect to `/login` with no hanging spinners.

---

## ~~Step 5 — Correlation-ID propagation across the saga  (P1, IMP-4)~~ ✅ DONE 2026-05-20

**Goal:** an `X-Correlation-ID` injected at nginx appears in every Go and Java service log line, including payment-service for Kafka-consumed events.

### Changes landed
1. **Root cause resolved by Step 3 (outbox)**: `createOrder()` now explicitly captures `MDC.get("correlationId")` on the HTTP request thread and writes it to `outbox.headers` JSONB. `OutboxPublisher.publishPending()` reads the correlation ID from DB (not MDC) and attaches it to the Kafka `ProducerRecord` header before publishing. This is more robust than MDC-based propagation across thread boundaries.
2. **`order-service/src/main/java/com/ecommerce/order_service/kafka/OutboxPublisher.java`** — added `MDC.put("correlationId", correlationId)` per event in `publishPending()` (read from stored headers) with `MDC.clear()` in `finally`. Scheduler-thread log lines for outbox publish events now carry the correlation ID.
3. **`order-service/src/main/java/com/ecommerce/order_service/config/AsyncConfig.java`** — added `TaskDecorator` that snapshots `MDC.getCopyOfContextMap()` on the submitting thread and restores it on the async thread (with `MDC.clear()` in `finally`). Defensive: zero `@Async` usages exist today but the executor is wired for future use.

### Verification (run to confirm)
- `cd order-service && ./mvnw test -Dtest="OrderOutboxIT,OrderServiceImplTest,OrderConcurrencyTest"` — **BUILD SUCCESS 31/31** (verified 2026-05-20)
- `bash script/test_correlation_id.sh` — all 5 services must log the injected UUID (requires running Docker stack).

---

## ~~Step 6 — Cart-service product-validate caching  (P1, IMP-7)~~ ✅ DONE 2026-05-20

**Goal:** `POST /api/v1/cart/items` P95 < 40ms at 500 RPS.

### Changes landed
1. **`cart-service/internal/client/product_client.go`** — added unexported `productCache` interface (`get`/`set`) + `redisProductCache` impl (key `product:v:{id}`, 5s TTL). `GetProduct()` checks cache first and returns immediately on hit, bypassing `cb.Allow()` entirely. On HTTP success (ACTIVE product only), calls `cache.set()` before returning. CB and retry loop remain unchanged for cache misses. Non-ACTIVE products and 404s are NOT cached.
2. **`cart-service/internal/client/product_client.go`** — `NewProductClient(baseURL string, rdb *redis.Client)` now accepts the `*redis.Client` already initialized in `main.go`.
3. **`cart-service/cmd/server/main.go` line 95** — `client.NewProductClient(cfg.ProductServiceURL, rdb)` — passes the existing `rdb` (no new infrastructure needed).
4. **`cart-service/internal/client/product_client_test.go`** (new) — 2 unit tests using an in-memory `mapCache` fake (no miniredis dependency): `TestGetProduct_CacheHit_SkipsHTTP` (asserts zero HTTP calls on hit) and `TestGetProduct_CacheMiss_PopulatesCache` (asserts cache populated after HTTP success).

### Verification (run to confirm)
- `cd cart-service && go test -race ./internal/client/...` — **PASS** (verified 2026-05-20); full suite `go test -race ./...` — **3 packages PASS**
- `bash script/test/phase2_run.sh` → `cart_ops.json` P95 < 40ms (needs running stack).
- `bash script/test/chaos_cb_cart.sh` — CB still opens correctly on misses (needs running stack).

---

## ~~Step 7 — Compensation latency + inventory cache freshness  (P2, IMP-3 + IMP-5)~~ ✅ DONE 2026-05-20

**Goal:** Compensation TTC P95 < 2s; `GET /inventory/:id` reflects DB state within a few seconds of a stock mutation.

### Changes landed
1. **`order-service/pom.xml`** — added `spring-retry` + `spring-aspects` dependencies (required for `@Retryable` AOP).
2. **`order-service/src/main/java/com/ecommerce/order_service/config/AsyncConfig.java`** — added `@EnableRetry` to activate Spring Retry on the application context.
3. **`order-service/src/main/java/com/ecommerce/order_service/service/impl/OrderServiceImpl.java`** — added `@Async("taskExecutor")` to `releaseStockForOrder`: Kafka consumer thread returns immediately after marking order CANCELLED; stock release runs on the pre-existing `order-async-*` thread pool with MDC correlationId captured by the pool's `TaskDecorator`.
4. **`order-service/src/main/java/com/ecommerce/order_service/client/ProductServiceClient.java`** — added `@Retryable(3×, 100ms→200ms→400ms backoff)` to `releaseStock`; added missing `HttpServerErrorException` catch (already present in `reserveStock` but absent from `releaseStock`).
5. **product-service — no changes needed**: `@CacheEvict(value="product", key="#productId")` already on both `reserveStock` and `releaseStock` (Step 2); `updateProduct` uses `@CachePut(value="product", key="#id")` covering admin restock. Cache name is `"product"` (30-min TTL), key pattern `product-service::product::{id}`.

### Verification (run to confirm)
- `cd order-service && ./mvnw test -Dtest="OrderServiceImplTest,OrderOutboxIT,OrderConcurrencyTest"` — **PASS 25/25** (verified 2026-05-20)
- `bash script/test/phase1_run.sh` → `saga_fail.json` `compensation_ttc_ms p95 < 2000` (needs running stack).
- Race test teardown line (`script/k6/race_inventory.js`) — `cached_stock` immediately reflects 0 after the race resolves (needs running stack).

---

## ~~Step 8 — Latency tails: AI warm-up + Redis AOF  (P2, IMP-9 + IMP-10)~~ ✅ DONE 2026-05-20

**Goal:** AI per-layer P95 within targets (embed <100ms, vector <50ms, rerank <30ms); zero Redis P99 latency spikes during burst writes.

### Changes landed
1. **`ai-service/main.py`** — added 3× `model.encode(["warmup"] * 3, normalize_embeddings=True)` calls via `run_in_executor` after model load, before `yield`. Forces tokenizer + BLAS paths to initialize before the first user request.
2. **`product-service/src/main/java/com/ecommerce/product_service/service/CacheWarmupService.java`** — injected `AISearchService`; added new concurrent `warmAI()` method (`@EventListener(ApplicationReadyEvent.class)` + `@Async("taskExecutor")`). Fires 3 representative queries (`"laptop"`, `"shoes"`, `"coffee maker"`) via `aiSearchService.search(q, 5, null, null)`. Warms: ai-service inference + pgvector `ivfflat.probes` planner cache + seeds aiSearch Redis cache. Catches all exceptions → WARN log, never blocks startup.
3. **`docker-compose.yml`** — Redis command changed to `redis-server --appendonly yes --appendfsync everysec --no-appendfsync-on-rewrite yes`. The `--no-appendfsync-on-rewrite yes` flag prevents main-thread fsync stalls while `bgrewriteaof` holds the OS page cache.
4. **`README.md`** — added Redis AOF tuning note with production trade-off guidance (`appendfsync always` for max durability, `no` for max throughput).

### Verification (run to confirm)
- `cd product-service && ./mvnw test` — **PASS 78/78** (verified 2026-05-20)
- `bash script/test/phase2_run.sh` — re-aggregate AI per-layer rows (parser greps `ai.search.layer` log lines); targets embed <100ms, vector <50ms, rerank <30ms P95 (needs running stack).
- `script/k6/results/monitors/redis.log` — peak max latency stays sub-10ms; the 150ms outlier should disappear (needs running stack).

---

## ~~Step 9 — Test infra + frontend polish + spec reconciliation  (P2 + P3, IMP-15 + IMP-16 + IMP-17 + IMP-14)~~ ✅ DONE 2026-05-20

**Goal:** flaky integration tests stable; frontend passes Playwright responsive + a11y; nginx rate-limit spec and config aligned.

### Changes landed
1. **`payment-service/internal/integration/payment_idempotency_test.go`** — added `testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(30*time.Second))` to `tcpostgres.Run()`; added `dsn = strings.Replace(dsn, "::1", "127.0.0.1", 1)` after `ConnectionString()` to force IPv4 on macOS. Added imports: `"strings"`, `"time"`, `testcontainers-go`, `testcontainers-go/wait`.
2. **`payment-service/internal/integration/payment_kafka_test.go`** — same wait strategy + IPv4 fix applied to the Postgres container block in `TestDuplicateDeliveryIdempotency`; same imports added.
3. **`frontend/src/components/ui/button.tsx`** — removed `whitespace-nowrap` from the base CVA class string. Button text can now reflow at 320px viewport, unblocking the Playwright responsive spec.
4. **`frontend/src/components/layout/Navbar.tsx`** — added `aria-label="Open cart"` to the SVG cart `<button>` (visible on every page; was the primary axe violation).
5. **`frontend/src/features/cart/CartDrawer.tsx`** — added `aria-label="Close cart"` to the `✕` close `<button>`.
6. **`frontend/src/features/cart/CartItem.tsx`** — added `aria-label="Decrease quantity"` / `aria-label="Increase quantity"` / `aria-label="Remove item"` to the three icon-only buttons.
7. **`frontend/src/pages/ProductDetailPage.tsx`** — added `aria-label="Decrease quantity"` and `aria-label="Increase quantity"` to the quantity stepper buttons.
8. **`docs/testing/testing_target.md` §7.D** — updated rate-limit thresholds to match actual nginx burst config: General API 11→16th request (burst=5 nodelay); Auth 6→9th request (burst=3 nodelay). Chose Option B (amend spec, keep burst values) because burst provides good UX for momentary spikes and the k6 scripts already assert the correct 16/9 thresholds.

### Verification (run to confirm)
- `cd payment-service && go build -tags=integration ./internal/integration/...` — **PASS** (verified 2026-05-20, both test files compile clean with new imports)
- `cd frontend && npx tsc --noEmit` — **PASS** (verified 2026-05-20, zero type errors)
- `cd payment-service && go test -tags=integration -v -count=5 -run TestConcurrentIdempotency ./internal/integration/...` — pass 5/5 times (needs running Docker for testcontainers).
- `cd frontend && npx playwright test responsive.spec.ts` — all 3 pages `scrollWidth - clientWidth ≤ 1` (needs running dev server).
- `cd frontend && npx playwright test a11y.spec.ts` — zero serious/critical axe violations (needs running dev server).
- `bash script/test/phase3_run.sh` — §7.D-API and §7.D-Auth rows pass against updated 16/9 thresholds.

---

## Final regression gate

Once Steps 1–8 land (Step 9 can be in parallel), run the full test cycle in this order:

```bash
bash script/test/phase1_run.sh        # functional + saga TTC
bash script/test/phase2_run.sh        # load + throughput
bash script/test/phase3_run.sh        # chaos
bash script/test/phase4_run.sh        # testing debt
bash script/test/phase5_run.sh        # frontend
```

**Definition of done** (per IMP):
- §1-User row → PASS  (IMP-6 closed)
- §1-Order, §3.A → PASS  (IMP-2, IMP-8, IMP-11 closed)
- Mid-Saga `pending_count == 0`  (IMP-13 closed)
- §7.C correlation id → PASS  (IMP-4 closed)
- §1-Cart → PASS  (IMP-7 closed)
- §2-Comp → PASS  (IMP-3 closed); race teardown shows fresh stock  (IMP-5 closed)
- §4-Embed, §4-Vector, §4-Rerank → PASS  (IMP-9 closed); §5-Redis peak < 10ms  (IMP-10 closed)
- §9.A-Idem flakes 0/5 times  (IMP-15 closed)
- §8.D-Resp, §8.D-A11y → PASS  (IMP-16, IMP-17 closed)
- §7.D rows match whichever side of IMP-14 was chosen
- All axios.test.ts cases pass including the 401-refresh case  (IMP-18 closed)

**Re-update:**
- `docs/testing/test_result.md` — executive summary should land on **35 / 35 PASS** (or rows that intentionally stay open with explicit notes).
- `docs/testing/improvement performance plan.md` — strike through closed IMPs and add the date.

---

## Out of scope for this plan

- **IMP-12** is direction, not work — already baked into the Step 1 → Step 2 ordering.
- New features. This is a hardening pass only.
- Production-grade observability (Prometheus / Grafana). Log-grep based reporting is sufficient until we ship.
- Network-level chaos (latency injection, packet loss). Phase 3 deferred this; re-evaluate after Steps 1–3.
