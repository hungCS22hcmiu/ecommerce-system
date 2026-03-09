# Concurrency Locking Strategy — Per Service

> Reference: proposal §10 (Concurrency & Race Conditions) and Appendix C
> (Concurrency Decision Matrix)

---

## Decision Matrix (from Appendix C)

| Situation                         | Optimistic | Pessimistic | Redis Atomic | Idempotency Key |
|-----------------------------------|:----------:|:-----------:|:------------:|:---------------:|
| Stock reservation (normal)        | ✅ Primary | fallback    | —            | —               |
| Order state transition            | —          | ✅ Primary  | —            | —               |
| Cart item update                  | —          | —           | ✅ WATCH/EXEC | —              |
| Payment processing                | —          | —           | —            | ✅ Primary      |
| Login attempt counter             | —          | ✅ row lock  | ✅ INCR      | —               |
| Cache operations (generic)        | —          | —           | ✅ atomic    | —               |

---

## Service-by-Service Breakdown

### 1. User Service (Golang)

**Race condition:** Two goroutines both read `failed_login_attempts = 4` and
both allow login, skipping the lockout that should trigger at 5.

**Strategy: Pessimistic Lock — `SELECT ... FOR UPDATE`**

```go
// GORM example (Day 8 implementation)
r.db.WithContext(ctx).
    Clauses(clause.Locking{Strength: "UPDATE"}).
    Where("email = ?", email).
    First(&user)
```

**Why pessimistic here (not optimistic)?**
- Login is a write-heavy operation on a single row — high contention expected.
- Optimistic locking would cause many retries under brute-force attack (bad UX).
- The lock duration is extremely short (one bcrypt compare + one UPDATE) so
  queue-up cost is low.
- `SELECT FOR UPDATE` is PostgreSQL-native and has zero extra round-trips.

**Also used:** Redis `INCR` for the sliding-window rate-limiter at the cache
layer (atomic — no Go-level lock needed).

---

### 2. Product Service (Java/Spring Boot)

**Race condition:** 100 concurrent users buying the last 10 items. Without
locking, all 100 read `stock = 10`, all reserve, and 100 reservations are
created — 90 units oversold.

**Strategy: Optimistic Lock — `@Version` + `@Retry`**

```java
@Entity
public class Product {
    @Version          // JPA adds WHERE version = ? to every UPDATE
    private Long version;
}

@Retry(name = "stockRetry", maxAttempts = 3,
       retryExceptions = OptimisticLockException.class)
public StockReservation reserveStock(Long productId, int qty, String orderId) { ... }
```

**Why optimistic (not pessimistic)?**
- Most requests succeed on the first try (normal traffic, not a flash sale).
- No database row lock held during business-logic validation — higher throughput.
- `OptimisticLockException` is cheap: JPA re-reads the row and retries. Only
  the "loser" pays the cost.
- Pessimistic lock is available as a fallback for flash-sale endpoints where
  contention is near 100%:

```java
@Lock(LockModeType.PESSIMISTIC_WRITE)
@Query("SELECT p FROM Product p WHERE p.id = :id")
Product findByIdWithPessimisticLock(@Param("id") Long id);
```

---

### 3. Cart Service (Golang)

**Race condition:** User opens two browser tabs and adds items simultaneously.
Both goroutines read the same cart JSON from Redis, each appends an item
independently, and both write back — the second write overwrites the first
item (lost update).

**Strategy: Redis Optimistic Lock — `WATCH / MULTI / EXEC`**

```go
r.redis.Watch(ctx, func(tx *redis.Tx) error {
    // WATCH: if key changes between here and EXEC, transaction aborts
    cartJSON, _ := tx.Get(ctx, key).Result()
    cart := deserializeCart(cartJSON)
    cart.AddItem(item)

    _, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
        pipe.Set(ctx, key, serializeCart(cart), 30*time.Minute)
        return nil
    })
    return err   // redis.TxFailedErr → retry loop
}, key)
```

**Why Redis-level optimistic lock (not a DB transaction or mutex)?**
- Cart primary storage is Redis, not PostgreSQL — locking must happen at the
  Redis layer.
- `WATCH/MULTI/EXEC` is Redis's built-in optimistic CAS mechanism — no
  external library needed.
- A Go `sync.Mutex` would only protect a single process instance; in a
  horizontally scaled deployment it would not help.
- Contention on a single user's cart is expected to be very low (rare to have
  two simultaneous writes), so optimistic is the right default.

**Background goroutine sync to PostgreSQL** uses a channel + ticker — the
goroutine serialises all writes, so no additional lock is needed for
PostgreSQL persistence.

---

### 4. Order Service (Java/Spring Boot)

**Race condition:** A Kafka `payments.completed` event arrives at the same
millisecond as the user clicks "Cancel". Two threads both try to transition
the order from `PENDING` — one to `CONFIRMED`, one to `CANCELLED`. Without
locking, both could succeed and the order ends up in an inconsistent state.

**Strategy: Pessimistic Lock — `SELECT ... FOR UPDATE` on order row**

```java
@Lock(LockModeType.PESSIMISTIC_WRITE)
@Query("SELECT o FROM Order o WHERE o.id = :id")
Optional<Order> findByIdWithLock(@Param("id") Long id);

@Transactional
public Order transitionStatus(Long orderId, OrderStatus newStatus) {
    Order order = orderRepository.findByIdWithLock(orderId)
        .orElseThrow(() -> new OrderNotFoundException(orderId));
    if (!order.getStatus().canTransitionTo(newStatus)) {
        throw new InvalidStateTransitionException(...);
    }
    order.setStatus(newStatus);
    return orderRepository.save(order);
}
```

**Why pessimistic (not optimistic)?**
- State transitions are infrequent per order (at most 4–5 in its lifetime)
  but the consequence of a race is catastrophic (paying AND cancelling).
- Optimistic lock would still allow both threads to attempt the transition —
  one would throw `OptimisticLockException`, but the application would need
  complex retry/compensation logic.
- Pessimistic lock guarantees exactly-once: the second thread simply queues
  up, reads the already-transitioned state, and `canTransitionTo()` returns
  false cleanly.
- Lock duration is sub-millisecond (just a status update — no bcrypt, no HTTP
  calls inside the lock).

**`CompletableFuture.allOf()` for stock reservation** is used before the
order row exists (nothing to lock), so parallel reservation is safe without
any additional lock.

---

### 5. Payment Service (Golang)

**Race condition:** Kafka may redeliver the same `orders.created` event
(at-least-once delivery). Two worker goroutines both start processing payment
for the same order simultaneously.

**Strategy: Idempotency Key + DB Unique Constraint**

```go
// 1. Attempt INSERT with unique idempotency_key
if err := s.repo.Create(ctx, payment); err != nil {
    if isDuplicateKeyError(err) {
        // Another goroutine already claimed it — return existing result
        return s.repo.FindByIdempotencyKey(ctx, req.IdempotencyKey)
    }
    return nil, err
}
// 2. Only the goroutine whose INSERT succeeded continues to the gateway
```

**Why idempotency key (not a lock)?**
- A pessimistic lock would require both goroutines to acquire a distributed
  lock before processing, adding latency on every payment.
- The DB `UNIQUE` constraint on `idempotency_key` acts as an atomic "claim"
  mechanism — exactly one goroutine succeeds, the rest get a duplicate-key
  error and return the existing result.
- This naturally handles Kafka at-least-once delivery with zero extra
  infrastructure (no ZooKeeper, no Redis lock).
- It also handles retries from the client side (same key → same result).

**Worker pool channel pattern** avoids shared mutable state between goroutines
— each worker owns its own stack frame, so no mutex is needed for the
processing logic itself.

---

## Summary: Why Each Service Uses Its Strategy

| Service | Strategy | Root Reason |
|---------|----------|-------------|
| User | Pessimistic (SELECT FOR UPDATE) | Login is write-heavy on one row; lockout correctness is critical; lock duration is short |
| Product | Optimistic (@Version) + Retry | Normal traffic has low contention; high throughput matters more; retries are cheap |
| Cart | Redis WATCH/MULTI/EXEC | Primary store is Redis, not DB; optimistic is correct for low-contention per-user writes |
| Order | Pessimistic (SELECT FOR UPDATE) | Catastrophic if two state transitions both succeed; lock duration is sub-ms |
| Payment | Idempotency Key + DB UNIQUE | Duplicate event delivery is the threat; DB constraint is the lightest correct solution |