# Phase 3 — Resilience & Chaos: Implementation Plan

> Companion to [`testing_plan.md`](./testing_plan.md) §Phase 3. Defines the concrete artifacts that will produce Phase 3 evidence in [`test_result.md`](./test_result.md).

## Context

Phase 3 verifies four target areas from `testing_target.md`:

- **§7.A Circuit breaker + degraded mode** — cart-service must open its breaker on product-service failure and keep `GET /cart` < 20ms from Redis.
- **§3.C Order state-machine deadlock** — concurrent `PAYMENT_COMPLETED` and `SHIPPED` events for the same order must serialize via the pessimistic lock; zero `DeadlockLoserDataAccessException`, zero 500s.
- **§7.D Nginx rate limits** — 10 r/s API and 5 r/min auth zones must return 429 once burst budget is consumed.
- **Mid-saga service kill recovery** — killing payment-service mid-batch must result in zero PENDING orders, empty DLQ, no duplicate payments after restart.

Today the repo has zero chaos artifacts. Three relevant building blocks already exist (do not re-create):
- Unit-level CB test `cart-service/internal/integration/product_contract_test.go:TestProductContract_CircuitBreakerOpens` proves CB opens after 5 failures and returns `client.ErrServiceUnavailable`.
- `OrderConcurrencyTest.java:concurrent_stateTransitions_exactlyOneWins` covers internal Java SHIP-vs-CANCEL race.
- `script/loadtest-orders.sh` provides the closest existing chaos sketch (SQL/DLQ assertions).

Phases 1–2 surfaced 12 improvement entries; Phase 3 is independent — it tests resilience invariants, not throughput.

**Decisions confirmed:**
- §7.D — test against current config; assert `≥35/50` general and `≥1/9` auth (matching burst=5 / burst=3). Spec/config mismatch goes into the improvement plan.
- §3.C — single **bash chaos script** races Kafka publishes against HTTP `PUT /orders/:id/ship` on the live stack.

---

## Findings From Exploration (anchor for the artifacts below)

| What | Where | Notes |
|---|---|---|
| Cart CB threshold | `cart-service/internal/client/circuit_breaker.go:46` | 5 failures, 30s timeout |
| Cart CB-open response | `cart-service/internal/handler/cart_handler.go:53` | HTTP **503** with code `SERVICE_UNAVAILABLE` |
| `GET /cart` is Redis-only | `cart-service/internal/repository/redis_cart_repository.go:39` | `HGETALL cart:{userId}` — no product-service call |
| CB silent on state changes | `circuit_breaker.go` | No log line on OPEN — detect via 503 response pattern |
| Order state machine | `order-service/.../service/OrderStateMachine.java` | Illegal transitions → `InvalidOrderStateException` → 409 |
| Order pessimistic lock | `OrderRepository.findByIdWithLock` | `@Lock(PESSIMISTIC_WRITE)`; no `@Retryable`; serializes naturally |
| Ship endpoint | `OrderController.java:56` → port 8082 | Reachable on host (nginx blocks externally) |
| Payment consumer | `payment-service/internal/kafka/consumer.go` | Group `payment-service`, manual commit AFTER processing |
| DLQ topic | `payments.dlq` | Inspect via `kafka-console-consumer --timeout-ms 5000` |
| Nginx zones | `nginx/nginx.conf:9-11, 54-56, 84` | `api_limit` 10r/s burst=5 nodelay; `auth_limit` 5r/min burst=3 nodelay |

---

## Targets → Artifacts

| Target | Artifact | Key mechanism |
|---|---|---|
| §7.A CB OPEN | `script/test/chaos_cb_cart.sh` (NEW) | Pause product-service; fire 10 `POST /cart/items`; assert ≥6 return 503 |
| §7.A Degraded GET /cart < 20ms | `script/k6/cart_get_degraded.js` (NEW) | Pre-seed cart, pause product-service, k6 10 RPS × 30s on `GET /cart`. Threshold `p(95)<20` |
| §3.C Deadlock-free state machine | `script/test/chaos_order_race.sh` (NEW) | 20 rounds × (Kafka `payments.completed` + HTTP `PUT /ship`) concurrently; grep order-service logs for `DeadlockLoserDataAccessException` + 5xx — assert 0/0 |
| §7.D API rate limit | `script/k6/rate_limit_api.js` (NEW) | 50 reqs / 1s burst to `/api/v1/products` via nginx; assert ≥35 are 429 |
| §7.D Auth rate limit | `script/k6/rate_limit_auth.js` (NEW) | 9 sequential `POST /auth/login` via nginx; assert ≥1 is 429 |
| Mid-saga kill recovery | `script/test/chaos_saga_kill.sh` (NEW) | Background order load 90s; at t+10s `docker compose kill payment-service`, sleep 5s, `docker compose start payment-service`; assert: 0 PENDING, 0 DLQ delta, 0 duplicate payments |
| Orchestrator + aggregator | `script/test/phase3_run.sh`, `script/test/aggregate_phase3.py` (NEW) | Same shape as Phases 1–2; appends to `test_result.md` |

---

## Concrete Artifact Specs

### 1. `script/test/chaos_cb_cart.sh` (NEW)

Pre-seed → `docker compose pause product-service` → fire 10 `POST /cart/items` and count 503s → run `cart_get_degraded.js` while paused → write `script/k6/results/chaos_cb_cart.json` with `{post_503_count, post_other_count}`. Always `docker compose unpause` on exit via `trap`.

Pass: `post_503_count >= 6` (5 to open + ≥1 fast-fail).

### 2. `script/k6/cart_get_degraded.js` (NEW)

`constant-arrival-rate 10 r/s × 30s` against `GET /cart` with `Authorization: Bearer ${TOKEN}`. Thresholds: `http_req_duration p(95)<20`, `checks rate>0.99`.

### 3. `script/test/chaos_order_race.sh` (NEW)

Per round (×20):
1. Login as customer; create order via `POST /orders` (PENDING).
2. Simultaneously: publish `payments.completed` Kafka event for `orderId` **and** `PUT /orders/:id/ship` direct to `localhost:8082`.
3. Capture HTTP status from the SHIP call (200, 409, or 500).

After 20 rounds: grep `docker compose logs --since N order-service` for:
- `DeadlockLoserDataAccessException` (expected: 0)
- `"status":500` (expected: 0)

Write `script/k6/results/chaos_order_race.json`:
```json
{"deadlock_errors": 0, "http_500": 0, "ship_200": N, "ship_409": M}
```

Note: `payments.completed` Kafka payload must match what `PaymentEventConsumer` deserializes. Look up `order-service/.../kafka/event/PaymentCompletedEvent.java` to derive the JSON shape. Use a `jq -n` template in the script.

### 4. `script/k6/rate_limit_api.js` (NEW)

```javascript
export const options = {
  scenarios: { burst: { executor: 'shared-iterations', vus: 50, iterations: 50, maxDuration: '5s' } },
};
export default function () {
  http.get(`${BASE}/api/v1/products?size=5`,
    { responseCallback: http.expectedStatuses(200, 429) });
}
export function handleSummary(data) {
  return { 'script/k6/results/rate_limit_api.json': JSON.stringify(data) };
}
```

Aggregator counts status=429 via response checks; asserts ≥35.

### 5. `script/k6/rate_limit_auth.js` (NEW)

Same pattern: 9 sequential `POST /api/v1/auth/login` from one source IP via nginx. Aggregator asserts ≥1 of 9 is 429.

### 6. `script/test/chaos_saga_kill.sh` (NEW)

Background `k6 run script/k6/order_create.js -e RATE=1 -e DURATION=90s` (50 orders/min). At `t+10s`: `docker compose kill payment-service`; `sleep 5`; `docker compose start payment-service`. Wait `k6` to finish + 30s settle.

Then assert via SQL + Kafka offset queries:
- `SELECT count(*) FROM orders WHERE status='PENDING' AND created_at > now() - interval '5 minutes'` → 0
- `payments.dlq` delta vs baseline → 0
- `SELECT count(*) FROM (SELECT order_id FROM payments GROUP BY order_id HAVING count(*)>1)` → 0

Write `script/k6/results/chaos_saga_kill.json`:
```json
{"pending_count": 0, "dlq_delta": 0, "duplicate_payment_count": 0}
```

### 7. `script/test/phase3_run.sh` (NEW)

Bash 3.2-compatible. Flow:

```
0. health gate (direct-port checks)
1. chaos_cb_cart.sh                 # §7.A — exits with stack restored (unpause)
2. rate_limit_api.js                # §7.D
3. rate_limit_auth.js               # §7.D
4. chaos_order_race.sh              # §3.C
5. chaos_saga_kill.sh               # mid-saga recovery
6. python3 aggregate_phase3.py      # append to test_result.md
7. summary + exit code
```

Each step traps to restore stack on any exit path.

### 8. `script/test/aggregate_phase3.py` (NEW)

Mirror `aggregate_phase2.py`. New helpers:
- `count_status(summary, code)` — counts 429s from k6 response checks
- `read_json(path)` — loads chaos script outputs

Rows:

| Target ID | Source | Pass criterion |
|---|---|---|
| §7.A-CB    | `chaos_cb_cart.json` | `post_503_count >= 6` |
| §7.A-Deg   | `cart_get_degraded.json` (k6 summary) | `p(95)<20`, `checks rate>0.99` |
| §3.C       | `chaos_order_race.json` | `deadlock_errors==0 AND http_500==0` |
| §7.D-API   | `rate_limit_api.json` | `count(status=429) >= 35` |
| §7.D-Auth  | `rate_limit_auth.json` | `count(status=429) >= 1` |
| Mid-saga   | `chaos_saga_kill.json` | `pending==0 AND dlq_delta==0 AND duplicates==0` |

---

## Critical Files (read-only references)

- `cart-service/internal/client/circuit_breaker.go` (threshold, timeout, state)
- `cart-service/internal/handler/cart_handler.go` (503 mapping)
- `cart-service/internal/repository/redis_cart_repository.go` (Redis-only `GET /cart`)
- `order-service/.../service/OrderStateMachine.java` (transitions)
- `order-service/.../kafka/PaymentEventConsumer.java` (consumer mapping)
- `order-service/.../kafka/event/PaymentCompletedEvent.java` (Kafka payload shape)
- `order-service/.../controller/OrderController.java:56` (`PUT /orders/:id/ship`)
- `payment-service/internal/kafka/consumer.go` (commit strategy)
- `nginx/nginx.conf` (zones)
- `script/k6/order_create.js` (Phase 2 — reused for kill-recovery background load)
- `script/loadtest-orders.sh` (SQL/DLQ assertion patterns)

---

## Verification

```bash
docker compose up -d                              # full stack
bash script/test/phase3_run.sh                    # ≈ 5–8 min full run
cat docs/testing/test_result.md                   # Phase 3 section appended
ls script/k6/results/ | grep -E 'chaos|rate_limit|degraded'
```

Per-target independent runs:

| Target | Command |
|---|---|
| §7.A CB OPEN | `bash script/test/chaos_cb_cart.sh` — expect `post_503_count ≥ 6` |
| §7.A degraded GET /cart | `TOKEN=... k6 run script/k6/cart_get_degraded.js` (while paused) |
| §3.C deadlock | `bash script/test/chaos_order_race.sh` — expect 0 deadlocks, 0 500s |
| §7.D API | `k6 run script/k6/rate_limit_api.js` — expect ≥35/50 are 429 |
| §7.D Auth | `k6 run script/k6/rate_limit_auth.js` — expect ≥1/9 is 429 |
| Mid-saga kill | `bash script/test/chaos_saga_kill.sh` — expect 0/0/0 |

**Acceptance:** Phase 3 row appears in `test_result.md` with PASS/FAIL per target. Any FAIL drops into `improvement performance plan.md`.

**Expected findings to surface:**
- §7.A-CB will PASS (unit test already proves it; live test is verification).
- §7.A-Degraded **may FAIL on 20ms** because the cart-service HTTP client to product-service can still consume goroutines/connections during its 5s timeout window while paused. If it fails → IMP-XX = isolate cart's read pool from its outbound HTTP client pool.
- §3.C will PASS (pessimistic lock serializes), but a SHIP attempt arriving before CONFIRMED returns 409, not a 200 — that's correct behavior.
- §7.D will surface the **config-vs-spec mismatch** (burst=5/3 not 0/0) — record as `IMP-RateLimitSpec`.
- Mid-saga kill — Phase 1 already proved idempotency works deterministically; this is the live-stack confirmation.

---

## Out of Scope

- Adding log lines to `circuit_breaker.go` (production change). 503 response is enough.
- Tightening nginx burst values (covered by the IMP entry).
- Adding `@Retryable` to `findByIdWithLock` (lock serialization is correct already).
- Network-level chaos (latency injection / packet loss) — would need pumba; defer.
- Per-PR CI integration.
