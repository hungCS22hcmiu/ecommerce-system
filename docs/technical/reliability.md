# Reliability Validation

## Headline Result

**100 concurrent orders submitted. Zero PENDING transactions. Zero dead-letter messages. Every saga path reached a terminal state.**

This was not asserted by inspecting logs or trusting happy-path smoke tests. It was asserted automatically by querying the database and Kafka after the load completed — two independent checks that would catch any gap: a stuck transaction left in the database, or a dropped event left unprocessed in Kafka.

---

## What Was Tested

The end-to-end saga path for the order-payment flow spans five services and two transports:

```
Client
  └─► Order Service       (creates order + outbox row, atomically)
        └─► Kafka          (OutboxPublisher publishes orders.created)
              └─► Payment Service  (consumes, charges mock gateway, publishes result)
                    └─► Kafka      (payments.completed or payments.failed)
                          └─► Order Service  (transitions order to CONFIRMED or CANCELLED)
```

A reliability failure in this chain can manifest as:
- A **PENDING order** — order created, Kafka event never consumed or payment never published back
- A **PENDING payment** — Kafka event consumed but result never committed to DB
- A **DLQ message** — event could not be processed after retries, routed to dead-letter queue
- A **duplicate payment row** — idempotency constraint failed, same order charged twice

All four were asserted to be zero.

---

## Load Test: 100 Concurrent Orders

**Script:** `script/loadtest-orders.sh`

```
ORDER_COUNT=100   CONCURRENCY=10   WAIT_SECONDS=30
```

The script:
1. Logs in as `customer@example.com` and locates a seeded product with available stock
2. Spawns **10 parallel workers** via `xargs -P 10`, each submitting `POST /api/v1/orders` until 100 orders are created
3. Waits 30 seconds for Kafka choreography to propagate
4. Queries PostgreSQL directly for terminal state counts
5. Reads the `payments.dlq` topic to confirm zero dead-letter messages

### SQL Assertions (Step 5)

```sql
-- ecommerce_payments
SELECT
  SUM(CASE WHEN status='COMPLETED' THEN 1 ELSE 0 END),
  SUM(CASE WHEN status='FAILED'    THEN 1 ELSE 0 END),
  SUM(CASE WHEN status='PENDING'   THEN 1 ELSE 0 END),
  COUNT(*)
FROM payments;

-- ecommerce_orders
SELECT
  SUM(CASE WHEN status='CONFIRMED'  THEN 1 ELSE 0 END),
  SUM(CASE WHEN status='CANCELLED'  THEN 1 ELSE 0 END),
  SUM(CASE WHEN status='PENDING'    THEN 1 ELSE 0 END)
FROM orders;
```

**Expected and observed:** `PENDING = 0` for both tables.

### Kafka DLQ Assertion (Step 6)

```bash
docker exec ecommerce-kafka kafka-console-consumer \
  --bootstrap-server localhost:29092 --topic payments.dlq \
  --from-beginning --timeout-ms 3000 --max-messages 1
```

**Expected and observed:** empty — no output, exit clean.

### Pass Condition

```
✓ No PENDING payments (all saga paths terminated)
✓ No PENDING orders (all orders reached terminal state)
✓ payments.dlq is empty
```

---

## Chaos Test: Payment Service Killed Mid-Saga

The load test above runs against a healthy stack. A harder test is: what happens if the payment service is killed while orders are in-flight?

**Script:** `script/test/chaos_saga_kill.sh`

```
DURATION=90s   KILL_AT=10s   RESTART_AFTER=5s   RATE=10 orders/s   SETTLE_AFTER=30s
```

The script:
1. Snapshots the baseline `payments.dlq` offset
2. Starts 10 orders/s background load via k6
3. At t+10s, runs `docker compose kill payment-service` — killing it mid-saga while orders are in-flight
4. Waits 5 seconds (orders piling up in Kafka, unprocessed)
5. Runs `docker compose start payment-service` — service restarts and resumes consuming from last committed offset
6. Waits for k6 to finish + 30s settle window
7. Queries PostgreSQL for PENDING and duplicate rows; diffs the DLQ offset

### Observed Result (Phase 3, 2026-05-21)

| Assertion | Target | Observed |
|---|---|---|
| PENDING orders | 0 | **0** |
| DLQ delta (new messages) | 0 | **0** |
| Duplicate payment rows | 0 | **0** |
| Payments processed in window | — | **900** |

### Why This Works: The Three-Layer Guarantee

**Layer 1 — Transactional Outbox (Order Service)**
Order creation and the `orders_outbox` row are written in a single database transaction. If the transaction commits, the event is guaranteed to be published by the `OutboxPublisher` polling loop — there is no dual-write window where the order exists but the Kafka event does not.

**Layer 2 — Kafka Offset Not Committed on PENDING (Payment Service)**
When payment-service is killed mid-gateway-call, the Kafka offset for that message was never committed (manual commit mode, `CommitInterval: 0`). On restart, the consumer picks up from the last committed offset — the same message is re-delivered. The `PENDING-resume` path in `ProcessPayment` detects the existing PENDING row and retries the gateway call using the same payment ID rather than creating a new one.

**Layer 3 — DB UNIQUE Constraint (Payment Service)**
Even if the resume path were to attempt a new insert, the `UNIQUE` constraint on `payments.idempotency_key` prevents any second payment row from being created for the same order. The constraint is the final backstop — `ErrDuplicateIdempotencyKey` is the signal that triggers the resume path, not the other way around.

---

## Saga Latency Under Load

Beyond correctness, the saga must complete in a reasonable time window so that users are not left waiting.

**Test:** `script/k6/saga_happy.js` + `script/k6/saga_fail.js`
**Configuration:** 10 VUs × 200 iterations each, measuring time-to-consistency (TTC) from `POST /orders` to terminal payment status.

| Scenario | Metric | Target | Observed |
|---|---|---|---|
| Happy path (payment succeeds) | TTC P95 | < 2,000ms | **822ms** |
| Happy path | TTC avg | — | **411ms** |
| Failure path (payment declined) | TTC P95 | < 1,500ms | **442ms** |
| Compensation (order CANCELLED) | Compensation TTC P95 | < 2,000ms | **741ms** |

All three latency targets passed with significant headroom. The saga completes in under 1 second at median for both happy and failure paths.

---

## Composite Checkout Under Sustained Load (50 VUs)

**Test:** `script/k6/checkout_50vu.js`
**Configuration:** 50 virtual users sustained for 3 minutes, each running the full chain: product search → add-to-cart → create order.

| Metric | Target | Observed |
|---|---|---|
| Full-chain P95 | < 1,000ms | **633ms** |
| Error rate | < 0.1% | **0.00%** |
| Check pass rate | > 99.9% | **100%** |

Zero errors across the entire 3-minute run at 50 concurrent users.

---

## Inventory Race Condition

**Test:** `script/k6/race_inventory.js`

10 concurrent requests attempt to reserve stock for the same product with only 1 unit available.

| Metric | Expected | Observed |
|---|---|---|
| Successful orders | exactly 1 | **1** |
| Conflict (409) responses | exactly 9 | **9** |
| Unexpected errors | 0 | **0** |
| Final DB stock | 0 | **0** |

The conditional native `UPDATE WHERE stock_available >= qty` in product-service guarantees exactly one reservation wins. No optimistic-lock retries, no overselling.

---

## Idempotency: Duplicate Kafka Delivery

**Test:** Integration test `TestDuplicateDeliveryIdempotency`

The same `orders.created` event was delivered **3 times** to the payment service (simulating Kafka at-least-once redelivery).

| Assertion | Expected | Observed |
|---|---|---|
| Payment rows created | 1 | **1** |
| Duplicate rows | 0 | **0** |
| Final payment status | terminal | **COMPLETED** |

The `UNIQUE(idempotency_key)` DB constraint and the PENDING-resume path together make repeated delivery a no-op after the first successful processing.

---

## Resilience: Circuit Breaker + Degraded Mode

**Test:** `script/test/chaos_cb_cart.sh`

With product-service paused (simulating a downstream outage), cart-service was subjected to 10 consecutive `POST /cart/items` requests.

| Metric | Expected | Observed |
|---|---|---|
| Requests before CB opens (non-503) | 5 | **5** |
| Requests after CB opens (503) | 5 | **5** |
| CB status sequence | `000 000 000 000 000 503 503 503 503 503` | **exact match** |

After the circuit opened, a separate k6 run measured read performance against the degraded stack:

| Metric | Target | Observed |
|---|---|---|
| `GET /cart` P95 (product-service down) | < 20ms | **4ms** |

`GET /cart`, `UpdateItem`, `RemoveItem`, and `ClearCart` bypass the product client entirely and serve from Redis — they continue to function at full speed even when the downstream service is unavailable.

---

## Correlation ID Propagation

**Test:** `script/test_correlation_id.sh`

An `X-Correlation-ID` was injected at the Nginx boundary and traced across all 5 service logs.

| Service | Header present in logs |
|---|---|
| user-service | ✓ |
| product-service | ✓ |
| cart-service | ✓ |
| order-service | ✓ |
| payment-service | ✓ |

Every service in the request path logged the same correlation ID, confirming end-to-end distributed traceability.

---

## Test Infrastructure

All tests are automated shell scripts driven by k6 (load) and psql / Kafka CLI (assertions). No manual inspection required.

| Script | What it validates |
|---|---|
| `script/loadtest-orders.sh` | 100 concurrent orders → 0 PENDING, 0 DLQ (the headline test) |
| `script/e2e-payment.sh` | Full saga end-to-end: login → order → payment → order confirmation (12 assertions) |
| `script/test/chaos_saga_kill.sh` | Mid-saga payment-service kill → 0 PENDING, 0 DLQ after restart |
| `script/test/chaos_cb_cart.sh` | Circuit breaker opens after 5 failures; degraded GET /cart ≤ 4ms P95 |
| `script/test/chaos_order_race.sh` | Concurrent Kafka + HTTP order transitions → 0 deadlocks, 0 5xx |
| `script/k6/saga_happy.js` | TTC P95 < 2,000ms (200 iterations, 10 VUs, 100% success rate) |
| `script/k6/saga_fail.js` | TTC P95 < 1,500ms; compensation TTC P95 < 2,000ms |
| `script/k6/checkout_50vu.js` | 50 VU full-chain checkout — error rate 0.00%, chain P95 633ms |
| `script/k6/race_inventory.js` | 10 concurrent reservations, 1 unit stock → exactly 1 success, 9 × 409 |
| Integration test `TestDuplicateDeliveryIdempotency` | 3× Kafka redelivery → 1 payment row |

Re-run the headline test at any time:

```bash
bash script/loadtest-orders.sh
# Expected output:
# ✓ No PENDING payments (all saga paths terminated)
# ✓ No PENDING orders (all orders reached terminal state)
# ✓ payments.dlq is empty
# Load test passed: 100 orders, 0 PENDING, 0 DLQ messages.
```
