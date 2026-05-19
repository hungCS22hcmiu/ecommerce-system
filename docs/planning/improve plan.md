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

## Step 1 — Unblock the front door: `/auth/login` throughput  (P0, IMP-6)

**Goal:** sustain 100 RPS on `POST /api/v1/auth/login` with P95 < 300ms, error rate < 1%.

### Explore
- `user-service/internal/handler/auth_handler.go` — how `Login()` is wired; identify the bcrypt call site and the login-attempt counter.
- `user-service/internal/service/auth_service.go` — current `SELECT ... FOR UPDATE` on login-attempt rows.
- `user-service/pkg/password/password.go` — bcrypt cost (likely the default 12).
- `user-service/cmd/server/main.go` — Gin server config (timeouts, max conns).
- Confirm Redis is already wired (the `pkg/blacklist` and `pkg/verification` packages already use go-redis). Reuse that client.

### Key changes
1. **Bcrypt worker pool with bounded queue** in `auth_service.go`:
   - New file `user-service/pkg/password/pool.go`: a goroutine pool of size `runtime.NumCPU()` consuming a `chan request` of `{password, hash, replyCh}` jobs. Bounded queue (e.g. 256). If full, reject with a sentinel `ErrBcryptOverload` → handler maps to **HTTP 503 with `Retry-After: 1`**. This is the load-shedding pattern.
   - Replace direct `bcrypt.CompareHashAndPassword(...)` calls with `pool.Verify(ctx, hash, password)`.
2. **Env-driven bcrypt cost** in `pkg/password/password.go`:
   - Read `BCRYPT_COST` env var (default 12 for prod, 10 for dev).
   - Add to `docker-compose.yml` for user-service: `BCRYPT_COST: ${BCRYPT_COST:-10}` for the test stack.
3. **Replace `SELECT FOR UPDATE` login counter with Redis INCR**:
   - In `auth_service.go::handleFailedLogin`, replace the DB row update with `INCR login_attempts:{email}` + `EXPIRE login_attempts:{email} 900`.
   - Lockout check: `GET login_attempts:{email}` ≥ N → return 423 Locked.
   - Drop the related Postgres column/transaction. Migration: leave column nullable; clean up later.
4. **Gin server tuning** in `cmd/server/main.go`:
   - Set `&http.Server{ ReadHeaderTimeout: 5s, IdleTimeout: 30s, ReadTimeout: 15s, WriteTimeout: 30s }`.

### Verification
- `cd user-service && go test -race -count=1 ./pkg/password/... ./internal/service/...` — unit/race tests pass.
- `bash script/test/phase2_run.sh` — re-run §1-User specifically:
  - Target: `auth_login.json` shows **P95 < 300ms, throughput ≥ 100/s, http_req_failed < 1%**.
- `bash script/test/phase1_run.sh` — saga_happy / saga_fail no longer block on slow logins (should improve TTC numbers indirectly).
- Look for **503 with Retry-After** in `auth_login.log` under overload — confirms load shedding works.

---

## Step 2 — Order/inventory retry strategy  (P1, IMP-2 + IMP-8 + IMP-11)

**Goal:** `POST /api/v1/orders` P95 < 400ms at 50 RPS, error rate < 0.1%; 50 VU composite checkout error rate < 0.1%.

### Explore
- `product-service/.../service/InventoryServiceImpl.java` — current `@Retryable(maxAttempts=3)` on the reserve flow.
- `product-service/.../repository/ProductRepository.java` — find the `@Version` field on `Product` and the reserve/release JPQL/native queries.
- `product-service/.../exception/InsufficientStockException.java` — the thrown error after retry exhaustion.
- `order-service/.../service/impl/OrderServiceImpl.java::createOrder` — how the order propagates the 409 back to the client.
- Phase 1 evidence: `script/k6/results/logs/saga_happy.log` (19/200 INSUFFICIENT_STOCK at 500 seeded stock).

### Key changes
1. **Conditional UPDATE instead of optimistic version check** in `ProductRepository.java` (preferred path):
   ```sql
   UPDATE products
   SET stock_reserved = stock_reserved + :qty
   WHERE id = :id
     AND stock_quantity - stock_reserved >= :qty
   ```
   Returns row-update count: 1 → success, 0 → genuine OOS. Drop `@Version`-based optimistic locking from the reserve path entirely. Same pattern for release (subtraction).
2. **Distinguish "true OOS" from "retry-exhausted"**:
   - If the conditional UPDATE returns 0 rows, query `stock_quantity - stock_reserved` once more — if it's > 0, the conflict is contention (theoretically impossible with conditional UPDATE, but safety belt), return 503; if 0, return 409 InsufficientStock.
3. **Bust the inventory cache on every successful mutation**:
   - In `ProductCacheService` (or wherever reserve/release commit hooks live), `DEL inventory:{productId}` after a successful row update. Pairs with IMP-5.
4. **Bonus (cheap):** add `@Index` on `(id, stock_reserved)` if missing.

### Verification
- `cd product-service && ./mvnw test -Dtest=InventoryConcurrencyTest` — existing test must still pass with the new conditional UPDATE.
- `cd product-service && ./mvnw test -Dtest=ProductRepositoryQueryIT` — Phase 4 query IT must still pass.
- `bash script/test/phase2_run.sh` — target rows:
  - `order_create.json`: P95 < 400ms, throughput ≥ 50/s, http_req_failed < 0.1%.
  - `checkout_50vu.json`: error rate < 0.1%.
- Direct race test: `bash script/k6/race_inventory.js` — 1×201 + 9×409, no 503s.

---

## Step 3 — Transactional outbox in order-service  (P1, IMP-13)

**Goal:** zero PENDING orders left behind across payment-service restart, even at sustained load.

### Explore
- `order-service/.../kafka/OrderEventProducer.java` — current `kafkaTemplate.send(...)` is fire-and-forget; the DB transaction commits before the Kafka publish settles.
- `order-service/src/main/resources/db/migration/` — Flyway directory; pick a free version number for the new migration.
- `payment-service/internal/kafka/consumer.go` — confirm idempotency key (`orderId.String()`) is set so reaper-driven re-publishes are safe.
- Phase 3 evidence: `script/k6/results/chaos_saga_kill.json` (`pending_count=1` of 898).

### Key changes
1. **New table** via Flyway migration `V7__orders_outbox.sql`:
   ```sql
   CREATE TABLE orders_outbox (
       id           BIGSERIAL PRIMARY KEY,
       order_id     UUID NOT NULL,
       payload      JSONB NOT NULL,
       headers      JSONB NOT NULL,
       created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
       published_at TIMESTAMPTZ
   );
   CREATE INDEX idx_orders_outbox_unpublished ON orders_outbox(published_at) WHERE published_at IS NULL;
   ```
2. **Insert event into outbox inside the order-create transaction** (`OrderServiceImpl.createOrder` — same `@Transactional` scope as the order insert).
3. **Outbox publisher worker** — new component `OutboxPublisher` with `@Scheduled(fixedDelay=100)`:
   - Pull up to 100 unpublished rows ordered by `id` (`FOR UPDATE SKIP LOCKED`).
   - For each: `kafkaTemplate.send(...)` then mark `published_at = now()`.
4. **Remove the inline publish call** from `OrderEventProducer.send()` (or keep it as a fast-path with `try-catch` that falls through to outbox).
5. **Reaper job** (orthogonal but cheap): `@Scheduled(fixedDelay=60s)` sweeps `orders` where `status='PENDING' AND created_at < now() - INTERVAL '2 minutes'` AND no row in `payments` → re-insert outbox row.

### Verification
- New IT: `OrderOutboxIT.java` (Testcontainers PG + EmbeddedKafka). Pattern after `OrderConcurrencyTest`. Assert: kill the publisher mid-batch, restart; all outbox rows eventually `published_at IS NOT NULL` and Kafka has every message exactly once.
- `bash script/test/chaos_saga_kill.sh` — must report **`pending_count=0`** across N runs.
- Sanity: under 100-order load, total Kafka messages on `orders.created` == total orders created (no drops, no dupes — checked via `kafka-run-class kafka.tools.GetOffsetShell`).

---

## Step 4 — Frontend axios `/auth/refresh` deadlock  (P1, IMP-18)

**Goal:** an expired refresh token must cleanly redirect to `/login`, not silently hang.

### Explore
- `frontend/src/lib/axios.ts:22-57` — the 401 interceptor.
- `frontend/src/lib/__tests__/axios.test.ts` — Phase 5 already wrote the test that surfaced this bug; rerun it to confirm the failing case.

### Key changes
**One-line edit** in `frontend/src/lib/axios.ts:26`:
```ts
const isAuthEndpoint =
  original.url?.includes('/auth/login') ||
  original.url?.includes('/auth/register') ||
  original.url?.includes('/auth/refresh')   // ← new
```

### Verification
- Update `frontend/src/lib/__tests__/axios.test.ts` to also cover the "refresh returns 401" case (currently it returns 500 to side-step the bug). After the fix, the 401-refresh case must produce the same outcome as the 500 case: `clearAuth()` + `window.location.href = '/login'`.
- `cd frontend && npx vitest run` — all axios tests pass.
- Manual smoke in the browser: revoke a refresh token in Redis (`DEL refresh_token:<userId>`), refresh the page, verify a single redirect to `/login` with no hanging spinners.

---

## Step 5 — Correlation-ID propagation across the saga  (P1, IMP-4)

**Goal:** an `X-Correlation-ID` injected at nginx appears in every Go and Java service log line, including payment-service for Kafka-consumed events.

### Explore
- `order-service/.../kafka/OrderEventProducer.java:26` — current `MDC.get("correlationId")` lookup.
- `order-service/.../filter/CorrelationFilter.java` — how MDC is populated on inbound HTTP.
- Find any `@Async` or `CompletableFuture` between the controller and the producer call (likely root cause of MDC loss).
- `payment-service/internal/kafka/consumer.go:144` — confirm header read is correct (was confirmed in Phase 1, just verify after the order-side change).

### Key changes
1. **Add a diagnostic log line** before `kafkaTemplate.send()` in `OrderEventProducer.java`:
   ```java
   log.info("about to publish orders.created, MDC={}", MDC.getCopyOfContextMap());
   ```
   Run a single `bash script/test_correlation_id.sh` — if the log shows `MDC={}` (empty), proceed with the fix below; if MDC is populated, the bug is elsewhere (probably an `@Async`).
2. **Pass correlation ID explicitly** through the producer signature:
   ```java
   void publish(OrderCreatedEvent event, String correlationId);
   ```
   In the controller, read the header directly and pass it down — never trust MDC across thread/async boundaries.
3. If the controller has a `CompletableFuture.runAsync(...)` in front of the producer (audit during exploration), either remove it (the publish is fast) or wrap the executor with `TaskDecorator` that copies MDC.

### Verification
- `bash script/test_correlation_id.sh` — all 5 services (user, cart, product, order, **payment**) must log the injected UUID.
- `bash script/test/phase1_run.sh` — the `§7.C` row in `test_result.md` flips to PASS.

---

## Step 6 — Cart-service product-validate caching  (P1, IMP-7)

**Goal:** `POST /api/v1/cart/items` P95 < 40ms at 500 RPS.

### Explore
- `cart-service/internal/client/product_client.go::GetProduct` — current sync HTTP call inside the cart write path.
- `cart-service/internal/service/cart_service.go::AddItem` — the call site for `GetProduct`.
- `cart-service/internal/client/circuit_breaker.go` — to confirm the CB still wraps the cached path correctly.

### Key changes
1. **Add a small TTL Redis cache for product existence/price** in `cart-service`:
   - Key: `product:exists:{id}` (5 second TTL).
   - Value: minimal `ProductInfo{ID, Price, Status}` JSON.
   - `productClient.GetProduct()` checks Redis first, falls back to HTTP on miss (existing path), populates cache on success.
2. **Important:** keep the CB on the HTTP fallback (do NOT cache CB-OPEN errors).
3. **Bonus:** when the cache hits, skip the CB `Allow()` call entirely (no network involved).

### Verification
- New unit test in `cart-service/internal/client/product_client_test.go`: with a populated cache, the test stub's HTTP handler must NOT be called. Confirm cache hit short-circuits the path.
- `bash script/test/phase2_run.sh` → `cart_ops.json` P95 < 40ms.
- Re-run `bash script/test/chaos_cb_cart.sh` — CB still opens correctly when product-service is paused for misses.

---

## Step 7 — Compensation latency + inventory cache freshness  (P2, IMP-3 + IMP-5)

**Goal:** Compensation TTC P95 < 2s; `GET /inventory/:id` reflects DB state within a few seconds of a stock mutation.

### Explore
- `order-service/.../service/impl/OrderServiceImpl.java` — current synchronous `productServiceClient.releaseStock(...)` call after `payments.failed`.
- `product-service/.../service/serviceImpl/InventoryServiceImpl.java` — cache invalidation on reserve/release.
- `product-service/.../config/CacheConfig.java` — inventory cache key + TTL.

### Key changes
1. **Make `releaseStock` truly fire-and-forget**:
   - In `PaymentEventConsumer.onPaymentFailed`, mark the order CANCELLED first (synchronously, user-visible).
   - Then enqueue a release-stock task to a small `@Async` executor — already wired as `taskExecutor` in `AsyncConfig.java`.
   - Failure of the release retries via existing `@Retryable` on the client.
2. **Invalidate `inventory:{id}` on every stock mutation** (already partially covered in Step 2). Confirm:
   - `reserveStock` → `DEL inventory:{id}`.
   - `releaseStock` → `DEL inventory:{id}`.
   - Admin restock → same.

### Verification
- `bash script/test/phase1_run.sh` → `saga_fail.json` `compensation_ttc_ms p95 < 2000`.
- Race test teardown line (`script/k6/race_inventory.js`) — `cached_stock` immediately reflects 0 after the race resolves, not the 30-min stale value.

---

## Step 8 — Latency tails: AI warm-up + Redis AOF  (P2, IMP-9 + IMP-10)

**Goal:** AI per-layer P95 within targets (embed <100ms, vector <50ms, rerank <30ms); zero Redis P99 latency spikes during burst writes.

### Explore
- `product-service/.../service/serviceImpl/AISearchServiceImpl.java` — current logging is wired (Phase 2 change). Confirm per-layer log lines.
- `ai-service/main.py` — model load is in `lifespan`; nothing warms inference.
- `product-service/.../service/CacheWarmupService.java` — exists for products; mirror for AI.
- `docker-compose.yml:27-44` — current Redis config.

### Key changes
1. **AI warm-up at startup** (product-service):
   - New `@PostConstruct` in `CacheWarmupService`: fire 3-5 representative embeds via `EmbeddingClient.embed(...)` immediately after Spring is ready (catch and log any `AIServiceException`, do not fail startup).
   - Also: run `SELECT count(*) FROM products` + one `<=>` query against a known-good vector — warms PG buffers + `ivfflat.probes` planner cache.
2. **AI service warm-up** (`ai-service/main.py`):
   - At end of `lifespan` `async with`, run `model.encode(["warmup", ...])` 2-3 times so the first user request isn't paying for cold inference.
3. **Redis AOF tuning** in `docker-compose.yml` for the dev stack:
   - Change `command: redis-server --appendonly yes` → `command: redis-server --appendonly yes --appendfsync everysec --no-appendfsync-on-rewrite yes`.
   - Document in `README.md` that production should use a different fsync policy depending on durability vs latency trade-off.

### Verification
- `bash script/test/phase2_run.sh` — re-aggregate AI per-layer rows (parser greps `ai.search.layer` log lines).
- `script/k6/results/monitors/redis.log` — peak max latency stays sub-10ms (won't get fully sub-1ms on Docker Desktop for M1, but the 150ms outlier should disappear).

---

## Step 9 — Test infra + frontend polish + spec reconciliation  (P2 + P3, IMP-15 + IMP-16 + IMP-17 + IMP-14)

**Goal:** flaky integration tests stable; frontend passes Playwright responsive + a11y; nginx rate-limit spec and config aligned.

### Explore
- For IMP-15: `payment-service/internal/integration/payment_kafka_test.go` and `payment_idempotency_test.go` — current `tcpostgres.Run(...)` calls.
- For IMP-16: `grep -rE "min-w-\[(4|5|6)[0-9][0-9]px\]|w-\[(3|4|5)[0-9][0-9]px\]|whitespace-nowrap" frontend/src/` — locate the offending fixed-width element on Home/Products.
- For IMP-17: open the per-page axe report from `frontend/test-results/.../axe-home.json` (preserved via the spec's `testInfo.attach`) — identifies the 13 nodes with their color pairs and DOM selectors.
- For IMP-14: re-read `nginx/nginx.conf:9-11` and the spec in `docs/testing/testing_target.md` §7.D.

### Key changes
1. **IMP-15 — testcontainers wait strategy**:
   ```go
   pgCtr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
       tcpostgres.WithDatabase(...), ...,
       testcontainers.WithWaitStrategy(
           wait.ForListeningPort("5432/tcp").WithStartupTimeout(30*time.Second),
       ),
   )
   ```
   Plus a `dsn = strings.Replace(dsn, "::1", "127.0.0.1", 1)` after `ConnectionString(...)` to force IPv4 on macOS.
2. **IMP-16 — frontend overflow**: replace the offender with a Tailwind `sm:`-gated rule, or wrap the wide element in `<div className="overflow-x-auto">`. Verify across all 3 pages (Home, Products, Cart).
3. **IMP-17 — a11y contrast**: bulk-replace `text-gray-{400,500}` with `text-gray-{600,700}` where the parent background is white/light. Add `aria-label` to every icon-only `<button>` (lucide-react icons need labels). The axe report tells you exactly which nodes.
4. **IMP-14 — pick a side** (one-line PR either way):
   - Tighten config: `burst=0 nodelay` on both zones in `nginx/nginx.conf`.
   - OR amend spec: update `testing_target.md` §7.D from "11th request" / "6th request" to "16th request" / "9th request" to match burst=5 / 3.

### Verification
- IMP-15: `cd payment-service && go test -tags=integration -v -count=5 -run TestConcurrentIdempotency ./internal/integration/...` — pass 5/5 times.
- IMP-16: `cd frontend && npx playwright test responsive.spec.ts` — all 3 pages pass.
- IMP-17: `cd frontend && npx playwright test a11y.spec.ts` — zero serious/critical violations.
- IMP-14: `bash script/test/phase3_run.sh` — `§7.D-API` and `§7.D-Auth` rows assert against the new thresholds.

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
