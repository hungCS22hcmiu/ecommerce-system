# Product Service — How It Works

Java 21 · Spring Boot 3.5 · PostgreSQL 16 · Redis · Port 8081

---

## Directory Structure

```
product-service/
├── src/main/java/com/ecommerce/product_service/
│   ├── ProductServiceApplication.java
│   ├── config/
│   │   ├── RedisConfig.java        # Cache manager, TTLs, serializer
│   │   ├── JpaConfig.java          # JPA auditing with OffsetDateTime
│   │   ├── AsyncConfig.java        # ThreadPoolTaskExecutor
│   │   └── RetryConfig.java        # Enables @Retryable
│   ├── controller/
│   │   ├── ProductController.java
│   │   ├── InventoryController.java
│   │   └── HealthController.java
│   ├── service/
│   │   ├── ProductService.java     # Interface
│   │   ├── InventoryService.java   # Interface
│   │   ├── CacheWarmupService.java # Async startup pre-load
│   │   └── serviceImpl/
│   │       ├── ProductServiceImpl.java
│   │       └── InventoryServiceImpl.java
│   ├── repository/
│   │   ├── ProductRepository.java
│   │   ├── CategoryRepository.java
│   │   ├── ProductImageRepository.java
│   │   └── StockMovementRepository.java
│   ├── model/
│   │   ├── Product.java
│   │   ├── Category.java
│   │   ├── ProductImage.java
│   │   ├── StockMovement.java
│   │   ├── ProductStatus.java      # ACTIVE | INACTIVE | DELETED
│   │   └── MovementType.java       # IN | OUT | RESERVE | RELEASE
│   ├── dto/
│   │   ├── ProductResponse.java
│   │   ├── ProductSummaryResponse.java
│   │   ├── CreateProductRequest.java
│   │   ├── UpdateProductRequest.java
│   │   ├── StockReserveRequest.java
│   │   ├── StockReleaseRequest.java
│   │   ├── StockResponse.java
│   │   ├── ProductImageRequest.java
│   │   ├── ProductImageResponse.java
│   │   └── ApiResponse.java        # Generic envelope { success, data, meta, error }
│   └── exception/
│       ├── GlobalExceptionHandler.java
│       ├── ProductNotFoundException.java
│       ├── InsufficientStockException.java
│       └── ProductAccessDeniedException.java
├── src/main/resources/
│   ├── application.yaml
│   └── db/migration/
│       ├── V1__baseline_schema.sql # Schema + indexes
│       └── V2__seed_products.sql   # 5 categories, 200 products
└── src/test/java/com/ecommerce/product_service/
    ├── ProductServiceApplicationTests.java
    ├── service/
    │   ├── ProductServiceImplTest.java      # 30 unit tests (Mockito)
    │   ├── InventoryServiceImplTest.java    # 9 unit tests
    │   └── ProductServiceCacheTest.java     # 5 AOP cache tests
    └── integration/
        ├── ProductCacheIntegrationTest.java  # 15 tests (real Redis + PG)
        └── InventoryConcurrencyTest.java     # 2 concurrency tests
```

---

## Models

### Product

The core entity. Notable fields:

| Field | Type | Purpose |
|---|---|---|
| `id` | Long | PK, auto-generated |
| `name` | String (200) | NOT NULL |
| `description` | String (TEXT) | nullable |
| `price` | BigDecimal (10,2) | NOT NULL |
| `sellerId` | UUID | Identifies the seller; no FK to user-service |
| `status` | ProductStatus | ACTIVE / INACTIVE / DELETED (soft-delete) |
| `stockQuantity` | int | Total units in warehouse |
| `stockReserved` | int | Units locked for pending orders |
| `version` | Long | **@Version** — optimistic locking |
| `category` | Category | @ManyToOne, lazy |
| `images` | List\<ProductImage\> | @OneToMany, cascade all, orphan removal |
| `createdAt / updatedAt` | OffsetDateTime | @CreatedDate / @LastModifiedDate via AuditingEntityListener |

`stockAvailable = stockQuantity − stockReserved` is a derived value computed in DTOs, not stored in the DB.

### Category

Hierarchical tree (self-referencing):
- `parent` (ManyToOne, nullable) — null = root category
- `children` (OneToMany, lazy)
- `slug` (UNIQUE) — URL-friendly identifier
- `sortOrder` — display order

### ProductImage

Multiple images per product, ordered by `sortOrder`. The first image becomes the `thumbnailUrl` in `ProductSummaryResponse`.

### StockMovement

Append-only audit log. Every stock change creates one row:

| Field | Purpose |
|---|---|
| `type` | IN / OUT / RESERVE / RELEASE |
| `quantity` | How many units changed |
| `referenceId` | Order ID or purchase ID that caused the change |
| `reason` | Optional human note |

Never updated, only inserted. Enables full inventory reconciliation.

---

## DTOs

| DTO | Used When |
|---|---|
| `ProductResponse` | Full product detail (GET /{id}, POST, PUT) |
| `ProductSummaryResponse` | Paginated listings (thumbnail only, no full image list) |
| `CreateProductRequest` | POST body — name, price, stock, categoryId, images |
| `UpdateProductRequest` | PUT body — all fields optional, partial update |
| `StockReserveRequest` | Record: quantity (≥1), referenceId |
| `StockReleaseRequest` | Record: quantity (≥1), referenceId |
| `StockResponse` | Record: productId, stockQuantity, stockReserved, availableStock |
| `ApiResponse<T>` | Universal envelope: `{ success, data, meta?, error? }` |

`ApiResponse` has static factories:
- `ApiResponse.ok(T data)` — success with payload
- `ApiResponse.ok(Page<T> page)` — adds `PageMeta { page, size, totalElements, totalPages }`
- `ApiResponse.error(String code, String message)` — failure

---

## Repositories

### ProductRepository

Extends `JpaRepository<Product, Long>` and `JpaSpecificationExecutor<Product>`.

Key methods:
- `findByIdAndStatus(id, ACTIVE)` — filters out soft-deleted products
- `findByCategoryIdAndStatus(categoryId, status, pageable)` — category browse
- `existsByIdAndSellerId(id, sellerId)` — ownership check before updates
- `findTop100ByStatusOrderByUpdatedAtDesc(ACTIVE)` — cache warmup query
- `searchActive(query, pageable)` — native query using PostgreSQL full-text search:

```sql
SELECT * FROM products
WHERE to_tsvector('english', name || ' ' || COALESCE(description, ''))
      @@ plainto_tsquery('english', :query)
  AND status = 'ACTIVE'
```

The GIN index `idx_products_fts` on `to_tsvector(...)` makes this fast.

### CategoryRepository

- `findBySlug(slug)` — slug lookup
- `findByParentIsNullOrderBySortOrderAsc()` — root categories
- `findByParentIdOrderBySortOrderAsc(parentId)` — direct children

### StockMovementRepository

Only standard CRUD from JpaRepository — all inserts, never updates.

---

## Services

### ProductServiceImpl

Annotated `@Service`, `@Transactional(readOnly = true)` by default (write methods override this).

**createProduct(sellerId, request)**
- `@Transactional` + `@CacheEvict(value="productList", allEntries=true)`
- Resolves category by ID (nullable — products can have no category)
- Builds `Product` entity with status=ACTIVE, stockReserved=0
- Saves, then maps to `ProductResponse`
- Evicts all list caches (new product invalidates any paginated result)

**getProduct(id)**
- `@Cacheable(value="product", key="#id")`
- Queries only ACTIVE products — throws `ProductNotFoundException` for missing or DELETED
- Cache hit = zero DB queries; miss = DB query + populate cache

**listProducts(categoryId, status, pageable)**
- `@Cacheable(value="productList", key="{#categoryId, #status, #pageable.pageNumber, #pageable.pageSize, #pageable.sort}")`
- If `categoryId` provided: uses `findByCategoryIdAndStatus`
- Otherwise: uses `JpaSpecificationExecutor` with a dynamic `Specification` (easily extended with more predicates)
- Returns `Page<ProductSummaryResponse>` (lightweight DTOs for listings)

**searchProducts(query, pageable)**
- `@Cacheable(value="productList", key="{'search', #query, #pageable.pageNumber, #pageable.pageSize}")`
- Delegates to the native full-text search query
- Results cached in "productList" (3-min TTL)

**updateProduct(id, sellerId, request)**
- `@Transactional` + `@Caching(put = @CachePut(value="product", key="#id"), evict = @CacheEvict(value="productList", allEntries=true))`
- Ownership check: `existsByIdAndSellerId` → throws `ProductAccessDeniedException` (403) if mismatch
- Partial update: only applies non-null fields from `UpdateProductRequest`
- Replaces images only when `request.getImages()` is non-null

**deleteProduct(id, sellerId)**
- `@Caching(evict = { @CacheEvict(value="product", key="#id"), @CacheEvict(value="productList", allEntries=true) })`
- Ownership check same as update
- Sets `status = DELETED` — **soft delete**, no DB row removal

### InventoryServiceImpl

Annotated `@Service`. No class-level `@Transactional` — each method controls its own.

**reserveStock(productId, quantity, referenceId)**
- `@Transactional` + `@Retryable(retryFor = ObjectOptimisticLockingFailureException.class, maxAttempts = 3, backoff = @Backoff(delay = 100))`
- Loads product → checks `stockQuantity − stockReserved >= quantity` → throws `InsufficientStockException` if not
- Increments `stockReserved` → saves (triggers optimistic lock check)
- On version conflict: Hibernate throws `ObjectOptimisticLockingFailureException` → @Retryable retries with fresh data (up to 3 times, 100ms apart)
- Records `StockMovement(type=RESERVE, quantity, referenceId)`

**releaseStock(productId, quantity, referenceId)**
- Same retry pattern
- Decrements `stockReserved` → throws `IllegalArgumentException` if releasing more than reserved
- Records `StockMovement(type=RELEASE, quantity, referenceId)`

**getStockLevel(productId)**
- `@Transactional(readOnly = true)` — no retry needed (read-only)
- Returns `StockResponse` with computed `availableStock`

### CacheWarmupService

```
@Async("taskExecutor")
@EventListener(ApplicationReadyEvent.class)
void warmCache()
```

Execution flow:
1. Fires after Spring context is fully ready (`ApplicationReadyEvent`)
2. Runs in the `taskExecutor` thread pool — **non-blocking**, does not delay startup
3. Queries `findTop100ByStatusOrderByUpdatedAtDesc(ACTIVE)` — 100 recently updated products
4. For each, calls `productService.getProduct(id)` — triggers `@Cacheable`, populates Redis with 30-min TTL
5. Silently skips any product that throws (e.g., deleted between query and load)
6. Logs count and elapsed time at INFO level

Purpose: fill the "product" cache with popular items before real traffic arrives, preventing a cold-start cache stampede.

---

## Controllers

### ProductController — `/api/v1/products`

| Method | Path | Auth | Notes |
|---|---|---|---|
| POST | `/` | X-Seller-Id header | → 201 Created |
| GET | `/{id}` | none | Cached 30 min |
| GET | `/` | none | ?categoryId= ?status=; paginated; cached 3 min |
| GET | `/search?q=` | none | Full-text; cached 3 min |
| PUT | `/{id}` | X-Seller-Id header | Ownership enforced |
| DELETE | `/{id}` | X-Seller-Id header | 204 No Content |

`@PageableDefault(size=20, sort="createdAt", direction=DESC)` on paginated endpoints.

`X-Seller-Id` is a UUID header expected to be injected by the API gateway. Missing header → 400.

### InventoryController — `/api/v1/inventory`

| Method | Path | Notes |
|---|---|---|
| POST | `/{productId}/reserve` | Used by cart-service / order-service |
| POST | `/{productId}/release` | Order cancellation path |
| GET | `/{productId}` | Current stock levels |

### HealthController — `/health`

| Method | Path | Response |
|---|---|---|
| GET | `/live` | `{"status": "UP"}` |

---

## Configuration

### RedisConfig

- **`redisObjectMapper()`** — custom `ObjectMapper`:
  - `JavaTimeModule` → serializes `OffsetDateTime` as ISO-8601 strings
  - `DefaultTyping.NON_FINAL` with `JsonTypeInfo.As.PROPERTY` → embeds `@class` in JSON so Redis values deserialize back to the correct DTO type
  - `WRITE_DATES_AS_TIMESTAMPS = false`

- **`cacheManager(RedisConnectionFactory, ObjectMapper)`** — `RedisCacheManager`:
  - Key serializer: `StringRedisSerializer` (plain readable keys)
  - Value serializer: `GenericJackson2JsonRedisSerializer` with custom mapper
  - Null values NOT cached (`disableCachingNullValues()`)
  - Key prefix: `"product-service::"`
  - TTLs per cache:
    - `"product"` → 30 minutes
    - `"productList"` → 3 minutes
    - default → 10 minutes

Redis keys look like: `product-service::product::42`

### JpaConfig

- `@EnableJpaAuditing(dateTimeProviderRef="offsetDateTimeProvider")`
- Provides `OffsetDateTime.now()` to `@CreatedDate` / `@LastModifiedDate` fields (instead of Spring's default `LocalDateTime`)

### AsyncConfig

- `@EnableAsync`
- `ThreadPoolTaskExecutor` named `"taskExecutor"`:
  - corePoolSize=5, maxPoolSize=20, queueCapacity=100
  - Thread name prefix: `"product-async-"`
  - `CallerRunsPolicy` — if queue fills up, caller runs the task (backpressure rather than dropping)
  - Waits 30s on shutdown for in-flight tasks to complete

### RetryConfig

- `@EnableRetry` — activates `@Retryable` and `@Recover` annotations across the application

---

## Exception Handling — GlobalExceptionHandler

`@RestControllerAdvice` maps all domain exceptions to `ApiResponse.error(code, message)`:

| Exception | HTTP Status | Error Code |
|---|---|---|
| `ProductNotFoundException` | 404 | `PRODUCT_NOT_FOUND` |
| `InsufficientStockException` | 409 | `INSUFFICIENT_STOCK` |
| `ObjectOptimisticLockingFailureException` | 409 | `CONCURRENT_MODIFICATION` |
| `ProductAccessDeniedException` | 403 | `ACCESS_DENIED` |
| `MethodArgumentNotValidException` | 400 | `VALIDATION_ERROR` (first field error) |
| `MissingRequestHeaderException` | 400 | `BAD_REQUEST` |
| `IllegalArgumentException` | 400 | `BAD_REQUEST` |
| `NoResourceFoundException` | 404 | `NOT_FOUND` |
| `Exception` (fallback) | 500 | `INTERNAL_ERROR` |

---

## Design Patterns

### 1. Optimistic Locking (Product.@Version)

```
Thread A reads product (version=5)
Thread B reads product (version=5)
Thread A writes → version becomes 6 ✓
Thread B writes → expects version 5, finds 6 → ObjectOptimisticLockingFailureException ✗
```

`@Version Long version` on `Product` tells Hibernate to include `WHERE version = ?` in every UPDATE statement. If the row version has changed since the entity was loaded, Hibernate throws `ObjectOptimisticLockingFailureException`.

**Why here**: Stock reservation/release is the highest-contention operation. Optimistic locking avoids database row locks while still preventing lost updates.

### 2. Retry on Conflict

```java
@Retryable(
    retryFor = ObjectOptimisticLockingFailureException.class,
    maxAttempts = 3,
    backoff = @Backoff(delay = 100)
)
```

When the lock conflict is detected, Spring retries the method from scratch (fresh DB read, recompute, re-attempt write). Up to 3 tries with 100ms between each. If all fail, the exception propagates and the caller receives HTTP 409.

**Why retry**: Under moderate concurrency most conflicts resolve within 1-2 retries. Failing immediately would force the client to retry, adding round-trip latency.

### 3. Cache-Aside (Look-Aside)

```
Request → Check Redis cache
              ↓ hit          ↓ miss
         return cached    query DB → store in Redis → return
```

Spring's `@Cacheable` implements this automatically. The method body is only executed on a cache miss.

**Two separate caches with different TTLs:**
- `"product"` (30 min): Individual products. Read-heavy, rarely changes → long TTL.
- `"productList"` (3 min): Paginated listings and search. Changes more often (creates/updates) → short TTL to limit staleness.

### 4. Write-Through Invalidation

On writes, caches are kept consistent:
- `updateProduct` → `@CachePut("product")` refreshes the individual entry + `@CacheEvict("productList", allEntries=true)` wipes all list pages
- `deleteProduct` → `@CacheEvict` on both "product" (by key) and "productList" (all entries)
- `createProduct` → `@CacheEvict("productList", allEntries=true)` (new product invalidates any list page)

`allEntries=true` is used on "productList" because any paginated result might include the changed product, and the composite cache key makes it impractical to evict selectively.

### 5. Soft Delete

```java
product.setStatus(ProductStatus.DELETED);  // never DELETE FROM products
```

Products are never hard-deleted. All queries filter `WHERE status = 'ACTIVE'`. Benefits:
- Historical data preserved for audits
- `StockMovement` references stay valid
- Reversible if a product is accidentally deleted

### 6. Audit Logging (Stock Movements)

Every stock state change creates a `StockMovement` row synchronously in the same transaction. This gives an immutable, append-only log for reconciliation. No updates, no deletes to this table — ever.

### 7. Full-Text Search via PostgreSQL GIN Index

```sql
CREATE INDEX idx_products_fts ON products
  USING GIN (to_tsvector('english', name || ' ' || COALESCE(description, '')));
```

`plainto_tsquery` parses the user query into lexemes; the GIN index makes the `@@` match operator fast. No Elasticsearch dependency needed at this scale.

### 8. Repository Pattern

All DB access is behind `JpaRepository` interfaces. `ProductServiceImpl` depends on `ProductRepository` (interface), not the concrete class. This makes unit testing trivial — just mock the repository.

---

## Database Schema (Flyway)

**V1__baseline_schema.sql** — schema only:
- PostgreSQL custom ENUM types: `product_status`, `movement_type`
- Tables: `categories`, `products`, `product_images`, `stock_movements`
- Indexes:
  - `idx_products_category`, `idx_products_seller`, `idx_products_created_at`, `idx_products_status`
  - `idx_products_active_cat` — partial index `WHERE status = 'ACTIVE'` for fast category browsing
  - `idx_products_fts` — GIN full-text index

**V2__seed_products.sql** — data only:
- 5 root categories (Electronics, Clothing, Home & Garden, Books, Sports & Outdoors)
- 14 subcategories
- 200 products distributed across categories; ~85% ACTIVE, ~10% INACTIVE, ~5% DELETED
- 3 seller UUIDs (round-robin)
- Category-specific price ranges (e.g., laptops $499+, books $9+)
- ~400 product images (primary + secondary for even-numbered products)

`flyway.enabled=true`, `ddl-auto=none` — schema is fully owned by Flyway migrations.

---

## Service Interactions

### Who calls product-service

**cart-service** (sync REST, `PRODUCT_SERVICE_URL` env var):
- `GET /api/v1/products/{id}` — validate product exists and get current price before adding to cart
- `POST /api/v1/inventory/{productId}/reserve` — reserve stock when order is confirmed

**order-service** (sync REST):
- `POST /api/v1/inventory/{productId}/reserve` — reserve stock on order creation
- `POST /api/v1/inventory/{productId}/release` — release stock on order cancellation

### What product-service calls

Nothing. It is a leaf service with no outbound HTTP calls.

### Kafka

None currently. `StockMovement` records provide an in-DB audit trail. If async event publishing is added in the future (e.g., "product.stock_reserved" events), it would publish to Kafka from `InventoryServiceImpl`.

---

## Tests

### Unit Tests (no Spring context, Mockito only)

**ProductServiceImplTest** — 30 tests, nested by operation:
- `CreateProduct` (4): saves entity, resolves category, handles null categoryId, attaches images
- `GetProduct` (3): happy path, not found, DELETED product
- `ListProducts` (8+): category filter, specification fallback, default status=ACTIVE, thumbnail from first image, pagination metadata
- `SearchProducts` (4): delegates to repository, empty page, pagination
- `UpdateProduct` (8): not found, access denied, partial update, full update, image replace, image keep
- `DeleteProduct` (3): not found, access denied, status=DELETED (no hard delete)

**InventoryServiceImplTest** — 9 tests:
- `ReserveStock` (4): happy path, insufficient stock, not found, exactly available
- `ReleaseStock` (3): happy path, release > reserved, not found
- `GetStockLevel` (2): correct available, not found

**ProductServiceCacheTest** — 5 tests (Spring context, mocked repos, in-memory cache):
- Cache hit: repository called once, second call from cache
- Cache populate: key exists after first call
- `@CachePut` on update: refreshes cached entry
- `@CacheEvict` on delete: removes key
- Documents that Spring `@Cacheable` has no built-in stampede protection

### Integration Tests (Testcontainers — real Redis + PostgreSQL)

**ProductCacheIntegrationTest** — 15 tests:
- Key format: verifies `"product-service::product::{id}"` prefix
- TTL: product cache = ~1800s, list cache = ~180s (checked via Redis TTL command)
- Serialization round-trip: `OffsetDateTime`, `UUID`, `BigDecimal`, `Enum`, `List<Image>` all survive Redis
- Not-found: exception thrown, nothing written to Redis (null not cached)
- Invalidation: `@CachePut` keeps key with new value; `@CacheEvict` removes key; `allEntries=true` wipes all list keys
- Warmup: after `warmCache()` completes, key is accessible in Redis

**InventoryConcurrencyTest** — 2 tests:
- **10 threads, stock=5**: exactly 5 reservations succeed, 5 fail (InsufficientStockException or OOLFE), final `stockReserved=5`, exactly 5 `RESERVE` movements — no lost updates
- **3 threads, stock=20**: all 3 succeed despite version conflicts — `@Retryable` handles it

### Smoke Test

**ProductServiceApplicationTests**: `contextLoads()` — Spring context initializes without errors.

---

## Application Configuration Summary

```yaml
server.port: 8081

spring.datasource:
  url: jdbc:postgresql://${DB_HOST}:${DB_PORT}/ecommerce_products
  hikari: maxPoolSize=20, minIdle=5, idleTimeout=5min

spring.jpa:
  hibernate.ddl-auto: none       # Flyway owns schema
  dialect: PostgreSQLDialect
  batch_size: 50                  # batch inserts/updates

spring.cache.type: redis
spring.data.redis:
  host: ${REDIS_HOST}
  port: ${REDIS_PORT}

logging:
  org.springframework.cache: TRACE    # see every hit/miss
  com.ecommerce.product_service: DEBUG
```

Test profile overrides `spring.cache.type: simple` (no Redis needed for unit/AOP tests).

---

## Request Flow Examples

### GET /api/v1/products/42

```
ProductController.getProduct(42)
  └─ ProductServiceImpl.getProduct(42)         @Cacheable("product", key="42")
       ├─ [Cache HIT]  → return cached ProductResponse
       └─ [Cache MISS] → productRepository.findByIdAndStatus(42, ACTIVE)
                           ├─ Not found → throw ProductNotFoundException → 404
                           └─ Found → map to ProductResponse → store in Redis (30 min) → return 200
```

### POST /api/v1/inventory/42/reserve (quantity=2, referenceId="order-99")

```
InventoryController.reserve(42, StockReserveRequest{2, "order-99"})
  └─ InventoryServiceImpl.reserveStock(42, 2, "order-99")   @Retryable
       ├─ productRepository.findById(42)
       ├─ check: stockQuantity(10) - stockReserved(3) = 7 >= 2 ✓
       ├─ product.stockReserved = 5
       ├─ productRepository.save(product)
       │    └─ [version conflict] → ObjectOptimisticLockingFailureException
       │         └─ @Retryable kicks in → retry up to 3x with 100ms backoff
       ├─ stockMovementRepository.save({type=RESERVE, qty=2, ref="order-99"})
       └─ return StockResponse{productId=42, stockQuantity=10, stockReserved=5, availableStock=5}
```

### PUT /api/v1/products/42 (X-Seller-Id: seller-uuid)

```
ProductController.updateProduct(42, sellerUUID, UpdateProductRequest)
  └─ ProductServiceImpl.updateProduct(42, sellerUUID, request)
       │   @CachePut("product", key="42") + @CacheEvict("productList", allEntries=true)
       ├─ productRepository.findById(42) → not found → 404
       ├─ productRepository.existsByIdAndSellerId(42, sellerUUID) → false → 403
       ├─ apply partial update (only non-null request fields)
       ├─ productRepository.save(product)
       ├─ @CachePut → Redis["product::42"] = new ProductResponse (30 min TTL)
       ├─ @CacheEvict(allEntries=true) → Redis DELETE product-service::productList::*
       └─ return 200 ProductResponse
```
