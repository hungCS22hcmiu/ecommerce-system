# product-service

Java 21 / Spring Boot 3.5 microservice for product catalog and inventory management. Part of a 5-service e-commerce platform.

- **Port:** 8081
- **Database:** PostgreSQL (`ecommerce_products`)
- **Cache:** Redis (cache-aside, TTL-based)
- **Concurrency:** Optimistic locking (`@Version`) + `@Retryable`

---

## Quick Start

```bash
# From repo root — start infrastructure
docker compose up -d postgres redis

# Run locally
cd product-service
./mvnw spring-boot:run

# Or build and run via Docker
docker compose build product-service
docker compose up -d product-service
```

Health check: `GET http://localhost:8081/health/live` → `{ "status": "UP" }`

The database schema and seed data (200 products, 19 categories) are applied automatically by Flyway on first start.

---

## API Reference

All responses follow the envelope format:
```json
{ "success": true, "data": { ... } }
{ "success": true, "data": [...], "meta": { "page": 0, "size": 20, "totalElements": 150, "totalPages": 8 } }
{ "success": false, "error": { "code": "PRODUCT_NOT_FOUND", "message": "..." } }
```

### Products — `/api/v1/products`

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/` | `X-Seller-Id` header (UUID) | Create product → 201 |
| `GET` | `/{id}` | None | Get product by ID (ACTIVE only) |
| `GET` | `/` | None | List products — paginated |
| `GET` | `/search?q=` | None | Full-text search — paginated |
| `PUT` | `/{id}` | `X-Seller-Id` header (UUID) | Update product (partial) |
| `DELETE` | `/{id}` | `X-Seller-Id` header (UUID) | Soft-delete → 204 |

**List query params:** `categoryId` (Long), `status` (ACTIVE/INACTIVE/DELETED), standard `page`/`size`/`sort`  
**Default page:** size=20, sort=createdAt DESC  
**Missing `X-Seller-Id`** on write endpoints → 400. Wrong seller on owned product → 403.

#### Create / Update request body

```json
{
  "name": "Widget Pro",
  "description": "Optional",
  "price": 49.99,
  "categoryId": 3,
  "stockQuantity": 100,
  "images": [
    { "url": "https://...", "altText": "front view", "sortOrder": 0 }
  ]
}
```

All `UpdateProductRequest` fields are optional (partial update). `images` array replaces all existing images when provided.

### Inventory — `/api/v1/inventory`

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/{productId}/reserve` | Reserve stock for an order |
| `POST` | `/{productId}/release` | Release previously reserved stock |
| `GET` | `/{productId}` | Get current stock levels |

**Reserve / Release body:**
```json
{ "quantity": 5, "referenceId": "order-abc123" }
```

**Stock response:**
```json
{ "productId": 1, "stockQuantity": 100, "stockReserved": 10, "availableStock": 90 }
```

### Error codes

| Code | HTTP | Trigger |
|------|------|---------|
| `PRODUCT_NOT_FOUND` | 404 | ID not found or product is DELETED |
| `INSUFFICIENT_STOCK` | 409 | Reserve quantity > available |
| `ACCESS_DENIED` | 403 | Seller ID doesn't own the product |
| `CONCURRENT_MODIFICATION` | 409 | Optimistic lock retries exhausted |
| `VALIDATION_ERROR` | 400 | Bean validation failure |
| `BAD_REQUEST` | 400 | Missing header, type mismatch, release > reserved |

---

## Data Model

```
categories (hierarchical, self-join)
  └─ id, name, slug (UNIQUE), parent_id, sort_order

products
  ├─ id, name, description, price
  ├─ category_id → categories (SET NULL on delete)
  ├─ seller_id (UUID — no FK, enforced at app level)
  ├─ status: ACTIVE | INACTIVE | DELETED
  ├─ stock_quantity  (total on hand)
  ├─ stock_reserved  (held by pending orders)
  └─ version         (optimistic lock counter)

product_images
  └─ product_id → products (CASCADE DELETE), url, alt_text, sort_order

stock_movements  (append-only audit log)
  └─ product_id, type: IN|OUT|RESERVE|RELEASE, quantity, reference_id, created_at
```

**Stock model:** `availableStock = stockQuantity - stockReserved`  
Reserving does not decrement `stockQuantity` — it increments `stockReserved`. This allows order cancellations to simply release the reservation without touching inventory counts.

**Soft delete:** `DELETE` sets `status = DELETED`. Products are never hard-deleted; foreign keys stay valid and history is preserved.

**Indexes:** GIN index on `to_tsvector('english', name || description)` for full-text search. Partial index on `(category_id, created_at DESC) WHERE status='ACTIVE'` for efficient category listings.

---

## Concurrency — Inventory

Stock operations are the contention point. Two concurrent reservations for the last 5 units must not both succeed.

### Optimistic locking + retry

```
Thread A reads  product: version=0, reserved=0, available=10
Thread B reads  product: version=0, reserved=0, available=10

Thread A writes: reserved=5, version → 1  ✓
Thread B writes: version=0 but DB has v1  → ObjectOptimisticLockingFailureException

@Retryable: Thread B waits 100ms, re-reads (version=1, reserved=5, available=5), retries → success
```

- `@Version Long version` on `Product` — Hibernate increments on every write
- `@Retryable(retryFor = ObjectOptimisticLockingFailureException.class, maxAttempts = 3, backoff = @Backoff(delay = 100))`
- After 3 failed retries → 409 CONCURRENT_MODIFICATION
- Proven by `InventoryConcurrencyTest`: 10 threads competing for 5 units — exactly 5 succeed, no lost updates

**Why optimistic (not pessimistic)?** Product reads far outnumber writes. `SELECT FOR UPDATE` would block every concurrent reader during a stock change. Optimistic locking adds zero overhead on reads.

---

## Caching — Redis Cache-Aside

### Cache configuration

| Cache | TTL | Key | What's stored |
|-------|-----|-----|---------------|
| `product` | 30 min | `product-service::product::{id}` | `ProductResponse` (full detail) |
| `productList` | 3 min | `product-service::productList::{params}` | `Page<ProductSummaryResponse>` |

- Values serialized as JSON (Jackson + `JavaTimeModule` — handles `OffsetDateTime`, `UUID`, `BigDecimal`)
- Null values not cached (`disableCachingNullValues`) — `ProductNotFoundException` never writes to Redis
- Key prefix `product-service::` prevents collisions with other services sharing the same Redis

### Cache operations

| Operation | Annotation | Effect |
|-----------|-----------|--------|
| `getProduct` | `@Cacheable("product")` | Miss → DB + cache; Hit → Redis only |
| `updateProduct` | `@CachePut("product")` + `@CacheEvict("productList")` | Refresh product key immediately; clear all list keys |
| `deleteProduct` | `@CacheEvict` both caches | Remove product key; clear all list keys |
| `createProduct` | `@CacheEvict("productList", allEntries=true)` | Clear all list keys (product count changed) |
| `listProducts` / `searchProducts` | `@Cacheable("productList")` | Short TTL (3 min) handles staleness |

### Startup cache warming

`CacheWarmupService` fires on `ApplicationReadyEvent` (async, non-blocking):
1. Queries the 100 most recently updated ACTIVE products
2. Calls `productService.getProduct(id)` for each → `@Cacheable` populates Redis
3. Logs completion: `[CacheWarmup] Completed: 98 products cached in 245ms`

Reduces cold-start latency for popular products on first requests after deploy.

### Cache stampede (known limitation)

When `productList` TTL expires and many concurrent requests arrive simultaneously, all find a cache miss and hit the DB together. Mitigations (not implemented) include probabilistic early refresh (PER), a distributed lock around the miss path, or a Caffeine + Redis two-tier cache.

### Inspect cache in Redis

```bash
# All cached keys
docker exec ecommerce-redis redis-cli KEYS "product-service::*"

# A product's cached value (human-readable JSON)
docker exec ecommerce-redis redis-cli GET "product-service::product::1"

# TTL remaining (seconds)
docker exec ecommerce-redis redis-cli TTL "product-service::product::1"
```

Cache hit/miss events are logged at TRACE level (`org.springframework.cache: TRACE` in `application.yaml`):
```
TRACE ... No cache entry for key '1' in cache(s) [product]      ← MISS
TRACE ... Cache entry for key '1' found in cache(s) [product]    ← HIT
```

---

## Configuration

| Env var | Default | Description |
|---------|---------|-------------|
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | DB username |
| `DB_PASSWORD` | `postgres` | DB password |
| `REDIS_HOST` | `localhost` | Redis host |
| `REDIS_PORT` | `6379` | Redis port |
| `REDIS_PASSWORD` | _(empty)_ | Redis password (optional) |

Flyway migrations live in `src/main/resources/db/migration/`:
- `V1__baseline_schema.sql` — tables, indexes, PostgreSQL enums
- `V2__seed_products.sql` — 19 categories, 200 products (3 seller IDs, images via picsum.photos)

---

## Project Structure

```
src/main/java/com/ecommerce/product_service/
├── config/
│   ├── AsyncConfig.java       # Thread pool for @Async (core=5, max=20)
│   ├── JpaConfig.java         # @EnableJpaAuditing with OffsetDateTime provider
│   ├── RedisConfig.java       # @EnableCaching, RedisCacheManager, Jackson serializer
│   └── RetryConfig.java       # @EnableRetry
├── controller/
│   ├── HealthController.java
│   ├── InventoryController.java
│   └── ProductController.java
├── dto/                       # Request / response records and classes
├── exception/
│   ├── GlobalExceptionHandler.java
│   ├── InsufficientStockException.java
│   ├── ProductAccessDeniedException.java
│   └── ProductNotFoundException.java
├── model/                     # JPA entities: Product, Category, ProductImage, StockMovement
├── repository/                # Spring Data JPA interfaces
└── service/
    ├── CacheWarmupService.java
    ├── InventoryService.java (interface)
    ├── ProductService.java    (interface)
    └── serviceImpl/
        ├── InventoryServiceImpl.java
        └── ProductServiceImpl.java
```

---

## Testing

```bash
./mvnw test                                           # all 62 tests
./mvnw test -Dtest="ProductServiceImplTest"           # unit only
./mvnw test -Dtest="ProductCacheIntegrationTest"      # cache integration (starts Redis + Postgres containers)
./mvnw test -Dtest="InventoryConcurrencyTest"         # concurrency (starts Postgres container)
```

| File | Type | Count | What it proves |
|------|------|-------|----------------|
| `ProductServiceImplTest` | Unit (Mockito) | 30 | CRUD logic, ownership checks, soft delete, mapping |
| `InventoryServiceImplTest` | Unit (Mockito) | 9 | Stock math, movement audit, edge cases |
| `ProductServiceCacheTest` | Unit + Spring cache | 5 | `@Cacheable`/`@CachePut`/`@CacheEvict` AOP fires correctly |
| `ProductCacheIntegrationTest` | Integration (Testcontainers) | 15 | Real Redis: key format, TTL, serialization round-trip, invalidation |
| `InventoryConcurrencyTest` | Integration (Testcontainers) | 2 | Lost-update prevention: 10 threads, 5 units — exactly 5 succeed |
| `ProductServiceApplicationTests` | Smoke | 1 | Spring context loads |

Integration tests spin up real PostgreSQL and Redis containers automatically — no local setup needed.

Test profile (`application-test.yaml`) uses an in-memory cache by default. `ProductCacheIntegrationTest` overrides this via `@DynamicPropertySource` to connect to the Redis container.

---

## Key Design Decisions

| Decision | Why |
|----------|-----|
| Optimistic locking, not pessimistic | Catalog is read-heavy; `SELECT FOR UPDATE` blocks readers. @Version adds zero overhead on reads. |
| Soft delete | Preserves audit history; FK constraints stay valid; reversible. |
| Cache-aside, not write-through | Redis failure doesn't break writes; app controls cache population timing. |
| productList TTL = 3 min | Pagination key space is huge (every page/size/sort combo). Short TTL prevents memory explosion. |
| Async cache warmup | Startup latency unchanged; pre-warmed cache is a best-effort optimization, not a hard dependency. |
| `stockQuantity` + `stockReserved` dual fields | Separates "total on hand" from "committed to orders"; cancellations release reservation without touching quantity. |
| `X-Seller-Id` in header | Simpler than JWT parsing in this service; gateway pre-validates JWT and forwards the seller ID. |
