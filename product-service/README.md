# product-service

Java 21 / Spring Boot 3.5 microservice for product catalog, inventory management, reviews/ratings, and AI-powered semantic search. Part of a 5-service e-commerce platform.

- **Port:** 8081
- **Database:** PostgreSQL (`ecommerce_products`)
- **Cache:** Redis (cache-aside, TTL-based)
- **Concurrency:** Conditional native UPDATE for stock (atomic, no retry) + Redis cache-aside

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

The database schema is applied automatically by Flyway on first start. Run `python3 script/seed_products.py` from the repo root to populate 10,000 products across 10 sellers and 52 categories (replaces the Flyway V2 sample seed).

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
| `GET` | `/{id}` | None | Get product by ID |
| `GET` | `/` | None | List products — paginated |
| `GET` | `/search?q=` | None | Full-text keyword search — paginated |
| `GET` | `/ai-search?q=&limit=` | None | pgvector semantic search |
| `PUT` | `/{id}` | `X-Seller-Id` header (UUID) | Update product (partial) |
| `DELETE` | `/{id}` | `X-Seller-Id` header (UUID) | Soft-delete → 204 |

**List query params:** `categoryId` (Long), `sellerId` (UUID), `status` (ACTIVE/INACTIVE/DELETED), `ratedOnly` (boolean — filters `rating_count > 0`), standard `page`/`size`/`sort`.

`ratedOnly=true` is used by the seller "Highest Rated" view — returns only the seller's products that have at least one review, sorted by `avgRating DESC`.

**Missing `X-Seller-Id`** on write endpoints → 400. Wrong seller on owned product → 403.

### Inventory — `/api/v1/inventory`

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/{productId}/reserve` | Reserve stock for an order (internal, nginx blocked externally) |
| `POST` | `/{productId}/release` | Release previously reserved stock (internal) |
| `GET` | `/{productId}` | Get current stock levels |

### Categories — `/api/v1/categories`

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | List all categories (hierarchical) |
| `GET` | `/{id}` | Get category by ID |
| `GET` | `/slug/{slug}` | Get category by slug |

### Reviews — `/api/v1/products/{productId}/reviews`

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/products/{id}/reviews` | Bearer JWT | Create a review (one per order item) |
| `GET` | `/products/{id}/reviews` | None | List reviews for a product (paginated) |
| `GET` | `/products/{id}/my-review?orderItemId=` | Bearer JWT | Get the current user's review for an order item |
| `PUT` | `/reviews/{reviewId}` | Bearer JWT | Update own review |
| `DELETE` | `/reviews/{reviewId}` | Bearer JWT | Delete own review |

On review creation: (1) `orderServiceClient.verifyPurchase()` is called synchronously — if the user did not actually purchase this product via order-service, the review is rejected with 403. (2) Duplicate check is enforced at the DB level via `UNIQUE(customer_id, order_item_id)` — `DataIntegrityViolationException` is caught and mapped to 409. (3) `avg_rating` and `rating_count` are recalculated immediately, and both `product` and `productList` caches are evicted. (4) `orderServiceClient.notifySellerReview()` is called fire-and-forget to notify the seller (logged WARN on failure, never surfaces to caller).

### Seller Public Profile — `/api/v1/sellers`

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/{sellerId}/products` | None | Public product listing for a seller's shop page |

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
  ├─ avg_rating DECIMAL(3,2)   (recalculated on every review create/update/delete)
  ├─ rating_count INT           (total number of reviews)
  └─ version         (optimistic lock counter)

product_images
  └─ product_id → products (CASCADE DELETE), url, alt_text, sort_order

stock_movements  (append-only audit log)
  └─ product_id, type: IN|OUT|RESERVE|RELEASE, quantity, reference_id, created_at

reviews
  ├─ id (UUID), product_id → products
  ├─ user_id (UUID), order_item_id (UUID)  ← one review per order item enforced
  ├─ rating (1–5), comment (nullable)
  └─ created_at / updated_at

products.embedding  vector(384)   ← pgvector, IVFFLAT index (cosine, lists=100)
```

**Stock model:** `availableStock = stockQuantity - stockReserved`. Reserving increments `stockReserved`; cancellations release the reservation without touching `stockQuantity`.

**Soft delete:** `DELETE` sets `status = DELETED`. History preserved; FK constraints stay valid.

---

## Concurrency — Inventory

### Conditional native UPDATE

Stock mutations use a single atomic SQL statement rather than read-modify-write:

```sql
-- Reserve: only decrements if enough stock is available
UPDATE products
SET stock_reserved = stock_reserved + :qty
WHERE id = :id AND (stock_quantity - stock_reserved) >= :qty

-- Release: always safe (cannot go negative)
UPDATE products
SET stock_reserved = stock_reserved - :qty
WHERE id = :id AND stock_reserved >= :qty
```

- Returns 0 rows updated → diagnoses out-of-stock vs. not-found via `findById` → appropriate exception
- No `@Version`, no `@Retryable`, no optimistic-lock retry loop
- Proven by `InventoryConcurrencyTest`: 10 threads competing for 5 units — exactly 5 succeed, 5 get `InsufficientStockException`, no lost updates

**Why conditional UPDATE (not SELECT FOR UPDATE)?** A `SELECT FOR UPDATE` would serialize all concurrent reads during a stock change. The conditional UPDATE is a single round-trip that is atomic at the DB level and adds zero overhead on read-only paths.

---

## Caching — Redis Cache-Aside

| Cache | TTL | What's stored |
|-------|-----|---------------|
| `product` | 30 min | `ProductResponse` (full detail) |
| `productList` | 3 min | `Page<ProductSummaryResponse>` |
| `aiSearch` | 1 min | `AISearchResponse` (skipped on `AIServiceException`) |

- Values serialized as JSON (Jackson + `JavaTimeModule`)
- Null values not cached — `ProductNotFoundException` never writes to Redis
- Key prefix `product-service::` prevents Redis namespace collisions

### Startup cache warming

`CacheWarmupService` fires `@Async` on `ApplicationReadyEvent`: queries the 100 most recent ACTIVE products and pre-populates the `product` cache. Non-blocking; logged on completion.

---

## AI Search — pgvector

**Flow:** query text → `EmbeddingClient.embed()` (HTTP to ai-service) → `SET LOCAL ivfflat.probes=10` → `findIdsBySemanticSimilarity(vector, categoryId, limit)` (or `findIdsBySemanticSimilarityBySeller` when `sellerId` is set) → re-rank: similarity 75% + rating boost 15% + sales-popularity boost 10% → `AISearchResponse{query, results, scores, mode}`. Latency per phase (embed/vector/rerank) is logged at INFO.

**Write-through embedding:** `ProductEmbeddingService.scheduleEmbedding()` fires `@Async` on every `createProduct` / `updateProduct`. Concatenates `name + description + categoryName`, sends to ai-service, stores 384-dim vector in `products.embedding`. Failures logged WARN; never surface to caller. Configurable via `ai-service.write-through-enabled`.

**Backfill:** `docker compose run --rm ai-service python scripts/embed_products.py`

---

## Flyway Migrations

| Version | File | What it does |
|---|---|---|
| V1 | `baseline_schema.sql` | All core tables + indexes |
| V2 | `seed_products.sql` | Sample 19 categories, 200 products (replaced at runtime by `script/seed_products.py`) |
| V3 | `add_product_embeddings.sql` | `embedding vector(384)` + IVFFLAT index (lists=100, cosine ops) |
| V4 | `add_reviews_and_ratings.sql` | `reviews` table + `avg_rating`/`rating_count` on products |

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
| `AI_SERVICE_URL` | `http://ai-service:8000` | ai-service base URL |
| `ORDER_SERVICE_URL` | `http://order-service:8082` | For review notifications |

---

## Project Structure

```
src/main/java/com/ecommerce/product_service/
├── config/
│   ├── AsyncConfig.java            # Thread pool for @Async (core=5, max=20)
│   ├── HttpClientConfig.java       # RestTemplate bean configuration
│   ├── JpaConfig.java              # @EnableJpaAuditing
│   ├── RedisConfig.java            # @EnableCaching, RedisCacheManager, Jackson serializer
│   ├── RestTemplateConfig.java     # RestTemplate with connection/read timeouts
│   └── RetryConfig.java            # @EnableRetry
├── controller/
│   ├── HealthController.java
│   ├── InventoryController.java
│   ├── ProductController.java      # includes ratedOnly param
│   ├── ReviewController.java
│   └── CategoryController.java
├── client/
│   ├── EmbeddingClient.java        # HTTP to ai-service
│   └── OrderServiceClient.java     # verifyPurchase (blocking) + notifySellerReview (fire-and-forget)
├── dto/
│   └── StockProjection.java        # Interface projection: stockQuantity + stockReserved only
├── filter/
│   └── CorrelationFilter.java      # OncePerRequestFilter: reads/generates X-Correlation-ID → MDC
├── model/                          # Product, Category, ProductImage, StockMovement, ProductReview
├── repository/                     # Spring Data JPA; conditional UPDATE queries; StockProjection; ratedOnly methods; seller AI/FTS search variants
└── service/
    ├── CacheWarmupService.java      # warmCache() + warmAI() on ApplicationReadyEvent
    ├── ProductEmbeddingService.java # @Async write-through embedding (only on semantic-change fields)
    ├── AISearchService.java / serviceImpl/AISearchServiceImpl.java  (re-rank: sim+rating+sales)
    ├── ProductService.java / serviceImpl/ProductServiceImpl.java     (ratedOnly branching, seller overload)
    ├── InventoryService.java / serviceImpl/InventoryServiceImpl.java (conditional UPDATE + StockProjection)
    └── ReviewService.java / serviceImpl/ReviewServiceImpl.java       (purchase verify + dual cache evict)
```

---

## Testing

```bash
./mvnw test                                           # all tests
./mvnw test -Dtest="ProductServiceImplTest"           # unit only
./mvnw test -Dtest="ProductCacheIntegrationTest"      # cache integration (Testcontainers)
./mvnw test -Dtest="InventoryConcurrencyTest"         # concurrency (Testcontainers)
```

| File | Type | What it proves |
|------|------|----------------|
| `ProductServiceImplTest` | Unit (Mockito) | CRUD logic, ownership checks, soft delete, mapping |
| `InventoryServiceImplTest` | Unit (Mockito) | Stock math, movement audit, edge cases |
| `ProductServiceCacheTest` | Unit + Spring cache | `@Cacheable`/`@CachePut`/`@CacheEvict` AOP fires correctly |
| `ProductCacheIntegrationTest` | Integration (Testcontainers) | Real Redis: key format, TTL, serialization, invalidation |
| `InventoryConcurrencyTest` | Integration (Testcontainers) | 10 threads, 5 units — exactly 5 succeed (lost-update proof) |
| `AISearchServiceTest` | Unit (Mockito) | AI search logic, fallback, cache skip on exception |
| `AISearchIntegrationTest` | Integration (Testcontainers) | Real pgvector similarity search |
| `AISearchFallbackTest` | Unit | Circuit-breaker / unavailable ai-service scenarios |

---

## Key Design Decisions

| Decision | Why |
|----------|-----|
| Conditional native UPDATE for inventory | Single atomic SQL (no read-modify-write loop); eliminates the spurious `@Version` increment from Hibernate dirty-check that caused false `InsufficientStockException` under concurrent load. |
| `StockProjection` for read-back | Loading a full managed entity after a conditional UPDATE in the same `@Transactional` context triggers Hibernate dirty-check → `@Version` increment → `OptimisticLockException` under concurrent requests. The projection avoids creating a managed entity. |
| Soft delete | Preserves audit history; FK constraints stay valid; reversible. |
| Cache-aside, not write-through | Redis failure doesn't break writes; app controls cache population timing. |
| `productList` TTL = 3 min | Pagination key space is huge. Short TTL prevents memory explosion. |
| `avg_rating` denormalized on products | Avoids aggregate query on every product listing. Recalculated synchronously on review mutation; evicts `product` + `productList` caches. |
| Purchase verification before saving review | Prevents fake reviews from users who never bought the product; synchronous because the review is invalid without it. |
| Review duplicate via `DataIntegrityViolationException` | DB `UNIQUE` constraint is the authoritative guard; catching the exception is simpler and race-condition-free vs. a pre-check + insert. |
| Review notification fire-and-forget | Seller notification is best-effort; review creation should never fail because the notification service is down. |
| `ratedOnly` boolean param | Spring Data derived queries handle `ratingCount > 0` cleanly; the Pageable sort alone cannot exclude zero-rating rows. |
| AI re-ranking (sim 75% + rating 15% + sales 10%) | Pure vector similarity ignores product quality signals; blending with rating and purchase popularity improves relevance for real users. |
