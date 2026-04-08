# Phase 3: Redis Caching (Cache-Aside Pattern) — Implementation Plan

## Context
The product-service has full CRUD + search but no caching layer. Adding Redis cache-aside will reduce DB load on read-heavy endpoints. Dependencies (`spring-boot-starter-cache`, `spring-boot-starter-data-redis`) and Redis connection config in `application.yaml` already exist. No `@EnableCaching` annotation or `CacheManager` bean exists yet.

## Key Observation
DTOs (`ProductResponse`, `ProductSummaryResponse`) are not `Serializable` — Redis JSON serialization will need a custom `RedisSerializer` (Jackson) or DTOs must implement `Serializable`. Jackson-based serialization is preferred (no `Serializable` boilerplate, human-readable keys).

## Implementation Steps

### Step 1: Create `RedisConfig` — Redis cache configuration class
**File:** `src/main/java/com/ecommerce/product_service/config/RedisConfig.java` (NEW)

- `@Configuration` + `@EnableCaching`
- Define `RedisCacheManager` bean with:
  - Default TTL: 10 minutes
  - Named cache configs:
    - `"product"` — TTL 30 min (single product, evicted on update/delete)
    - `"productList"` — TTL 3 min (listing/search results, short-lived)
  - Jackson `GenericJackson2JsonRedisSerializer` for values (handles OffsetDateTime, UUID, enums)
  - Disable caching null values
  - Use cache key prefix: `product-service::`

### Step 2: Make DTOs serialization-friendly
No changes needed if using Jackson serializer. But register `JavaTimeModule` in the serializer for `OffsetDateTime` support.

### Step 3: Add cache annotations to `ProductServiceImpl`
**File:** `src/main/java/.../service/serviceImpl/ProductServiceImpl.java`

| Method | Annotation | Cache Name | Key |
|--------|-----------|------------|-----|
| `getProduct(Long id)` | `@Cacheable("product")` | product | `#id` |
| `updateProduct(...)` | `@CachePut("product")` | product | `#id` |
| `deleteProduct(...)` | `@CacheEvict("product")` | product | `#id` |
| `createProduct(...)` | `@CacheEvict(value="productList", allEntries=true)` | productList | all |

For `listProducts` and `searchProducts` — add `@Cacheable("productList")` with a composite key from all parameters. The short TTL (3 min) handles staleness.

**Note on `@CachePut` vs `@CacheEvict` for update:**
- `updateProduct` returns `ProductResponse` — use `@CachePut` so the cache is refreshed with the new value immediately
- Also evict `productList` cache since listings are now stale

**Note on delete:**
- `deleteProduct` returns `void` — use `@CacheEvict` on both `"product"` and `"productList"`

### Step 4: Add cache hit/miss logging
**File:** `src/main/java/.../config/RedisConfig.java`

Add a `CacheInterceptor` or use Spring's built-in logging:
- Set `logging.level.org.springframework.cache=TRACE` in application.yaml for dev
- Alternatively, create a simple `@Aspect` that logs cache hits/misses with product IDs

Simpler approach: add `logging.level.org.springframework.cache: TRACE` to application.yaml and a custom `CacheEventLogger` aspect for cleaner output.

### Step 5: Implement cache warming on startup
**File:** `src/main/java/.../service/CacheWarmupService.java` (NEW)

- `@Component` with `@EventListener(ApplicationReadyEvent.class)`
- Use `@Async("taskExecutor")` (already configured in AsyncConfig)
- Query top 100 products ordered by `updatedAt DESC` (most recently active)
- Call `productService.getProduct(id)` for each — populates the `"product"` cache
- Log start/completion with count and duration

### Step 6: Update application.yaml — cache logging
**File:** `src/main/resources/application.yaml`

Add under `logging.level`:
```yaml
logging:
  level:
    org.springframework.cache: TRACE
    com.ecommerce.product_service.config: DEBUG
```

### Step 7: Add `findTop100ByStatusOrderByUpdatedAtDesc` to ProductRepository
**File:** `src/main/java/.../repository/ProductRepository.java`

Add query method for cache warming:
```java
List<Product> findTop100ByStatusOrderByUpdatedAtDesc(ProductStatus status);
```

### Step 8: Unit tests for caching behavior
**File:** `src/test/java/.../service/ProductServiceCacheTest.java` (NEW)

Test with `@SpringBootTest` + embedded Redis (or mock CacheManager):
- **Cache hit test:** Call `getProduct` twice, verify repository called once
- **Cache invalidation on update:** Call `getProduct`, then `updateProduct`, then `getProduct` — verify fresh data
- **Cache invalidation on delete:** Call `getProduct`, then `deleteProduct`, then `getProduct` — verify cache miss
- **Document cache stampede:** Add a comment/test documenting the scenario (100 concurrent requests on expired key) — observe but don't solve

## Files to Create
1. `src/main/java/com/ecommerce/product_service/config/RedisConfig.java`
2. `src/main/java/com/ecommerce/product_service/service/CacheWarmupService.java`
3. `src/test/java/com/ecommerce/product_service/service/ProductServiceCacheTest.java`

## Files to Modify
1. `src/main/java/com/ecommerce/product_service/service/serviceImpl/ProductServiceImpl.java` — add cache annotations
2. `src/main/java/com/ecommerce/product_service/repository/ProductRepository.java` — add warmup query
3. `src/main/resources/application.yaml` — add cache logging config

## Verification Plan
1. `./mvnw test` — all existing + new tests pass
2. Start service with `docker compose up -d redis postgres` then `./mvnw spring-boot:run`
3. **Cache hit test:** `GET /api/v1/products/1` twice — logs show cache MISS then HIT, DB queried once
4. **Cache invalidation:** `PUT /api/v1/products/1` then `GET` — fresh data returned
5. **Cache warming:** Check startup logs for "Cache warmup completed" message
6. **Redis inspection:** `docker exec ecommerce-redis redis-cli KEYS "product-service::*"` to see cached keys
7. **Cache stampede:** Document in test comments what would happen (all 100 requests hit DB simultaneously when key expires)
