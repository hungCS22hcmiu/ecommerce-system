# System Testing Plan

This plan defines how every benchmark in [`testing_target.md`](./testing_target.md) will be verified. It is phased to match the §10 verification roadmap (Functional → Load → Resilience → Testing Debt) and adds a frontend phase that the target doc calls out as currently un-automated.

Results from executing this plan are recorded in [`test_result.md`](./test_result.md). Any FAIL or AT-RISK target is then triaged into [`improvement performance plan.md`](./improvement%20performance%20plan.md).

> **Execution status (2026-05-19): all 5 phases run end-to-end.** See the executive summary at the top of [`test_result.md`](./test_result.md) for a roll-up of 35 evaluated targets → 17 PASS / 14 FAIL / 1 AT_RISK / 3 INFRA_FLAKE-or-MISSING, and 18 improvement-plan entries (1× P0, 8× P1, 7× P2, 1× P3, 1× direction). Each phase has its own implementation doc — [`phase1_implementation.md`](./phase1_implementation.md), [`phase2_implementation.md`](./phase2_implementation.md), [`phase3_implementation.md`](./phase3_implementation.md), [`phase4_implementation.md`](./phase4_implementation.md), [`phase5_implementation.md`](./phase5_implementation.md) — and a re-runnable orchestrator under `script/test/phaseN_run.sh`.

---

## 0. Tooling & Environment

| Concern | Choice | Rationale |
|---|---|---|
| Stack | `docker compose up --build -d` against `nginx:80` | Single command brings up all 8 services + Kafka + Redis + Postgres |
| Health gate | `bash script/health-dashboard.sh` before every run | Confirms `/health/ready` on user, product, cart, order, payment, ai |
| Load tool | **k6** v0.50+ | Scriptable JS, native P95 thresholds, `--summary-export` for evidence files |
| Load scripts | `script/k6/*.js` | Three starters already exist (`cart_ops.js`, `order_create.js`, `product_browse.js`); plan extends + adds |
| Kafka producers (chaos / saga replay) | `kcat` + Go test harness | Direct topic writes for idempotency and DLQ scenarios |
| Frontend | **Vitest** (unit/component) + **Playwright** (E2E) | Required because `frontend/package.json` currently has zero test runners |
| Chaos | `docker compose pause/kill/stop` | Sufficient for service-down/cold-start scenarios; defer `pumba` until network-fault tests are needed |
| Monitoring sidecars | `pg_stat_activity` poller, `redis-cli --latency-history`, `kafka-consumer-groups.sh --describe`, `docker stats` | Capture infrastructure metrics during load runs |
| Evidence | Each scenario writes JSON to `script/k6/results/<scenario>.json`; aggregator emits Markdown rows into `test_result.md` |

**Standard environment variables**

```
BASE_URL=http://localhost
ADMIN_EMAIL / ADMIN_PASSWORD          # from script/sample_users.sql
CUSTOMER_EMAIL / CUSTOMER_PASSWORD
SELLER_EMAIL / SELLER_PASSWORD
```

---

## Phase 1 — Functional & Saga Correctness

Verifies: **§2 Saga TTC**, **§3.B Race condition**, **§7.B Saga idempotency**, **§7.C Correlation IDs**.

| Target | Test artifact | Method | Pass criterion |
|---|---|---|---|
| Happy path TTC P95 < 2.0s | `script/k6/saga_happy.js` (new) | Create order, poll `/api/v1/payments/order/:id` every 50ms until terminal, record elapsed | P95 across 200 iterations < 2000ms |
| Payment failure TTC P95 < 1.5s | `script/k6/saga_fail.js` (new) | Trigger `payment-service` decline (amount-trigger or chaos flag); assert order → CANCELLED | P95 < 1500ms |
| Compensation TTC P95 < 2.0s | extend `script/e2e-payment.sh` | After CANCELLED, GET `/api/v1/inventory/:id` and verify stock restored | P95 < 2000ms |
| Race condition: 10 users → 1 success | `script/k6/race_inventory.js` (new); anchors on `InventoryConcurrencyTest.java` | Seed product with `quantity=1`, fire 10 concurrent `POST /orders` | Exactly 1× 201, 9× 409; final `quantity = 0` |
| Saga idempotency — exactly one payment | `payment-service/internal/integration/saga_replay_test.go` (new) | Produce same `orders.created` event 3× via embedded Kafka | 1 `payments` row; later attempts return `ErrDuplicateIdempotencyKey` |
| Correlation ID 100% propagation | `script/test_correlation_id.sh` (new) | Inject `X-Correlation-ID: <uuid>` at nginx, exercise full saga, `docker compose logs` and grep for uuid in user/cart/product/order/payment | uuid found in all 5 service logs |

Existing assets already covering portions of Phase 1: `script/e2e-test.sh`, `script/e2e-payment.sh`, `OrderConcurrencyTest.java`, `payment_idempotency_test.go`.

---

## Phase 2 — Load & Throughput

Verifies: **§1 Service performance**, **§3.A Concurrent capacity**, **§4 AI search**, **§5 Infrastructure**, **§6 Connection pooling**.

Every scenario embeds a k6 `thresholds` block; a breach fails the run automatically.

### 2.A Per-service performance (§1)

| Endpoint | k6 scenario | Threshold |
|---|---|---|
| `POST /api/v1/auth/login` | `script/k6/auth_login.js` (new) — uses seeded users from `sample_users.sql` | `http_req_duration{name:login} p(95)<300`, `http_reqs rate>=100/s`, error <1% |
| `POST /api/v1/cart/items` | extend `script/k6/cart_ops.js` | `p(95)<40`, `rate>=500/s` |
| `GET /api/v1/products/search` | extend `script/k6/product_browse.js` | `p(95)<150`, `rate>=150/s` |
| `POST /api/v1/orders` | extend `script/k6/order_create.js` | `p(95)<400`, `rate>=50/s`, error <0.1% |
| Kafka `orders.created` consumer | `script/k6/payment_kafka_throughput.sh` (new) — `kcat` produces 10k messages; handler latency extracted from payment-service logs | mean handler latency <100ms, throughput ≥200 msg/s |

### 2.B AI semantic search (§4)

`script/k6/ai_search.js` (new): exercises `/api/v1/products/ai-search?q=…&limit=10` with a rotating query corpus.

| Layer | Threshold | Capture |
|---|---|---|
| Total request P95 | < 250ms | k6 |
| Embedding generation P95 | < 100ms | `AISearchService` Micrometer `Timer("ai.embed.duration")` — wire if absent |
| Vector search P95 | < 50ms | `Timer("ai.vector.duration")` around `findIdsBySemanticSimilarity` |
| Re-ranking P95 | < 30ms | `Timer("ai.rerank.duration")` |
| Throughput | ≥ 20 RPS | k6 `rate` |
| Searchability lag P95 | < 1.0s | `AISearchabilityLagIT.java` (new) — POST product, poll `/ai-search` until present |
| Cold start | < 15s | `script/test_ai_cold_start.sh` (new) — restart container, poll `/health/ready` every 250ms |
| Memory | ≤ 1.5 GB | `script/monitor_ai_mem.sh` (new) — `docker stats ai-service` sampled 1Hz during AI load |

### 2.C Composite checkout — 50 VU (§3.A)

`script/k6/checkout_50vu.js` (new): 50 VUs run login → browse → add-to-cart → create-order on loop for 5 minutes.

Threshold: `error_rate<0.001`, `http_req_duration p(95)<1000`.

### 2.D Infrastructure & pooling sidecars (§5, §6)

Run as parallel processes alongside each Phase 2.A scenario.

| Monitor | Script | Assertion |
|---|---|---|
| Postgres per-DB conns | `script/monitor_pg_connections.sh` (new) — polls `pg_stat_activity GROUP BY datname` every 1s into CSV | Each DB ≤ pool limit (25 Go / 20 Java); total across 5 DBs ≤ 150 |
| Redis latency | `script/monitor_redis.sh` (new) — `redis-cli --latency-history -i 1` | P99 acquisition < 1ms; baseline latency < 0.2ms |
| Kafka lag | `script/monitor_kafka_lag.sh` (new) — `kafka-consumer-groups.sh --describe` every 5s | Lag < 50 at any sample during peak |
| HikariCP leaks | scan `docker compose logs product-service order-service` after run | Zero `Connection leak detected` warnings |
| Pool acquisition P95 | Micrometer `hikaricp.connections.acquire` (already exposed) → `/actuator/prometheus` | P95 < 5ms |

---

## Phase 3 — Resilience & Chaos

Verifies: **§3.C Order deadlock**, **§7.A Circuit breaker**, **§7.D Rate limits**, plus mid-saga recovery.

| Target | Test | Pass criterion |
|---|---|---|
| Circuit breaker OPEN after 5 failures | `script/chaos_cb_cart.sh` (new): `docker compose pause product-service`, run k6 at 10 RPS for 60s against `POST /cart/items` and `GET /cart` | `cart-service` logs `state=OPEN` after ≤5 failures; subsequent `POST` returns fast-fail; `GET /cart` p95 < 20ms |
| Degraded GET cart < 20ms | same scenario | Captured by same k6 run |
| Order state-machine deadlock-free | extend `OrderConcurrencyTest.java` + new k6 producer that races `PAYMENT_COMPLETED` and `SHIPPED` events for the same orderId | Zero `DeadlockLoserDataAccessException`; no 500 responses; transitions either applied sequentially or rejected with explicit 409 |
| Nginx API rate limit 10 r/s | `script/k6/rate_limit_api.js` (new): burst 50 reqs in 1s to `/api/v1/products` | ≥ 40 responses are 429 |
| Nginx auth rate limit 5 r/min | `script/k6/rate_limit_auth.js` (new): 6 logins from one IP within 60s | 6th request 429 |
| Mid-saga service kill recovery | extend `script/loadtest-orders.sh`: while load runs, `docker compose kill payment-service && sleep 5 && docker compose start payment-service` | After settle, zero PENDING orders, DLQ empty, no duplicate payments |

---

## Phase 4 — Repository, Client & Security Testing Debt

Verifies: **§9.A**, **§9.B**, **§9.C**.

| Debt area | New / extended test | Coverage goal |
|---|---|---|
| `ProductRepository` FTS + pgvector | `ProductRepositoryQueryIT.java` (new, Testcontainers pgvector:pg16) — covers `findByFullTextSearch` (ranking, empty, tsquery escaping) and `findIdsBySemanticSimilarity` (probes set, no matches, dimension mismatch error) | 100% on these two methods |
| `OrderRepository` pessimistic lock | `OrderRepositoryLockIT.java` (new) — two threads call `findByIdWithLock`; second must block until first commits, then observe new state | 100% on `findByIdWithLock` |
| `PaymentRepository` idempotency | extend `payment-service/internal/integration/payment_idempotency_test.go` with N=20 concurrent same-key inserts | Exactly 1 row; 19× `ErrDuplicateIdempotencyKey` |
| Cart `ProductClient` circuit breaker + retries | new unit test using `httptest.Server` returning 503 then 200 — assert state transitions CLOSED→OPEN→HALF_OPEN→CLOSED and retry counts | 100% on `circuit_breaker.go` |
| `payment-service` Kafka consumer | extend `payment_kafka_test.go` with three scenarios: poison (malformed JSON → DLQ immediately), transient (3× retry 100/200/400ms then DLQ), permanent decline (no DLQ) | All three branches |
| Product `EmbeddingClient` | unit test stubbing AI with timeout / 500 / 200 — assert `AIServiceException` raised, write-through async failure logged WARN, caller not blocked | 100% on `EmbeddingClient.java` |
| `user-service/pkg/blacklist` | unit tests: revocation TTL respected, re-revoke idempotent, expired key removed | 100% line coverage |
| `user-service/pkg/verification` | unit tests: code generation entropy, cooldown enforcement, attempt-counter expiry | 100% |
| `user-service/pkg/reset` | unit tests: token TTL, single-use enforcement, reuse rejected | 100% |

**Coverage gates**
- Go services: `go test -race -coverprofile=cover.out ./...`; CI fails when listed packages < 80% lines or < 100% on lock/idempotency funcs.
- Java services: JaCoCo report; same thresholds.

---

## Phase 5 — Frontend UX & Reliability

Verifies: **§8** (Error communication, optimistic updates, refresh-token flow, component integrity).

Add tooling to `frontend/package.json`:

```jsonc
"scripts": {
  "test":      "vitest run",
  "test:watch":"vitest",
  "test:e2e":  "playwright test"
}
```

### 5.A Vitest unit/component

| Test file | Verifies |
|---|---|
| `src/lib/__tests__/axios.test.ts` | 401 interceptor queue: three concurrent failed requests, mock single `/auth/refresh`, all three replay with new token, zero `/login` redirects (§8.C) |
| `src/features/cart/__tests__/useCart.test.ts` | TanStack optimistic increment then rollback within 2s when API returns error (§8.B) |
| `src/lib/__tests__/toast.test.ts` | Backend error message ("Password must be at least 8 characters") rendered verbatim, not generic fallback (§8.A) |

### 5.B Playwright E2E (runs against `docker compose` stack on `http://localhost:3001`)

| Spec | Verifies |
|---|---|
| `tests/e2e/auth.spec.ts` | Invalid password → backend-specific toast visible |
| `tests/e2e/cart.spec.ts` | Add-to-cart while offline (network condition) → badge increments then rolls back |
| `tests/e2e/refresh.spec.ts` | Force-expire access token mid-page-load → all parallel requests succeed after refresh, no `/login` redirect |
| `tests/e2e/responsive.spec.ts` | 320px viewport on Home / Cart / Product detail → no horizontal overflow, all CTAs reachable |
| `tests/e2e/a11y.spec.ts` | `@axe-core/playwright` scan on Home / Product / Cart / Checkout → zero serious violations, all interactive elements have accessible names |

---

## 6. Execution Sequence

1. **Gate:** `docker compose up --build -d && bash script/health-dashboard.sh`. Abort if any service unhealthy.
2. **Phase 1** (functional) — fast, must pass before Phase 2.
3. **Phase 2** (load) — run Phase 2.A endpoints sequentially with sidecar monitors from 2.D running in parallel each time. Then 2.B (AI), then 2.C (50 VU composite).
4. **Phase 3** (chaos) — runs only after Phase 1+2 pass; chaos perturbs stack state.
5. **Phase 4** (testing debt) — independent; can run in parallel with Phase 3 on a separate stack copy.
6. **Phase 5** (frontend) — independent; can run anytime after stack is healthy.
7. **Aggregate:** `python script/k6/aggregate_results.py` (new) merges every `script/k6/results/*.json` and test exit codes into `docs/testing/test_result.md`.
8. **Triage:** For every `FAIL` or `AT-RISK` row, append an entry to `docs/testing/improvement performance plan.md`.

---

## 7. Reporting Format (`test_result.md` schema)

Each phase writes a Markdown table with the following columns; rows map 1:1 to targets in `testing_target.md`.

| Target ID | Target | Observed | Status | Evidence |
|---|---|---|---|---|
| §1-User | `/auth/login` P95 < 300ms, 100 RPS | _filled by aggregator_ | PASS / FAIL / AT-RISK | `script/k6/results/auth_login.json` |

`AT-RISK` = observed within 10% of the target. Drives improvement-plan entries even when nominally passing.

---

## 8. Critical Files (read-only references)

**Targets & related docs:** `docs/testing/testing_target.md`, `docs/adrs/locking-strategy.md`, `docs/adrs/saga-resilience.md`, `docs/technical/service_integration.md`.

**Existing load / e2e:** `script/loadtest-orders.sh`, `script/perf-baseline.sh`, `script/e2e-test.sh`, `script/e2e-payment.sh`, `script/k6/{cart_ops,order_create,product_browse}.js`.

**Concurrency anchors to extend or mirror:**
- `product-service/src/test/java/com/ecommerce/product_service/integration/InventoryConcurrencyTest.java`
- `order-service/src/test/java/com/ecommerce/order_service/integration/OrderConcurrencyTest.java`
- `payment-service/internal/integration/payment_idempotency_test.go`
- `payment-service/internal/integration/payment_kafka_test.go`
- `user-service/internal/integration/auth_flow_test.go`
- `cart-service/internal/repository/redis_cart_repository.go`

**Production-under-test files referenced by targets:**
- `cart-service/internal/client/circuit_breaker.go` (5-failure / 30s timeout)
- `nginx/nginx.conf` (rate-limit zones lines 9–10; route attachments throughout)
- `ai-service/main.py` (lifespan model load, `/health/ready`)
- `product-service/src/main/java/com/ecommerce/product_service/client/EmbeddingClient.java`
- `product-service/src/main/java/com/ecommerce/product_service/repository/ProductRepository.java` (FTS + pgvector SQL)
- `order-service/src/main/java/com/ecommerce/order_service/repository/OrderRepository.java` (`findByIdWithLock`)
- `payment-service/internal/repository/payment_repository.go` (idempotency key)
- `user-service/pkg/{blacklist,verification,reset}/`
- `frontend/src/lib/axios.ts`, `frontend/src/features/cart/useCart.ts`, `frontend/src/lib/toast.ts`

**Config sources:**
- `product-service/src/main/resources/application.yaml` (HikariCP: max 20, min 5, leak 60s, timeout 30s)
- `order-service/src/main/resources/application.yaml` (same)
- `user-service/cmd/server/main.go` and `cart-service/cmd/server/main.go` (GORM: MaxOpen 25, MaxIdle 5)

---

## 9. Exit Criteria

The plan is considered fully executed when:

1. Every target in `testing_target.md` has a corresponding row in `test_result.md` with `Observed` and `Status` filled.
2. Every `FAIL` and `AT-RISK` row has a matching entry in `improvement performance plan.md` with: target, observed, suspected bottleneck, proposed fix, priority.
3. `make test-all` (future Makefile aggregating phases 1–2) returns 0 on a clean stack.
4. Phases 3–5 documented as scheduled (nightly chaos, weekly FE E2E, per-PR Vitest).
