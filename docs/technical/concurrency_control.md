# Concurrency Control

## Overview

Four distinct concurrency-control strategies are applied across the platform. Each was chosen for the specific threat model of its service — there is no single strategy applied uniformly.

| Service | Strategy | Threat | Why this strategy |
|---|---|---|---|
| Order Service | Pessimistic `SELECT FOR UPDATE` | Two threads transitioning the same order simultaneously | Correctness is catastrophic if two state transitions both commit; lock duration is sub-ms |
| Product Service | Optimistic conditional `UPDATE … WHERE` | Concurrent stock reservations overselling inventory | Low contention on most products; single atomic SQL statement; no retry loops |
| Cart Service | Redis `WATCH / MULTI / EXEC` | Two tabs writing to the same cart simultaneously | Primary store is Redis; optimistic is correct for low-contention per-user writes |
| Payment Service | Idempotency key + DB `UNIQUE` constraint | Kafka delivering the same `orders.created` event twice | Duplicate event delivery is the primary threat; constraint is the lightest correct solution |

---

## 1. Pessimistic `SELECT FOR UPDATE` — Order Service

### The problem

Order state transitions follow a strict state machine (PENDING → CONFIRMED → SHIPPED → DELIVERED; PENDING/CONFIRMED → CANCELLED). Two concurrent requests targeting the same order — a Kafka payment event and a customer cancellation request, for example — can both read the same current status, both validate the transition as legal, and both commit, leaving the order in an undefined state.

### Implementation

`order-service/src/main/java/com/ecommerce/order_service/repository/OrderRepository.java`
```java
@Query("SELECT o FROM Order o WHERE o.id = :id")
@Lock(LockModeType.PESSIMISTIC_WRITE)
Optional<Order> findByIdWithLock(@Param("id") UUID id);
```

Spring Data JPA translates `PESSIMISTIC_WRITE` to `SELECT … FOR UPDATE`. Postgres blocks the second transaction at the row level until the first commits or rolls back. When the second transaction acquires the lock, it re-reads the current status — by which point the first transaction has already changed it — and the state machine rejects the now-illegal transition.

`order-service/src/main/java/com/ecommerce/order_service/service/impl/OrderServiceImpl.java` — `updateOrderStatus()`
```java
@Transactional
public OrderResponse updateOrderStatus(UUID orderId, OrderStatus newStatus,
                                       String reason, String changedBy) {
    // Pessimistic lock: only one transaction wins concurrent transitions
    Order order = orderRepository.findByIdWithLock(orderId)
            .orElseThrow(() -> new OrderNotFoundException(orderId));

    OrderStatus oldStatus = order.getStatus();
    stateMachine.validateTransition(oldStatus, newStatus);   // throws if illegal

    order.setStatus(newStatus);
    orderRepository.save(order);

    historyRepository.save(OrderStatusHistory.builder()
            .orderId(orderId)
            .oldStatus(oldStatus)
            .newStatus(newStatus)
            .reason(reason)
            .changedBy(changedBy)
            .build());

    return OrderResponse.from(order);
}
```

The lock is held for the entire transaction — read → validate → write → history insert. This is intentionally short (sub-millisecond at the DB layer) because no external calls happen inside the lock boundary.

### Test

`order-service/src/test/java/com/ecommerce/order_service/integration/OrderConcurrencyTest.java`
```java
/**
 * Two threads race to transition the same CONFIRMED order.
 * Thread A: CONFIRMED → SHIPPED
 * Thread B: CONFIRMED → CANCELLED
 * Both fire simultaneously via CountDownLatch.
 *
 * Expected: exactly one succeeds; the other throws InvalidOrderStateException
 * because the winner's commit changes the status before the loser reads it.
 */
@Test
void concurrent_stateTransitions_exactlyOneWins() throws Exception {
    Order order = createConfirmedOrder();
    CountDownLatch startGate = new CountDownLatch(1);

    Callable<String> shipTask = () -> {
        startGate.await();
        try {
            orderService.updateOrderStatus(order.getId(), OrderStatus.SHIPPED,
                    "Shipped", "thread-ship");
            return "success";
        } catch (InvalidOrderStateException e) { return "failed"; }
    };

    Callable<String> cancelTask = () -> {
        startGate.await();
        try {
            orderService.updateOrderStatus(order.getId(), OrderStatus.CANCELLED,
                    "Cancelled", "thread-cancel");
            return "success";
        } catch (InvalidOrderStateException e) { return "failed"; }
    };

    ExecutorService executor = Executors.newFixedThreadPool(2);
    Future<String> f1 = executor.submit(shipTask);
    Future<String> f2 = executor.submit(cancelTask);
    startGate.countDown();   // release both threads simultaneously

    long successCount = Stream.of(f1.get(10, SECONDS), f2.get(10, SECONDS))
                              .filter("success"::equals).count();

    assertThat(successCount).isEqualTo(1);   // exactly one winner
}
```

The test is run 5 times in a loop (`repeated_concurrent_transitions_alwaysExactlyOneWins`) to rule out lucky serialization. Uses Testcontainers (real Postgres) and EmbeddedKafka. Run with `./mvnw test -pl order-service`.

### What happens without the lock

Without `SELECT FOR UPDATE`, both threads can read `status=CONFIRMED`, both validate `CONFIRMED → X` as legal, and both commit. The order ends up in the state written by whichever transaction commits last — neither thread's view of the state is consistent.

---

## 2. Optimistic Conditional `UPDATE … WHERE` — Product Service

### The problem

Stock reservation requires a check-then-act on `stock_available = stock_quantity - stock_reserved`. A naive read-then-write sequence has a window where two threads both read sufficient stock and both write their reservation, causing overselling.

### Implementation

Rather than using `@Version` optimistic locking (which requires retry loops and fires spurious Hibernate dirty-check updates under concurrent load), product-service issues a single atomic SQL statement whose `WHERE` clause is the guard condition:

`product-service/src/main/java/com/ecommerce/product_service/repository/ProductRepository.java`
```java
// Atomic conditional reserve: increments stock_reserved only when available >= qty.
// Returns 1 on success, 0 on insufficient stock or missing product.
// clearAutomatically = true ensures subsequent findById() reads fresh DB state in the same TX.
@Modifying(clearAutomatically = true)
@Query(value = "UPDATE products SET stock_reserved = stock_reserved + :qty " +
               "WHERE id = :id AND (stock_quantity - stock_reserved) >= :qty",
       nativeQuery = true)
int reserveStockConditional(@Param("id") Long id, @Param("qty") int qty);

// Atomic conditional release: decrements stock_reserved only when reserved >= qty.
@Modifying(clearAutomatically = true)
@Query(value = "UPDATE products SET stock_reserved = stock_reserved - :qty " +
               "WHERE id = :id AND stock_reserved >= :qty",
       nativeQuery = true)
int releaseStockConditional(@Param("id") Long id, @Param("qty") int qty);
```

The `WHERE (stock_quantity - stock_reserved) >= qty` clause is evaluated atomically by Postgres. Only one concurrent `UPDATE` can increment `stock_reserved` when exactly one unit remains — the others see zero rows affected.

`product-service/src/main/java/com/ecommerce/product_service/service/serviceImpl/InventoryServiceImpl.java` — `reserveStock()`
```java
@Transactional
@CacheEvict(value = "product", key = "#productId")
public StockResponse reserveStock(Long productId, int quantity, String referenceId) {
    int updated = productRepository.reserveStockConditional(productId, quantity);

    if (updated == 0) {
        // Zero rows updated — diagnose whether it's out-of-stock or not-found
        Product product = productRepository.findById(productId)
                .orElseThrow(() -> new ProductNotFoundException(productId));
        int available = product.getStockQuantity() - product.getStockReserved();
        if (available < quantity) {
            throw new InsufficientStockException(productId, quantity, available);
        }
        throw new StockContentionException(productId);   // theoretically unreachable
    }

    stockMovementRepository.save(StockMovement.builder()
            .productId(productId).type(MovementType.RESERVE)
            .quantity(quantity).referenceId(referenceId).build());

    // StockProjection avoids loading a full managed entity, which would trigger a
    // spurious Hibernate dirty-check UPDATE (incrementing @Version) and produce
    // false ObjectOptimisticLockingFailureException under concurrent load.
    StockProjection sp = productRepository.findStockById(productId)
            .orElseThrow(() -> new ProductNotFoundException(productId));
    return new StockResponse(productId, sp.getStockQuantity(), sp.getStockReserved(),
            sp.getStockQuantity() - sp.getStockReserved());
}
```

### Test: 10 threads competing for the same stock

`script/k6/race_inventory.js` — k6 load test, Phase 1 §3.B
```js
// 10 VUs try to buy the last unit of a product with stockQuantity=1.
// Expectation: exactly 1 order returns 201 Created,
//              exactly 9 orders return 409 Conflict.
export const options = {
  scenarios: {
    race: {
      executor: 'shared-iterations',
      vus: 10,
      iterations: 10,    // one attempt per VU, all fire simultaneously
      maxDuration: '30s',
    },
  },
  thresholds: {
    order_success:  ['count==1'],   // exactly 1 winner
    order_conflict: ['count==9'],   // exactly 9 losers
    order_other:    ['count==0'],   // no unexpected errors
  },
};

export function setup() {
  // Seller creates a fresh product with stockQuantity=1 (repeatable, self-contained)
  const productPayload = JSON.stringify({
    name: `race-test-${uuidv4()}`,
    price: 9.99, categoryId: 1,
    stockQuantity: 1,             // exactly 1 unit — only 1 thread can win
  });
  // ...
}

export default function (data) {
  const res = http.post(`${ORDER_URL}/api/v1/orders`, payload, {
    responseCallback: http.expectedStatuses(201, 409),
  });

  if (res.status === 201)      orderSuccess.add(1);   // stock was available
  else if (res.status === 409) orderConflict.add(1);  // InsufficientStockException
  else                          orderOther.add(1);
}
```

**Observed result (Phase 1, 2026-05-21):**

| Metric | Expected | Observed |
|---|---|---|
| `order_success` | exactly 1 | **1** |
| `order_conflict` (409) | exactly 9 | **9** |
| `order_other` | 0 | **0** |
| Final DB stock | 0 | **0** |

The same mechanism handles any N-of-M scenario: with `stockQuantity=5` and 10 concurrent threads, the conditional `UPDATE` allows exactly 5 rows to match the `WHERE` clause and returns 0 rows for the remaining 5 — 5 successes, 5 × 409. The number of winners equals the available stock, never more.

---

## 3. Redis `WATCH / MULTI / EXEC` — Cart Service

### The problem

A user with two browser tabs open can submit simultaneous `POST /cart/items` requests. Both tabs read the same cart state from Redis, apply their changes, and write back — the second write silently overwrites the first, losing one item.

### Implementation

`cart-service/internal/repository/redis_cart_repository.go` — `AddOrUpdateItem()`
```go
var ErrConcurrentUpdate = errors.New("concurrent cart update, please retry")

func (r *redisCartRepository) AddOrUpdateItem(
    ctx context.Context, userID uuid.UUID,
    productID int64, val CartItemValue,
) error {
    key := fmt.Sprintf("cart:%s", userID)
    field := strconv.FormatInt(productID, 10)
    jsonVal, _ := json.Marshal(val)

    for retries := 0; retries < 3; retries++ {
        err := r.rdb.Watch(ctx, func(tx *redis.Tx) error {
            _, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
                pipe.HSet(ctx, key, field, jsonVal)
                pipe.Expire(ctx, key, 7*24*time.Hour)
                return nil
            })
            return err
        }, key)   // WATCH key: aborts EXEC if key changes between WATCH and EXEC

        if err == nil {
            return nil
        }
        if !errors.Is(err, redis.TxFailedErr) {
            return err   // non-transactional error — propagate immediately
        }
        // redis.TxFailedErr: key was modified between WATCH and EXEC
        // retry up to 3 times, then return ErrConcurrentUpdate → HTTP 409
    }
    return ErrConcurrentUpdate
}
```

**How `WATCH / MULTI / EXEC` works in Redis:**

```
Client A                          Client B
   │                                 │
   ├── WATCH cart:{userId}           │
   │   (records current version)     │
   │                                 ├── WATCH cart:{userId}
   │                                 │
   ├── MULTI                         ├── MULTI
   ├── HSET cart:... field val       ├── HSET cart:... field val
   ├── EXPIRE cart:... 604800        ├── EXPIRE cart:... 604800
   ├── EXEC ← commits                │
   │   (version changed for B)       │
   │                                 ├── EXEC → nil (aborted — WATCH saw change)
   │                                 │   redis.TxFailedErr → retry
```

`TxPipelined` wraps the commands in `MULTI … EXEC`. If the watched key was modified between `WATCH` and `EXEC`, Redis returns a nil reply (no commands execute) and `go-redis` surfaces this as `redis.TxFailedErr`. The retry loop re-reads and re-applies, up to 3 attempts. After 3 failures, `ErrConcurrentUpdate` is returned to the handler, which responds with HTTP 409 so the client can retry.

Read operations (`GetCart`), removals (`RemoveItem`), and `ClearCart` do not use transactions — `WATCH/MULTI/EXEC` is only needed for the read-modify-write pattern in `AddOrUpdateItem`.

---

## 4. Idempotency Key + DB `UNIQUE` Constraint — Payment Service

### The problem

Kafka guarantees at-least-once delivery. The same `orders.created` event can be delivered multiple times — on restart after a crash, after a network partition, or after a consumer rebalance. Without a guard, each delivery would charge the customer again.

### Schema

`payment-service/migrations/000001_baseline_schema.up.sql`
```sql
CREATE TABLE IF NOT EXISTS payments (
    id                 UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id           UUID           NOT NULL,
    idempotency_key    VARCHAR(255)   NOT NULL,
    -- ...
    CONSTRAINT uq_payments_order_id UNIQUE (order_id)   -- one payment per order
);

-- Unique index: second INSERT with the same idempotency_key hits SQLSTATE 23505
CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_idempotency ON payments(idempotency_key);
```

Two constraints defend against different failure modes:
- `UNIQUE(idempotency_key)` — blocks a second Kafka redelivery of the same event
- `UNIQUE(order_id)` — blocks any other code path from creating a second payment for the same order

### Implementation

`payment-service/internal/repository/payment_repository.go` — `Create()`
```go
// isDuplicateKey returns true when err is a PostgreSQL unique-violation (SQLSTATE 23505).
func isDuplicateKey(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *paymentRepository) Create(ctx context.Context,
    p *model.Payment, h *model.PaymentHistory) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Create(p).Error; err != nil {
            if isDuplicateKey(err) {
                return ErrDuplicateIdempotencyKey   // caller handles this case
            }
            return err
        }
        h.PaymentID = p.ID
        return tx.Create(h).Error
    })
}
```

`payment-service/internal/service/payment_service.go` — `ProcessPayment()`
```go
func (s *paymentService) ProcessPayment(ctx context.Context,
    in ProcessPaymentInput) (*model.Payment, error) {

    p := &model.Payment{ /* ... */ IdempotencyKey: in.IdempotencyKey }
    if err := s.repo.Create(ctx, p, h); err != nil {
        if !errors.Is(err, repository.ErrDuplicateIdempotencyKey) {
            return nil, err
        }
        existing, _ := s.repo.FindByIdempotencyKey(ctx, in.IdempotencyKey)
        if existing.Status != model.PaymentStatusPending {
            return existing, nil   // already COMPLETED or FAILED — idempotent return
        }
        // PENDING: service was killed after DB write but before gateway call.
        // Resume with the existing payment ID so the gateway uses it as its own
        // idempotency key — prevents double-charging even if the gateway was reached.
        p = existing
    }

    txnID, err := s.gw.Charge(gwCtx, p.Amount, p.Currency, p.ID.String())
    // ...
}
```

The `orderId` is used directly as the `idempotency_key` — each order maps to exactly one payment.

### Test: 20 goroutines racing to insert the same key

`payment-service/internal/integration/payment_idempotency_test.go` — `TestConcurrentIdempotency`
```go
// N goroutines racing to insert the same idempotency key must result in
// exactly one payment row and one history row.
const N = 20
idemKey := uuid.NewString()
orderID := uuid.New()

gate := make(chan struct{})   // start-gate: all goroutines wait here before racing

for i := 0; i < N; i++ {
    wg.Add(1)
    go func(idx int) {
        defer wg.Done()
        <-gate   // blocked until close(gate) fires all goroutines simultaneously
        results[idx] = repo.Create(ctx, &model.Payment{
            IdempotencyKey: idemKey,
            OrderID: orderID,
            // ...
        }, h)
    }(i)
}

close(gate)   // release all 20 goroutines at once
wg.Wait()

// Exactly one goroutine must succeed
successCount := 0
for _, err := range results {
    if err == nil { successCount++ } else {
        assert.ErrorIs(t, err, repository.ErrDuplicateIdempotencyKey)
    }
}
assert.Equal(t, 1, successCount, "exactly one goroutine must succeed")

// DB must have exactly one payment row
var paymentCount int64
db.Model(&model.Payment{}).Where("idempotency_key = ?", idemKey).Count(&paymentCount)
assert.Equal(t, int64(1), paymentCount)
```

**Observed result (Phase 4, 2026-05-21):** `N=20` concurrent inserts → 1 payment row, 19 × `ErrDuplicateIdempotencyKey`, PASS.

Run with: `go test -tags=integration -v -race -run TestConcurrentIdempotency ./internal/integration/`

---

## Summary: Strategy Selection Rationale

Each strategy maps to a specific threat at a specific granularity:

```
Order Service     → Row-level DB lock (pessimistic)
                    Threat: concurrent state machine transitions on one row
                    Lock scope: one order row, sub-ms duration

Product Service   → Conditional UPDATE (optimistic, no version field)
                    Threat: concurrent stock reservations exceeding available stock
                    Lock scope: none (atomic SQL clause is the guard)

Cart Service      → Redis WATCH/MULTI/EXEC (optimistic, application-level)
                    Threat: concurrent writes to the same user's cart hash
                    Lock scope: one Redis key, one transaction pipeline

Payment Service   → DB UNIQUE constraint (declarative, no application logic)
                    Threat: duplicate event delivery via Kafka
                    Lock scope: entire payments table (index enforces uniqueness globally)
```

A single strategy applied everywhere would be wrong: pessimistic locks on the cart (a hot key) would serialize all cart writes; Redis transactions on the order state machine (a DB entity) would bypass the DB transaction; UNIQUE constraints on stock reservation (a numeric field) cannot express the conditional decrement semantics.
