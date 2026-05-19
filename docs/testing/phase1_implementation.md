# Phase 1 — Functional & Saga Correctness: Implementation Plan

> Companion to [`testing_plan.md`](./testing_plan.md) §1. Defines the concrete artifacts that will produce Phase 1 evidence in [`test_result.md`](./test_result.md).

## Context

Phase 1 must pass before Phase 2 (load) and Phase 3 (chaos) run. Today the repo has partial coverage of these targets: `script/e2e-test.sh` covers a single happy-path browse→cart→order flow, `script/e2e-payment.sh` covers a happy-path saga, `InventoryConcurrencyTest.java` covers optimistic-lock retries at the unit level, and `payment_idempotency_test.go` covers DB-level duplicate-key handling.

**Coverage gaps this plan closes:**
- No test today measures **Time-to-Consistency (TTC)** as a percentile.
- No test forces a payment failure deterministically (decline is RNG-based at 10%).
- No test reproduces the 10-buyer / 1-stock race at the HTTP boundary.
- No test replays an `orders.created` Kafka event to verify saga idempotency end-to-end.
- No test verifies `X-Correlation-ID` propagation across all five services.

**Decisions:**
- Payment decline is controlled by `GATEWAY_SUCCESS_RATE` (default 0.9). The fail/happy scripts use a **docker-compose override** that restarts only `payment-service` with the rate set to 1.0 (happy) or 0.0 (fail), then cleans up.
- Race-condition test seeds its product via the **seller API in k6 `setup()`** — self-contained, no SQL.

---

## Targets → Artifacts

| Phase 1 target (testing_plan.md §1) | New artifact | Verification mechanism |
|---|---|---|
| Happy path TTC P95 < 2.0s | `script/k6/saga_happy.js` | Create order, poll `/api/v1/payments/order/:id` every 50ms, record elapsed in `Trend("saga_ttc_ms")` |
| Payment failure TTC P95 < 1.5s | `script/k6/saga_fail.js` | Same flow with `GATEWAY_SUCCESS_RATE=0.0`; assert terminal=FAILED |
| Compensation TTC P95 < 2.0s | `script/k6/saga_fail.js` (second Trend metric) | After FAILED, GET `/api/v1/inventory/{productId}` and record `compensation_ttc_ms` once stock restored |
| Race condition: 10 → 1 success | `script/k6/race_inventory.js` | Seller creates `quantity=1` product; 10 VUs `shared-iterations` startTime=0; assert 1× 201 + 9× 409 + final stock=0 |
| Saga idempotency (1 payment per 3 replays) | `payment-service/internal/integration/saga_replay_test.go` | Testcontainers Kafka+PG; publish same `OrderCreatedEvent` 3× with same orderId; assert exactly 1 `payments` row + `ErrDuplicateIdempotencyKey` returned on retries |
| Correlation-ID 100% propagation | `script/test_correlation_id.sh` | Inject `X-Correlation-ID: <uuid>` at nginx; exercise login→cart→order saga; `docker compose logs` grep the uuid across all 5 services |

Plus one runner that aggregates results:

| Aggregation | New artifact |
|---|---|
| Run all of Phase 1 sequentially, write a markdown table to `docs/testing/test_result.md` | `script/test/phase1_run.sh` |

---

## Artifact Specs

### 1. `script/k6/saga_happy.js` (NEW)

**Pre-run hook (handled by `phase1_run.sh`):** apply `docker-compose.phase1-happy.override.yml` setting `payment-service.environment.GATEWAY_SUCCESS_RATE=1.0`, then `docker compose up -d payment-service` to restart that service only.

**k6 script structure:**

```javascript
// setup(): login as customer@example.com / Customer@123 → { token, userId }
// default(): per VU iteration
//   t0 = Date.now()
//   POST /api/v1/orders  with X-User-Id; capture orderId from .data.id
//   loop: GET /api/v1/payments/order/{orderId} every 50ms until .data.status === 'COMPLETED'
//   ttcMs = Date.now() - t0
//   trend.add(ttcMs); check(status === 'COMPLETED')
// thresholds:
//   saga_ttc_ms: ['p(95)<2000']
//   checks: ['rate==1.0']
// scenarios: { happy: { executor: 'shared-iterations', vus: 10, iterations: 200, maxDuration: '5m' } }
```

Reuse pattern from `script/k6/order_create.js` (login, X-User-Id header, payload shape). Output: `--summary-export=script/k6/results/saga_happy.json`.

### 2. `script/k6/saga_fail.js` (NEW)

**Pre-run hook:** apply `docker-compose.phase1-fail.override.yml` (`GATEWAY_SUCCESS_RATE=0.0`), restart payment-service.

Mirrors `saga_happy.js` with three changes:
- Terminal status: `'FAILED'`.
- `saga_ttc_ms p(95)<1500`.
- **Compensation check appended to the same VU iteration:** after detecting FAILED, capture the productIds + quantities ordered, query `GET /api/v1/inventory/{productId}` for each, record `compensation_ttc_ms = Date.now() - t0` once all reservations are released. Second Trend `compensation_ttc_ms: ['p(95)<2000']`.

This collapses two target rows into one script.

### 3. `script/k6/race_inventory.js` (NEW)

**setup():**
- Login as `seller@example.com` / `Seller@123`.
- `POST /api/v1/products` with `X-Seller-Id` header: `{ name: "race-test-<uuid>", price: 9.99, quantity: 1, categoryId: 1, ... }`. Capture productId.
- Login as `customer@example.com`; capture customerToken + customerId.
- Return `{ productId, customerToken, customerId, sellerToken }`.

**default():** `shared-iterations` over 10 VUs × 10 iterations with `startTime: 0` to maximize collision pressure.
- `POST /api/v1/orders` with `items: [{ productId, quantity: 1 }]`.
- Tag response: `Counter('order_success')` (201), `Counter('order_conflict')` (409), `Counter('order_other')` else.

**teardown():**
- `GET /api/v1/inventory/{productId}` → assert `data.quantity === 0`.
- Cleanup: `DELETE /api/v1/products/{productId}` (seller).

**thresholds:** `order_success: count==1`, `order_conflict: count==9`, `checks: rate==1.0`.

### 4. `payment-service/internal/integration/saga_replay_test.go` (NEW)

Mirror the scaffolding in `payment-service/internal/integration/payment_kafka_test.go` — same testcontainers Kafka + Postgres setup, same `ensureTopics`, `publishMsg`, `startConsumer` helpers.

```go
//go:build integration

func TestSagaReplay_SameOrderId_OnlyOnePayment(t *testing.T) {
    ctx := context.Background()
    bootstrap := startKafkaBroker(t, ctx)
    db := startPostgres(t, ctx)
    repo := repository.NewPaymentRepository(db)
    startConsumer(t, ctx, bootstrap, repo /* + gateway with success=1 */)

    orderId := uuid.New()
    evt := event.OrderCreatedEvent{
        OrderID: orderId, UserID: uuid.New(),
        TotalAmount: decimal.NewFromFloat(50.00),
        Items: []event.OrderItemEvent{{ProductID: 1, Quantity: 1, Price: decimal.NewFromFloat(50.00)}},
    }
    for i := 0; i < 3; i++ {
        require.NoError(t, publishMsg(ctx, bootstrap, "orders.created", evt))
    }
    require.Eventually(t, func() bool {
        p, err := repo.GetByOrderID(ctx, orderId); return err == nil && p != nil
    }, 5*time.Second, 50*time.Millisecond)
    time.Sleep(500 * time.Millisecond)

    var count int64
    require.NoError(t, db.Model(&model.Payment{}).Where("order_id = ?", orderId).Count(&count).Error)
    require.Equal(t, int64(1), count, "exactly one payment row per orderId")
}
```

Anchors:
- Idempotency key: `evt.OrderID.String()` (`payment-service/internal/kafka/consumer.go:176`).
- Duplicate sentinel: `repository.ErrDuplicateIdempotencyKey` (`payment-service/internal/repository/payment_repository.go:15`).

### 5. `script/test_correlation_id.sh` (NEW)

```bash
#!/usr/bin/env bash
set -euo pipefail
CID=$(uuidgen)
SINCE_TS=$(date -u +%s)

curl -s -X POST http://localhost/api/v1/auth/login \
  -H "X-Correlation-ID: $CID" -H 'Content-Type: application/json' \
  -d '{"email":"customer@example.com","password":"Customer@123"}' > /tmp/login.json
TOKEN=$(jq -r .data.access_token /tmp/login.json)
USER_ID=$(jq -r .data.user.id /tmp/login.json)

curl -s -X POST http://localhost/api/v1/cart/items \
  -H "X-Correlation-ID: $CID" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"productId":1,"quantity":1}' > /dev/null

curl -s -X POST http://localhost/api/v1/orders \
  -H "X-Correlation-ID: $CID" -H "X-User-Id: $USER_ID" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg c "$(uuidgen)" '{cartId:$c,items:[{productId:1,quantity:1}],shippingAddress:{street:"1 St",city:"HCM",state:"HCM",country:"VN",zipCode:"700"}}')" \
  > /dev/null

sleep 3
FAIL=0
for svc in user-service cart-service product-service order-service payment-service; do
  if docker compose logs --since "${SINCE_TS}s" "$svc" 2>/dev/null | grep -q "$CID"; then
    echo "  ✓ $svc"
  else
    echo "  ✗ $svc — correlation id not found"; FAIL=1
  fi
done
exit $FAIL
```

Notes: Java services log via MDC (`%X{correlationId}`). Go services pass through context — if any doesn't write the field to its structured logger, the test surfaces that gap.

### 6. `script/test/phase1_run.sh` (NEW) — orchestrator

```bash
#!/usr/bin/env bash
set -euo pipefail
mkdir -p script/k6/results
bash script/health-dashboard.sh

apply_override docker-compose.phase1-happy.override.yml
k6 run --summary-export=script/k6/results/saga_happy.json script/k6/saga_happy.js

apply_override docker-compose.phase1-fail.override.yml
k6 run --summary-export=script/k6/results/saga_fail.json   script/k6/saga_fail.js

revert_override
k6 run --summary-export=script/k6/results/race_inventory.json script/k6/race_inventory.js
(cd payment-service && go test -tags=integration -v -race -run TestSagaReplay ./internal/integration/...)
bash script/test_correlation_id.sh

python3 script/test/aggregate_phase1.py    # writes Phase 1 section into docs/testing/test_result.md
```

Override file `docker-compose.phase1-happy.override.yml`:
```yaml
services:
  payment-service:
    environment:
      GATEWAY_SUCCESS_RATE: "1.0"
```
(`fail` override sets it to `"0.0"`.) `apply_override` cp's the file to `docker-compose.override.yml` and runs `docker compose up -d payment-service`. `revert_override` removes it and restarts.

**Output schema** (appended to `docs/testing/test_result.md`):

```markdown
## Phase 1 — Functional & Saga Correctness  (run YYYY-MM-DD HH:MM)

| Target ID | Target | Observed | Status | Evidence |
|---|---|---|---|---|
| §2-Happy | Saga TTC P95 < 2.0s | <p95>ms | PASS/FAIL | script/k6/results/saga_happy.json |
| §2-Fail  | Fail TTC P95 < 1.5s | …       | …       | script/k6/results/saga_fail.json |
| §2-Comp  | Compensation TTC P95 < 2.0s | … | …    | script/k6/results/saga_fail.json |
| §3.B     | Race: 1× 201, 9× 409, stock=0 | … | …    | script/k6/results/race_inventory.json |
| §7.B     | Saga replay: 1 payment row | … | …       | go test output |
| §7.C     | Correlation ID in 5 services | … | …      | docker compose logs |
```

---

## Critical Files (read-only references)

**API contracts (consume, do not change):**
- `order-service/.../controller/OrderController.java` — POST `/api/v1/orders`, `X-User-Id` header, response `{ data: { id, status } }`.
- `payment-service/internal/handler/payment_handler.go` — GET `/api/v1/payments/order/:orderId`, terminal `COMPLETED`/`FAILED`.
- `product-service/.../controller/InventoryController.java` — GET `/api/v1/inventory/{productId}`.
- `user-service/internal/handler/auth_handler.go` — login response `.data.access_token`, `.data.user.id`.
- `script/sample_users.sql` — `customer@example.com:Customer@123`, `seller@example.com:Seller@123`.

**Decline mechanism (drives docker overrides):**
- `payment-service/internal/gateway/mock_gateway.go` lines 38–54.
- `payment-service/config/config.go` line 44 — `GATEWAY_SUCCESS_RATE`.

**Saga replay scaffolding to copy:**
- `payment-service/internal/integration/payment_kafka_test.go`.
- `payment-service/internal/kafka/event/events.go` lines 18–23.
- `payment-service/internal/kafka/consumer.go:176`.
- `payment-service/internal/repository/payment_repository.go:15`.

**Correlation-ID chain:**
- `nginx/nginx.conf` lines 13–22 + per-upstream `proxy_set_header X-Correlation-ID $correlation_id;`.
- `cart-service/internal/middleware/correlation.go`, `payment-service/internal/middleware/correlation.go`.
- `product-service/.../filter/CorrelationFilter.java`, `order-service/.../filter/CorrelationFilter.java`.
- `payment-service/internal/kafka/{producer.go,consumer.go}`.

**Patterns to mirror:**
- `script/e2e-payment.sh`, `script/k6/order_create.js`, `script/health-dashboard.sh`.

---

## Verification

```bash
docker compose down -v && docker compose up --build -d
bash script/test/phase1_run.sh
cat docs/testing/test_result.md
ls script/k6/results/
```

Per-target independent runs:

| Target | Command |
|---|---|
| Happy TTC | apply happy override, then `k6 run script/k6/saga_happy.js` — expect `saga_ttc_ms p(95)` < 2000 |
| Fail + compensation TTC | apply fail override, then `k6 run script/k6/saga_fail.js` — expect saga_ttc_ms p95<1500 and compensation_ttc_ms p95<2000 |
| Race | `k6 run script/k6/race_inventory.js` — expect `order_success count=1`, `order_conflict count=9` |
| Saga replay | `cd payment-service && go test -tags=integration -v -race -run TestSagaReplay ./internal/integration/...` |
| Correlation ID | `bash script/test_correlation_id.sh` — expect 5 ✓ |

Failure modes the plan surfaces (rather than patches):
- A Go service missing `correlationID` in its structured logs → correlation-ID test fails for that service → improvement-plan entry.
- Spring-Kafka deserialization issues when Go publishes without `__TypeId__` → saga_replay surfaces it.
- Non-409 codes in race responses (e.g., 503 from optimistic-lock exhaustion) → record exact code distribution in the result row.

---

## Out of Scope

- Adding Micrometer timers inside services (Phase 2).
- Writing `improvement performance plan.md` entries (post-run triage step).
- CI integration — local-runner only for now.
