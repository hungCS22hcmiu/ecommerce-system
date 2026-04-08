# JPA Optimistic Locking & Concurrent Inventory Management

## 1. Optimistic Locking: How `@Version` Works in JPA

### The Core Concept

Optimistic locking assumes conflicts are **rare**. Instead of holding a database lock, JPA tracks a **version number** on each row. If two transactions try to update the same row, only the first one wins — the second gets an exception.

### Setting It Up

```java
@Entity
public class Product {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    private String name;
    private int stock;
    private BigDecimal price;

    @Version
    private Long version;  // JPA manages this automatically
}
```

> The `@Version` field can be `int`, `Integer`, `long`, `Long`, `short`, `Short`, or `java.sql.Timestamp`.

### What Happens Under the Hood

When JPA executes an `UPDATE`, it automatically appends the version check:

```sql
-- What JPA generates behind the scenes:
UPDATE product
SET stock = 99, version = 6        -- increments version
WHERE id = 1 AND version = 5;      -- checks current version
```

- If **1 row is affected** → update succeeded, version bumped.
- If **0 rows affected** → someone else updated first → JPA throws `OptimisticLockException`.

### Handling `OptimisticLockException`

```java
try {
    productService.updateStock(productId, newQty);
} catch (OptimisticLockException | ObjectOptimisticLockingFailureException ex) {
    // Re-read fresh data and retry, or surface an error to the caller
    throw new ConflictException("Product was modified concurrently. Please retry.");
}
```

`ObjectOptimisticLockingFailureException` is Spring Data's wrapper — catch whichever layer your code works at.

---

## 2. Optimistic vs. Pessimistic Locking for a Product Catalog

### Pessimistic Locking (`SELECT ... FOR UPDATE`)

```java
@Lock(LockModeType.PESSIMISTIC_WRITE)
Optional<Product> findById(Long id);
```

- Acquires a **row-level DB lock** immediately.
- Other readers/writers **block** until the lock is released.
- Guarantees no conflict — but at the cost of serialization.

### Why Optimistic Wins for a Product Catalog

| Factor | Pessimistic | Optimistic |
|---|---|---|
| Read-heavy traffic | ❌ Blocks all readers | ✅ Reads never lock |
| Low write contention | ❌ Overkill | ✅ Conflicts are rare |
| Distributed / microservices | ❌ DB-tied locks | ✅ Works across nodes |
| Throughput under load | ❌ Lock queue bottleneck | ✅ Scales horizontally |
| Deadlock risk | ❌ Possible | ✅ None |

**Rule of thumb:**
- **High read, low write contention** (browsing a catalog, viewing prices) → **Optimistic**.
- **High contention on the same row** (flash-sale, single hot ticket) → **Pessimistic** or queue-based.

A product catalog is read thousands of times between each stock update. Pessimistic locking would serialize every product page load — that's a serious performance anti-pattern.

---

## 3. Spring `@Retryable` — Automatic Retry on Version Conflicts

### Why Retry?

An `OptimisticLockException` doesn't mean something is *wrong* — it means "try again with fresh data." Spring Retry automates this.

### Setup

```xml
<!-- pom.xml -->
<dependency>
    <groupId>org.springframework.retry</groupId>
    <artifactId>spring-retry</artifactId>
</dependency>
<dependency>
    <groupId>org.springframework</groupId>
    <artifactId>spring-aspects</artifactId>
</dependency>
```

Enable in your config:

```java
@Configuration
@EnableRetry
public class RetryConfig {}
```

### Annotate the Service Method

```java
@Service
public class ProductService {

    @Retryable(
        retryFor = { ObjectOptimisticLockingFailureException.class },
        maxAttempts = 3,
        backoff = @Backoff(delay = 100, multiplier = 2)  // 100ms, 200ms
    )
    @Transactional
    public void decrementStock(Long productId, int quantity) {
        Product product = productRepository.findById(productId)
            .orElseThrow(() -> new ProductNotFoundException(productId));

        if (product.getStock() < quantity) {
            throw new InsufficientStockException(productId);
        }

        product.setStock(product.getStock() - quantity);
        productRepository.save(product);
    }

    @Recover
    public void recoverFromLockFailure(
            ObjectOptimisticLockingFailureException ex,
            Long productId,
            int quantity) {
        // Called only after all retry attempts are exhausted
        log.error("Stock update failed after retries for product {}", productId);
        throw new ServiceUnavailableException("Could not update stock — too much contention.");
    }
}
```

### How the Retry Flow Works

```
Attempt 1 → OptimisticLockException
    ↓ wait 100ms (re-reads fresh entity on next attempt)
Attempt 2 → OptimisticLockException
    ↓ wait 200ms
Attempt 3 → ✅ Success  (or → @Recover if still failing)
```

> **Important:** Each retry starts a **new transaction**, so the entity is re-read from the database with the latest version. This is why `@Transactional` + `@Retryable` must be on the **same method** — Spring Retry wraps the whole transaction.

---

## 4. Stock Reservation vs. Stock Deduction — Two-Phase Approach

### The Problem with Direct Deduction

```
User adds item to cart → stock immediately decremented
User abandons cart     → stock never restored
Result: phantom out-of-stock
```

Deducting stock at add-to-cart time causes **inventory bleed** from cart abandonment (which is extremely common in e-commerce).

### The Two-Phase Model

```
Phase 1: RESERVE   — soft-hold stock when item is added to cart
Phase 2: DEDUCT    — hard-commit stock only when order is confirmed/paid
```

### Entity Design

```java
@Entity
public class Product {

    @Id
    private Long id;

    private int totalStock;       // Physical inventory
    private int reservedStock;    // Soft-held by active carts/sessions

    @Version
    private Long version;

    // Available for new reservations
    public int getAvailableStock() {
        return totalStock - reservedStock;
    }
}
```

### Phase 1: Reserve (Cart Add)

```java
@Retryable(retryFor = ObjectOptimisticLockingFailureException.class, maxAttempts = 3)
@Transactional
public Reservation reserveStock(Long productId, int qty, String sessionId) {
    Product product = productRepository.findById(productId).orElseThrow(...);

    if (product.getAvailableStock() < qty) {
        throw new InsufficientStockException();
    }

    product.setReservedStock(product.getReservedStock() + qty);
    productRepository.save(product);

    // Create a time-limited reservation record
    return reservationRepository.save(
        Reservation.builder()
            .productId(productId)
            .quantity(qty)
            .sessionId(sessionId)
            .expiresAt(Instant.now().plus(Duration.ofMinutes(15)))
            .status(ReservationStatus.ACTIVE)
            .build()
    );
}
```

### Phase 2: Deduct (Order Confirmed)

```java
@Retryable(retryFor = ObjectOptimisticLockingFailureException.class, maxAttempts = 3)
@Transactional
public void confirmReservation(Long reservationId) {
    Reservation reservation = reservationRepository.findById(reservationId).orElseThrow(...);
    Product product = productRepository.findById(reservation.getProductId()).orElseThrow(...);

    // Release the soft-hold AND reduce real stock
    product.setReservedStock(product.getReservedStock() - reservation.getQuantity());
    product.setTotalStock(product.getTotalStock() - reservation.getQuantity());
    productRepository.save(product);

    reservation.setStatus(ReservationStatus.CONFIRMED);
    reservationRepository.save(reservation);
}
```

### Phase 2 (Alternate): Release Reservation (Cart Abandoned / Payment Failed)

```java
@Transactional
public void releaseReservation(Long reservationId) {
    Reservation reservation = reservationRepository.findById(reservationId).orElseThrow(...);
    Product product = productRepository.findById(reservation.getProductId()).orElseThrow(...);

    // Only release the soft-hold — totalStock unchanged
    product.setReservedStock(product.getReservedStock() - reservation.getQuantity());
    productRepository.save(product);

    reservation.setStatus(ReservationStatus.RELEASED);
    reservationRepository.save(reservation);
}
```

### Expiry Cleanup (Scheduled Job)

```java
@Scheduled(fixedDelay = 60_000)  // every 60 seconds
@Transactional
public void expireStaleReservations() {
    List<Reservation> expired = reservationRepository
        .findByStatusAndExpiresAtBefore(ReservationStatus.ACTIVE, Instant.now());

    for (Reservation r : expired) {
        releaseReservation(r.getId());
    }
}
```

### Two-Phase Summary

```
                  ┌─────────────────────────────────────┐
Add to Cart ────► │  RESERVE: reservedStock += qty       │
                  │  totalStock unchanged                 │
                  └────────────┬──────────────┬──────────┘
                               │              │
                    Payment OK │              │ Abandoned / Expired
                               ▼              ▼
                  ┌─────────────────┐  ┌─────────────────────┐
                  │ DEDUCT:         │  │ RELEASE:            │
                  │ totalStock -= q │  │ reservedStock -= qty │
                  │ reservedStock   │  │ totalStock unchanged │
                  │     -= qty      │  └─────────────────────┘
                  └─────────────────┘
```

---

## Quick Reference Cheat Sheet

| Concept | Key Point |
|---|---|
| `@Version` | JPA increments version on every update; `0 rows affected` → `OptimisticLockException` |
| Optimistic lock SQL | `WHERE id=? AND version=?` — no DB lock held |
| Pessimistic lock | `SELECT FOR UPDATE` — holds row lock, blocks others |
| Use optimistic when | Read-heavy, low write contention (product catalog) |
| Use pessimistic when | High contention on same row (flash sales) |
| `@Retryable` | Auto-retries on `OptimisticLockException`; each attempt = new transaction |
| `@Recover` | Fallback after all retry attempts exhausted |
| Reserve phase | Soft-hold: `reservedStock += qty`, `totalStock` unchanged |
| Deduct phase | Hard-commit: both `totalStock` and `reservedStock` decrease |
| Release phase | Abandoned cart: only `reservedStock` decreases |