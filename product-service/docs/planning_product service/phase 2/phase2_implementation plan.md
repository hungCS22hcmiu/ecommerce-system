# Phase 2: Inventory Management with Optimistic Locking

## Context

Phase 1 delivered basic Product CRUD. Phase 2 adds **inventory operations** (reserve/release stock) that will be called by the order-service during checkout. The core challenge: multiple concurrent orders can try to reserve the same stock simultaneously. We use **optimistic locking** (`@Version` + `@Retryable`) to handle this safely without database-level locks, matching the project's locking strategy (see `docs/adr/locking-strategy.md`).

**Goal:** A working concurrent stock reservation system you can demo and explain in an interview.

---

## What Already Exists

| Asset | Status | Notes |
|-------|--------|-------|
| `Product.java` — `stockQuantity`, `stockReserved`, `@Version` | Done | Fields and optimistic lock already on the entity |
| `stock_movements` table + `MovementType` enum (IN, OUT, RESERVE, RELEASE) | Done | DB table in V1 migration; Java enum in `model/` |
| `GlobalExceptionHandler` | Done | Needs new exception handlers added |
| `spring-retry` dependency | **Missing** | Must add to `pom.xml` |

Key insight: there is **no separate Inventory entity**. Stock lives on `Product` (fields `stockQuantity` and `stockReserved`). The "inventory" layer is a new **service + controller** that operates on the Product entity's stock fields.

---

## Implementation Steps

### Step 1: Add `spring-retry` dependency

**File:** `pom.xml`

Add two dependencies:
```xml
<dependency>
    <groupId>org.springframework.retry</groupId>
    <artifactId>spring-retry</artifactId>
</dependency>
<dependency>
    <groupId>org.springframework</groupId>
    <artifactId>spring-aspects</artifactId>
</dependency>
```

Add `@EnableRetry` to a config class (new `RetryConfig.java` or existing `JpaConfig.java`).

---

### Step 2: Create `StockMovement` entity

**New file:** `model/StockMovement.java`

Maps to the existing `stock_movements` table. Fields: `id`, `productId` (FK), `type` (MovementType enum), `quantity`, `referenceId`, `reason`, `createdAt`. This provides an audit trail of all stock changes.

---

### Step 3: Create `InsufficientStockException`

**New file:** `exception/InsufficientStockException.java`

Simple RuntimeException. Thrown when `stockQuantity - stockReserved < requestedQuantity`.

Update `GlobalExceptionHandler` to handle:
- `InsufficientStockException` → **409 Conflict**, code `INSUFFICIENT_STOCK`
- `OptimisticLockingFailureException` → **409 Conflict**, code `CONCURRENT_MODIFICATION` (when all retries exhausted)

---

### Step 4: Create Inventory DTOs

**New files in `dto/`:**
- `StockReserveRequest.java` — `@NotNull @Min(1) Integer quantity`, `String referenceId` (order ID for audit trail)
- `StockReleaseRequest.java` — `@NotNull @Min(1) Integer quantity`, `String referenceId`
- `StockResponse.java` — `Long productId`, `int stockQuantity`, `int stockReserved`, `int availableStock`

---

### Step 5: Create `InventoryService` interface + implementation

**New files:**
- `service/InventoryService.java` (interface)
- `service/serviceImpl/InventoryServiceImpl.java`

**Methods:**

```java
public interface InventoryService {
    StockResponse reserveStock(Long productId, int quantity, String referenceId);
    StockResponse releaseStock(Long productId, int quantity, String referenceId);
    StockResponse getStockLevel(Long productId);
}
```

**`InventoryServiceImpl` logic:**

- `@Service`, `@RequiredArgsConstructor`
- Depends on `ProductRepository` + `StockMovementRepository`

**`reserveStock`:**
1. `@Transactional` + `@Retryable(retryFor = OptimisticLockingFailureException.class, maxAttempts = 3, backoff = @Backoff(delay = 100))`
2. Fetch product by ID (throw `ProductNotFoundException` if missing)
3. Calculate `available = stockQuantity - stockReserved`
4. If `available < quantity` → throw `InsufficientStockException`
5. `product.setStockReserved(stockReserved + quantity)` → save (triggers `@Version` check)
6. Record a `StockMovement` with type `RESERVE`
7. Return `StockResponse`

**`releaseStock`:**
1. `@Transactional` + `@Retryable` (same config)
2. Fetch product
3. Validate `stockReserved >= quantity` (can't release more than reserved)
4. `product.setStockReserved(stockReserved - quantity)` → save
5. Record a `StockMovement` with type `RELEASE`
6. Return `StockResponse`

**`getStockLevel`:**
1. `@Transactional(readOnly = true)`
2. Fetch product, return current stock info

**Why `@Retryable` on the service method, not the repository:** The entire read-check-write must be retried from the beginning with fresh data. Retrying just the save would re-save stale data.

---

### Step 6: Create `StockMovementRepository`

**New file:** `repository/StockMovementRepository.java`

Extends `JpaRepository<StockMovement, Long>`. No custom queries needed initially.

---

### Step 7: Create `InventoryController`

**New file:** `controller/InventoryController.java`

```
@RestController
@RequestMapping("/api/v1/inventory")
```

**Endpoints:**

| Method | Path | Body | Response |
|--------|------|------|----------|
| `POST` | `/{productId}/reserve` | `StockReserveRequest` | `ApiResponse<StockResponse>` (200) |
| `POST` | `/{productId}/release` | `StockReleaseRequest` | `ApiResponse<StockResponse>` (200) |
| `GET`  | `/{productId}` | — | `ApiResponse<StockResponse>` (200) |

These are **internal endpoints** (called by order-service), no seller auth header needed.

---

### Step 8: Unit Tests — `InventoryServiceImplTest`

**New file:** `src/test/java/com/ecommerce/product_service/service/InventoryServiceImplTest.java`

Follow existing pattern: `@ExtendWith(MockitoExtension.class)`, `@Nested` groups, AssertJ assertions.

**Test groups:**

`@Nested ReserveStock:`
- Happy path: stock=10, reserve 5 → stockReserved becomes 5, StockMovement saved
- Insufficient stock: stock=10, reserved=8, try to reserve 5 → throws `InsufficientStockException`
- Product not found → throws `ProductNotFoundException`
- Verify `repository.save()` is called with updated stockReserved

`@Nested ReleaseStock:`
- Happy path: stockReserved=5, release 3 → stockReserved becomes 2, StockMovement saved
- Release more than reserved → throws `IllegalArgumentException`
- Product not found → throws `ProductNotFoundException`

`@Nested GetStockLevel:`
- Returns correct available stock (stockQuantity - stockReserved)
- Product not found → throws `ProductNotFoundException`

---

### Step 9: Concurrent Integration Test — THE KEY DELIVERABLE

**New file:** `src/test/java/com/ecommerce/product_service/integration/InventoryConcurrencyTest.java`

This is the interview-ready proof that optimistic locking works.

**Setup:** `@SpringBootTest` with real database (Testcontainers or the Docker Postgres). Insert a product with `stockQuantity=5, stockReserved=0`.

**Test 1 — `concurrent_reservations_with_optimistic_locking`:**
1. Create `ExecutorService` with 10 threads
2. Submit 10 `Callable` tasks, each calling `inventoryService.reserveStock(productId, 1, "order-" + i)`
3. Collect `Future` results — count successes vs failures
4. Assert: exactly 5 succeed, exactly 5 fail with `InsufficientStockException`
5. Assert: final `stockReserved == 5`
6. Assert: exactly 5 `StockMovement` records with type `RESERVE`

**Test 2 — `retry_handles_optimistic_lock_exception`:**
1. Same setup but with `stockQuantity=10` (enough for all)
2. Fire 10 concurrent reservations of 1 each
3. All 10 should succeed (some after retries)
4. Assert: final `stockReserved == 10`
5. This proves `@Retryable` correctly handles `OptimisticLockingFailureException`

**Test 3 — `without_version_annotation_race_condition` (educational/failing test):**
- Document what happens without `@Version`: multiple threads read the same stockReserved value, all write back the same incremented value → lost updates
- This can be a commented-out conceptual test with explanation, since removing `@Version` at runtime isn't practical. Alternatively, use a raw SQL update that bypasses the version check to demonstrate the lost-update problem.

---

## Files Summary

| Action | File |
|--------|------|
| **Modify** | `pom.xml` — add spring-retry + spring-aspects |
| **Modify** | `config/` — add `@EnableRetry` (new `RetryConfig.java`) |
| **Modify** | `exception/GlobalExceptionHandler.java` — add 2 exception handlers |
| **Create** | `model/StockMovement.java` |
| **Create** | `exception/InsufficientStockException.java` |
| **Create** | `dto/StockReserveRequest.java` |
| **Create** | `dto/StockReleaseRequest.java` |
| **Create** | `dto/StockResponse.java` |
| **Create** | `service/InventoryService.java` |
| **Create** | `service/serviceImpl/InventoryServiceImpl.java` |
| **Create** | `repository/StockMovementRepository.java` |
| **Create** | `controller/InventoryController.java` |
| **Create** | `test/.../service/InventoryServiceImplTest.java` |
| **Create** | `test/.../integration/InventoryConcurrencyTest.java` |

---

## Verification

1. **Unit tests:** `./mvnw test` — all existing 28+ product tests still pass + new inventory unit tests pass
2. **Integration test:** Run `InventoryConcurrencyTest` with Docker Postgres running — concurrent test proves exactly 5/10 reservations succeed
3. **Manual smoke test:**
   - Start service: `./mvnw spring-boot:run`
   - Create a product with stockQuantity=5
   - `POST /api/v1/inventory/{id}/reserve` with quantity=3 → success, available=2
   - `POST /api/v1/inventory/{id}/reserve` with quantity=3 → 409 Insufficient Stock
   - `POST /api/v1/inventory/{id}/release` with quantity=2 → success, available=4
   - `GET /api/v1/inventory/{id}` → verify stock levels

---

## Interview Talking Points

- **Why optimistic over pessimistic?** Low contention (products rarely get simultaneous orders for same item), higher throughput — no database row locks held during request processing
- **What does `@Version` do?** Hibernate adds `WHERE version = ?` to UPDATE. If another TX changed the row, version won't match → `OptimisticLockingFailureException`
- **Why retry?** The failing TX had stale data. Retrying re-reads fresh data and tries again. 3 attempts handles typical contention.
- **What about high contention?** If a flash sale has 1000 concurrent buyers, optimistic locking with 3 retries may not be enough. Switch to pessimistic locking (`SELECT FOR UPDATE`) or use Redis atomic decrements for that use case.
