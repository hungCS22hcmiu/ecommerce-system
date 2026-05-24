# product-service: How It Works

---

## 1. What Is It?

The `product-service` is a Java/Spring Boot microservice that owns the **product catalog, inventory, reviews/ratings, and AI-powered semantic search** for the entire ecommerce platform.

**Analogy:** Think of it as a warehouse with a reservation desk, a suggestion engine, and a feedback board. The warehouse shelves (catalog) let anyone browse what's available — served from a fast display case (Redis cache) rather than the back stockroom. The reservation desk (inventory) handles stock locks before orders ship. The suggestion engine (ai-service sidecar) understands natural language — "comfortable shoes for long walks" finds the right products even without exact keyword matches. The feedback board (reviews) collects customer ratings and pins the running average next to every product.

**Responsibilities:**
- Product CRUD with seller ownership enforcement via `X-Seller-Id` header
- Full-text search across names and descriptions (PostgreSQL GIN index, `plainto_tsquery`)
- AI semantic search via pgvector IVFFLAT index + ai-service embedding sidecar
- Write-through async embedding on every product create/update
- Redis cache-aside: 30-min per-product TTL, 3-min paginated list TTL, 1-min AI search TTL
- Async cache warmup on startup — top 100 active products pre-loaded before traffic arrives
- Conditional native UPDATE for inventory (single atomic SQL, no `@Version` retry loop — reads are never blocked)
- `StockProjection` interface used after stock mutations to read new levels without loading a managed entity (prevents spurious `@Version` increment under concurrent load)
- Append-only `stock_movements` audit trail for every reserve/release; cache-evicts the product entry on each mutation
- Reviews: purchase verification against order-service (synchronous, blocking) before saving; duplicate enforced via DB `UNIQUE` + `DataIntegrityViolationException`; recalculates `avg_rating` + `rating_count` synchronously and evicts both `product` and `productList` caches
- Fire-and-forget review notification to order-service (seller receives in-app alert on new review)
- Public seller shop endpoint — paginated ACTIVE product listing scoped to one seller

---

## 2. Why It Matters

### In this project
- Cart-service validates price and existence here synchronously before adding items. Order-service reserves and releases stock here as part of the distributed saga.
- This service was originally a leaf node (calls nobody else). After adding reviews, it now makes two outbound calls: the **ai-service** (for embeddings) and the **order-service** (for review notifications). Both are fire-and-forget — failures are logged as WARN and never surface to the caller.
- The `avg_rating` and `rating_count` fields are denormalized onto `products` rather than computed at query time. This allows the existing product listing and search queries to return rating data without aggregate subqueries — critical for "Highest Rated" seller sort and for surfacing star ratings on every product card.
- The cache-aside layer absorbs the read traffic that dominates a catalog workload. Without it, every product page and search hits Postgres, which cannot sustain high browse concurrency.
- The `@Version` column is the correctness guarantee for inventory. Without it, two concurrent reservations of the last unit read `available=1`, both pass the check, and both decrement — overselling by one.

### In real-world systems
- Shopify, Amazon, and every high-scale catalog use cache-aside (or cache-on-write) to move product reads off the primary DB onto distributed caches.
- Optimistic locking is the standard choice for inventory at moderate contention: it keeps reads unblocked (no shared locks) and only serializes at the moment of conflict.
- PostgreSQL full-text search with `tsvector` / GIN index handles catalogs up to millions of products at sub-20ms latency — no Elasticsearch deployment, no sync pipeline.
- Denormalizing aggregate ratings onto the parent table is the standard approach for high-traffic catalog services — Amazon product ratings work exactly this way.

---

## 3. How It Works — Step-by-Step Flows

### Create Product
```
POST /api/v1/products  (X-Seller-Id: <UUID>)
    │
    ├─ @Valid validates CreateProductRequest fields
    ├─ Resolve category (findById → 404 if provided but missing)
    ├─ Persist Product (status=ACTIVE, stockReserved=0, version=0, avgRating=null, ratingCount=0)
    ├─ Persist ProductImage rows (ordered by sortOrder)
    ├─ @CacheEvict("productList", allEntries=true) ← new product invalidates all list pages
    ├─ @Async: ProductEmbeddingService.scheduleEmbedding(product)
    │     └─ Embed name+description+categoryName → POST ai-service:8000/embed
    │        → UPDATE products SET embedding=? WHERE id=?
    │        (failure → log WARN, never thrown to caller)
    └─ Return ProductResponse (201)
```

### Get Single Product (cache-aside hot path)
```
GET /api/v1/products/{id}
    │
    ├─ @Cacheable("product") key=id
    │     ├─ Cache HIT  → return Redis value, zero DB queries (TTL=30min)
    │     └─ Cache MISS
    │           ├─ productRepository.findByIdAndStatus(id, ACTIVE)
    │           ├─ Found → map to ProductResponse (includes avgRating, ratingCount) → write to Redis → return 200
    │           └─ Not found → throw ProductNotFoundException → 404
    │                          (404 is NOT cached — prevents negative cache poisoning)
    └─ GlobalExceptionHandler maps exception → ApiResponse.error(...)
```

### AI Semantic Search
```
GET /api/v1/products/ai-search?q=comfortable+shoes&limit=10[&categoryId=5][&sellerId=<UUID>]
    │
    ├─ @Cacheable("aiSearch") key={q, limit, categoryId, sellerId}  ← 1-min TTL
    │     unless = result.results().isEmpty()  ← never cache empty (async embedding lag)
    │     ├─ Cache HIT → return cached AISearchResponse
    │     └─ Cache MISS
    │           ├─ embeddingClient.embed(q)
    │           │     └─ POST ai-service:8000/embed → float[384]
    │           │        On failure → throw AIServiceException (cache write skipped)
    │           ├─ SET LOCAL ivfflat.probes = 10  ← improves recall at slight latency cost
    │           ├─ findIdsBySemanticSimilarity(vectorLiteral, categoryId, limit)
    │           │     └─ (or findIdsBySemanticSimilarityBySeller if sellerId set)
    │           │        SELECT id, 1-(embedding <=> CAST(:vec AS vector)) AS score
    │           │        FROM products WHERE status='ACTIVE' [AND category_id=?] [AND seller_id=?]
    │           │        ORDER BY embedding <=> CAST(:vec AS vector) LIMIT ?
    │           ├─ productRepository.findAllById(ids) → load full entities
    │           ├─ Re-rank: score = sim*0.75 + (avgRating/5)*0.15 + (stockReserved/max)*0.10
    │           │     (similarity 75%, rating boost 15%, sales-popularity boost 10%)
    │           └─ Return AISearchResponse{query, results[], scores[], mode="ai"}
    │              (latency logged: embed_ms / vector_ms / rerank_ms)
```

### Reserve Stock (the critical path)
```
POST /api/v1/inventory/{productId}/reserve  body:{quantity, referenceId}
    │
    ├─ @CacheEvict("product", productId) ← any stock change invalidates the cached product
    │
    ├─ productRepository.reserveStockConditional(productId, quantity)
    │     └─ UPDATE products SET stock_reserved = stock_reserved + :qty
    │          WHERE id = :id AND (stock_quantity - stock_reserved) >= :qty
    │          (@Modifying clearAutomatically=true — subsequent reads see fresh DB state)
    │          Returns 1 (success) or 0 (insufficient stock or product not found)
    │
    ├─ updated == 0:
    │     ├─ findById → not found → ProductNotFoundException → 404
    │     └─ available < qty → InsufficientStockException → 409
    │        (StockContentionException is theoretically unreachable — safety belt only)
    │
    ├─ stockMovementRepository.save(RESERVE movement)
    │
    ├─ productRepository.findStockById(productId) → StockProjection (read-only interface)
    │     └─ SELECT stockQuantity, stockReserved FROM Product WHERE id=?
    │        Loads only 2 fields — does NOT create a managed entity, so Hibernate
    │        cannot trigger a dirty-check UPDATE or increment @Version
    │
    └─ Return StockResponse{productId, stockQuantity, stockReserved, availableStock}
```

### Create Review (with purchase verification and seller notification)
```
POST /api/v1/products/{productId}/reviews  Authorization: Bearer <JWT>
    body:{orderItemId, rating, comment?}
    │
    ├─ Validate: product must be ACTIVE; filter(status == ACTIVE).orElseThrow()
    │
    ├─ orderServiceClient.verifyPurchase(customerId, productId, orderItemId)  ← BLOCKING call
    │     └─ GET order-service:8082/api/v1/orders/purchase-verification?productId=&orderItemId=
    │          header: X-User-Id: {customerId}
    │        → false → throw PurchaseNotVerifiedException → 403
    │        → exception → throw PurchaseVerificationException → 503
    │
    ├─ reviewRepository.save(review)
    │     └─ UNIQUE(customer_id, order_item_id) violated → DataIntegrityViolationException
    │           → caught → throw AlreadyReviewedException → 409
    │        (duplicate enforced at DB level, not via a pre-check query)
    │
    ├─ recalculateAndEvict(product):
    │     ├─ findAvgRating(productId) → SELECT AVG(rating) FROM reviews WHERE product_id=?
    │     ├─ countByProductId(productId) → SELECT COUNT(*) FROM reviews WHERE product_id=?
    │     ├─ product.avgRating = avg; product.ratingCount = count
    │     ├─ productRepository.save(product)
    │     ├─ cacheManager.getCache("productList").clear()    ← all list pages stale
    │     └─ cacheManager.getCache("product").evict(productId)  ← single product stale
    │
    ├─ orderServiceClient.notifySellerReview(sellerId, productId, title, body)
    │     └─ POST order-service:8082/api/v1/orders/notifications/internal/review
    │        body:{sellerId, productId, "New review on {name}", "{rating}/5 stars — {comment[:80]}"}
    │        try { restTemplate.postForEntity(...) }
    │        catch (Exception e) { log.warn("...") }   ← NEVER rethrows; review already saved
    │
    └─ Return ReviewResponse (201)
```

### Seller "Highest Rated" Product List
```
GET /api/v1/products?sellerId=<UUID>&ratedOnly=true&sort=avgRating,DESC
    │
    ├─ sellerId != null → productService.listProductsBySeller(sellerId, status, ratedOnly=true, pageable)
    │
    └─ ProductServiceImpl.listProductsBySeller:
          if (ratedOnly && status != null):
            → findBySellerIdAndStatusAndRatingCountGreaterThan(sellerId, status, 0, pageable)
          if (ratedOnly):
            → findBySellerIdAndRatingCountGreaterThan(sellerId, 0, pageable)
          ← Spring Data derives WHERE seller_id=? AND rating_count > 0
          ← Pageable sort=avgRating,DESC applied as ORDER BY
```

### Cache Warmup (startup)
```
ApplicationReadyEvent fires (after Spring context is fully ready)
    │
    ├─ warmCache() — @Async("taskExecutor")
    │     ├─ findTop100ByStatusOrderByUpdatedAtDesc(ACTIVE) — one DB query
    │     └─ For each product: getProduct(id) → triggers @Cacheable → writes to Redis
    │        Server accepts traffic immediately; top 100 products warm within ~2 seconds
    │
    └─ warmAI() — @Async("taskExecutor")
          ├─ Runs 3 representative AI queries: "laptop", "shoes", "coffee maker"
          └─ Each triggers @Cacheable("aiSearch") → warms pgvector planner + seeds Redis
             (failures logged WARN and skipped — doesn't block startup)
```

---

## 4. System Design — Components & Architecture

```
                    ┌─────────────────────────────────────────────────────────────────┐
                    │                     product-service                               │
                    │                                                                   │
  HTTP ─────────────┤  ProductController  InventoryController  ReviewController        │
  (X-Seller-Id hdr) │       │                   │                  │                  │
                    │  ProductServiceImpl  InventoryServiceImpl  ReviewServiceImpl     │
                    │    @Cacheable           @Retryable          recalculate +        │
                    │    @CachePut            @Transactional      notifySellerReview   │
                    │    @CacheEvict               │                  │                │
                    │       │                      │                  │                │
                    │  ProductRepo  CategoryRepo  StockMovementRepo  ReviewRepo        │
                    │       │                                                           │
                    │  ProductEmbeddingService   EmbeddingClient   OrderServiceClient  │
                    │    (@Async write-through)  (HTTP to ai-svc)  (fire-and-forget)  │
                    └──────┬──────────────────────┬───────────────────────┬────────────┘
                           │                      │                       │
            ┌──────────────┴──────┐   ┌───────────┴──────────┐  ┌────────┴──────────────┐
            │     PostgreSQL       │   │       Redis           │  │  ai-service:8000       │
            │                     │   │                       │  │  POST /embed           │
            │ products (@Version  │   │ product::{id}  30min  │  └────────────────────────┘
            │   avgRating,        │   │ productList::* 3min   │
            │   ratingCount)      │   │ aiSearch::*    1min   │  ┌────────────────────────┐
            │ categories (tree)   │   │ prefix: product-      │  │  order-service:8082     │
            │ product_images      │   │   service::           │  │  POST /notifications/  │
            │ stock_movements     │   └───────────────────────┘  │    internal/review     │
            │   (append-only)     │                              └────────────────────────┘
            │ reviews             │
            └─────────────────────┘
```

### Key components

| Component | Role |
|---|---|
| `ProductServiceImpl` | CRUD, cache annotations (`@Cacheable`/`@CachePut`/`@CacheEvict`), seller ownership checks, `ratedOnly` branching, `getProduct(id, sellerId)` overload for seller-view |
| `InventoryServiceImpl` | Conditional native UPDATE (no `@Retryable`); `StockProjection` read-back; evicts product cache on every mutation |
| `ReviewServiceImpl` | Purchase verification (blocking); duplicate via `DataIntegrityViolationException`; recalculates ratings; evicts `product` + `productList`; fire-and-forget notify |
| `ProductEmbeddingService` | `@Async` write-through: embeds on create; re-embeds on update only when name/description/category changes |
| `AISearchServiceImpl` | Query embed → pgvector → re-rank (sim 75% + rating 15% + sales 10%); cached 1min; supports `categoryId` + `sellerId` filters |
| `OrderServiceClient` | `verifyPurchase()` (blocking, throws on error); `notifySellerReview()` (fire-and-forget, logs WARN) |
| `CacheWarmupService` | `@EventListener(ApplicationReadyEvent)` — `warmCache()` (top 100 products) + `warmAI()` (3 representative queries) |
| `CorrelationFilter` | `OncePerRequestFilter` — reads/generates `X-Correlation-ID`; injects into MDC for structured logging |
| `RedisConfig` | `RedisCacheManager` with per-cache TTLs, Jackson JSON serializer |
| `GlobalExceptionHandler` | Maps all domain exceptions to `ApiResponse.error()` envelopes |

### Data model

```
products
  id              BIGSERIAL PK
  seller_id       UUID NOT NULL
  name            VARCHAR NOT NULL
  price           NUMERIC(12,2)
  stock_quantity  INT DEFAULT 0
  stock_reserved  INT DEFAULT 0       ← availableStock = stock_quantity - stock_reserved
  avg_rating      DECIMAL(3,2)        ← denormalized; recalculated on every review mutation
  rating_count    INT DEFAULT 0       ← total reviews; ratedOnly filter uses > 0
  version         BIGINT DEFAULT 0    ← @Version field (Long in Java) — Hibernate optimistic lock; incremented by Hibernate on any entity save
  status          product_status      ← ACTIVE | INACTIVE | DELETED (soft delete)
  embedding       vector(384)         ← pgvector; IVFFLAT index (cosine, lists=100)
  search_vector   TSVECTOR            ← GIN-indexed for plainto_tsquery

reviews
  id              UUID PK
  product_id      BIGINT FK → products
  user_id         UUID NOT NULL
  order_item_id   UUID NOT NULL
  rating          SMALLINT (1–5)
  comment         TEXT (nullable)
  UNIQUE (user_id, order_item_id)     ← one review per order item enforced at DB level

stock_movements (append-only)
  product_id      BIGINT FK
  movement_type   movement_type       ← IN | OUT | RESERVE | RELEASE
  quantity        INT
  reference_id    VARCHAR             ← order ID that caused this change
```

### Cache key convention
```
product-service::product::42          ← single product by ID (30 min)
product-service::productList::...     ← composite: page + size + filters (3 min)
product-service::aiSearch::...        ← composite: query + limit (1 min); never written on AIServiceException
```

### Flyway migrations
| Version | What |
|---|---|
| V1 | `baseline_schema.sql` — tables, indexes, enums |
| V2 | `seed_products.sql` — 19 categories, 200 products |
| V3 | `add_product_embeddings.sql` — `embedding vector(384)` + IVFFLAT index |
| V4 | `add_reviews_and_ratings.sql` — `reviews` table + `avg_rating`/`rating_count` on products |

---

## 5. Code Examples

### Conditional native UPDATE — atomic stock reservation

```java
// InventoryServiceImpl.java
@Override
@Transactional
@CacheEvict(value = "product", key = "#productId")
public StockResponse reserveStock(Long productId, int quantity, String referenceId) {
    // Single atomic SQL — no read-modify-write, no version check, no retry loop.
    // The WHERE clause is the guard: both conditions must hold in the same atomic statement.
    int updated = productRepository.reserveStockConditional(productId, quantity);
    //   UPDATE products SET stock_reserved = stock_reserved + :qty
    //   WHERE id = :id AND (stock_quantity - stock_reserved) >= :qty
    //   Returns 1 on success, 0 on insufficient stock or missing product.

    if (updated == 0) {
        Product product = productRepository.findById(productId)
                .orElseThrow(() -> new ProductNotFoundException(productId));
        int available = product.getStockQuantity() - product.getStockReserved();
        if (available < quantity) {
            throw new InsufficientStockException(productId, quantity, available);
        }
        throw new StockContentionException(productId); // safety belt — theoretically unreachable
    }

    stockMovementRepository.save(StockMovement.builder()
            .productId(productId).type(MovementType.RESERVE)
            .quantity(quantity).referenceId(referenceId).build());

    // Read back via StockProjection — NOT findById() — to avoid loading a managed entity.
    // A full entity load in this writable @Transactional triggers Hibernate dirty-check,
    // which increments @Version even though no app code changed the entity.
    // Under concurrent load that causes spurious ObjectOptimisticLockingFailureException.
    StockProjection sp = productRepository.findStockById(productId)
            .orElseThrow(() -> new ProductNotFoundException(productId));
    return new StockResponse(productId, sp.getStockQuantity(), sp.getStockReserved(),
            sp.getStockQuantity() - sp.getStockReserved());
}
```

### Review creation with rating recalculation and fire-and-forget notification

```java
// ReviewServiceImpl.java
@Transactional
public ReviewResponse createReview(Long productId, UUID userId, CreateReviewRequest req) {
    Product product = productRepository.findById(productId)
        .orElseThrow(() -> new ProductNotFoundException(productId));

    // verifyPurchase is a BLOCKING call — throws PurchaseVerificationException (503) on error
    if (!orderServiceClient.verifyPurchase(customerId, productId, req.getOrderItemId())) {
        throw new PurchaseNotVerifiedException(); // 403
    }

    ProductReview review = ProductReview.builder()
        .product(product).customerId(customerId).orderItemId(req.getOrderItemId())
        .rating(req.getRating()).comment(req.getComment()).build();
    try {
        reviewRepository.save(review);
    } catch (DataIntegrityViolationException e) {
        // UNIQUE(customer_id, order_item_id) violated — duplicate review
        throw new AlreadyReviewedException(); // 409
    }

    recalculateAndEvict(product);  // UPDATE avg_rating, rating_count + evict both caches

    // Fire-and-forget: seller gets notified, but review creation never fails because of this
    String notifBody = req.getRating() + "/5 stars"
        + (req.getComment() != null && !req.getComment().isBlank()
           ? " — " + req.getComment().substring(0, Math.min(req.getComment().length(), 80)) : "");
    orderServiceClient.notifySellerReview(
        product.getSellerId(), product.getId(),
        "New review on " + product.getName(), notifBody);

    return toReviewResponse(review);
}

private void recalculateAndEvict(Product product) {
    double avg = reviewRepository.findAvgRating(product.getId()).orElse(0.0);
    long count = reviewRepository.countByProductId(product.getId());
    product.setAvgRating(BigDecimal.valueOf(avg).setScale(2, RoundingMode.HALF_UP));
    product.setRatingCount((int) count);
    productRepository.save(product);
    // Evict both caches — rating change invalidates individual product AND all list pages
    cacheManager.getCache("productList").clear();
    cacheManager.getCache("product").evict(product.getId());
}
```

### `ratedOnly` derived query methods

```java
// ProductRepository.java
// Spring Data generates: WHERE seller_id=? AND rating_count > 0
Page<Product> findBySellerIdAndRatingCountGreaterThan(UUID sellerId, int minCount, Pageable pageable);
Page<Product> findBySellerIdAndStatusAndRatingCountGreaterThan(UUID sellerId, ProductStatus status, int minCount, Pageable pageable);

// ProductServiceImpl.java — branching on ratedOnly flag
public Page<ProductSummaryResponse> listProductsBySeller(
        UUID sellerId, ProductStatus status, boolean ratedOnly, Pageable pageable) {
    if (ratedOnly) {
        return (status != null)
            ? productRepository.findBySellerIdAndStatusAndRatingCountGreaterThan(sellerId, status, 0, pageable)
                .map(this::toSummaryResponse)
            : productRepository.findBySellerIdAndRatingCountGreaterThan(sellerId, 0, pageable)
                .map(this::toSummaryResponse);
    }
    if (status != null) {
        return productRepository.findBySellerIdAndStatus(sellerId, status, pageable).map(this::toSummaryResponse);
    }
    return productRepository.findBySellerId(sellerId, pageable).map(this::toSummaryResponse);
}
```

### Cache-aside with coordinated invalidation on update

```java
// ProductServiceImpl.java
@Cacheable(value = "product", key = "#id")
public ProductResponse getProduct(Long id) {
    return productRepository.findByIdAndStatus(id, ACTIVE)
        .map(this::toResponse)
        .orElseThrow(() -> new ProductNotFoundException(id));
}

@CachePut(value = "product", key = "#id")           // refresh single entry
@CacheEvict(value = "productList", allEntries = true) // nuke all list pages
@Transactional
public ProductResponse updateProduct(Long id, String sellerId, UpdateProductRequest req) {
    if (!productRepository.existsByIdAndSellerId(id, UUID.fromString(sellerId)))
        throw new ProductAccessDeniedException(id, sellerId);
    // ... apply partial update, save, return response
}
```

### Full-text search with GIN index and popularity boost

```java
// ProductRepository.java (native query)
@Query(value = """
    SELECT * FROM products
    WHERE to_tsvector('english', name || ' ' || COALESCE(description, ''))
          @@ plainto_tsquery('english', :query)
      AND status = 'ACTIVE'
      AND (:categoryId IS NULL OR category_id = :categoryId)
    ORDER BY
      ts_rank(to_tsvector('english', name || ' ' || COALESCE(description, '')),
              plainto_tsquery('english', :query))
      * (1.0 + COALESCE(avg_rating, 0)::float / 5.0 * 0.2   -- rating boost (up to +20%)
             + LEAST(stock_reserved, 50)::float / 50.0 * 0.1 -- popularity boost (up to +10%)
        ) DESC
    """, ...)
Page<Product> searchActive(@Param("query") String query,
                           @Param("categoryId") Long categoryId,
                           Pageable pageable);
// ts_rank is the base relevance score; multiplied by a popularity factor so
// better-rated and more-purchased products rank higher on equal text relevance.
// GIN index on the computed tsvector makes @@ O(log N + results) instead of O(N).
// Also: searchActiveBySeller(..., sellerId, ...) for per-seller full-text search.
```

---

## 6. Trade-offs

### Conditional UPDATE vs. optimistic locking vs. pessimistic locking for inventory

| | Conditional UPDATE (our choice) | Optimistic (`@Version` + retry) | Pessimistic (`SELECT FOR UPDATE`) |
|---|---|---|---|
| Reads blocked? | Never | Never | Yes — while writer holds lock |
| Write cost | One statement, always atomic | Load + save + possible retry | Load + lock + save |
| Contention failure | Impossible (atomic) | Retries exhaust → 409 | Lock wait timeout → slow failure |
| Spurious errors | None | `@Version` bumped by Hibernate dirty-check under concurrent load → false 409 | N/A |
| **Our choice** | ✅ Inventory: one SQL, zero contention | Old approach — replaced due to spurious OptimisticLockException | Order state: catastrophic to lose a transition |

The previous `@Version`+`@Retryable` implementation produced false `InsufficientStockException` errors under concurrent load because loading a full entity in a writable `@Transactional` context caused Hibernate to increment `@Version` on dirty-check even when no app code modified the entity. The conditional UPDATE sidesteps this entirely: no entity is loaded for the write path; `StockProjection` is used for the read-back.

### Fire-and-forget vs. synchronous review notification

| | Fire-and-forget (our choice) | Synchronous call |
|---|---|---|
| Review creation failure | Never blocked by notification failure | Notification failure → 500 for reviewer |
| Notification reliability | Best-effort — WARN logged on failure | Guaranteed if transaction commits |
| Coupling | Loose — product-service doesn't know order-service state | Tight — order-service availability affects product-service |
| **Our choice** | ✅ Notification is not core to review creation | Fine if notification is as important as the primary action |

A seller missing a notification is a minor inconvenience. A customer unable to leave a review because the notification endpoint is down is a worse outcome. Fire-and-forget keeps the critical path (save review + recalculate rating) isolated from the non-critical path (notify seller).

### Denormalized `avg_rating` vs. computed aggregate

| | Denormalized on `products` (our choice) | Computed with `AVG()` subquery |
|---|---|---|
| Read performance | O(1) — field already in the row | O(reviews) per product per query |
| Write performance | Extra UPDATE on every review mutation | No extra write |
| Consistency | Recalculated synchronously within same TX | Always real-time |
| **Our choice** | ✅ Catalog reads dominate; ratings change rarely | Acceptable only for small, read-light catalogs |

### Short (3-min) TTL for product lists

Accepts brief staleness in exchange for a high cache hit rate on paginated listing pages. `allEntries=true` eviction on writes is a best-effort pre-emptive flush; the TTL is the safety net.

### Skip aiSearch cache on `AIServiceException`

If ai-service is down, the fallback result (whatever is returned) should never be cached — the next request should try again when the service recovers. A cached bad result would persist for 1 minute, degrading all semantic search results in that window.

---

## 7. When to Use / Avoid

### Use this pattern when:
- **Read-heavy catalog** (browse >> writes): cache-aside captures most requests without cache thrash.
- **Moderate inventory contention** (tens of concurrent reservations): `@Retryable` handles version conflicts without serializing readers.
- **Review volume is low relative to reads**: denormalized `avg_rating` stays correct with synchronous recalculation.
- **Non-critical cross-service side effects**: fire-and-forget notifications decouple primary action from secondary effects.

### Avoid when:
- **Flash sales / high-burst reservations**: optimistic retry storms exhaust the retry budget; use a Redis `DECR` atomic counter or a reservation queue instead.
- **Real-time inventory accuracy on listing pages**: the 3-min list cache means sold-out items remain visible briefly.
- **Fuzzy / autocomplete search**: `plainto_tsquery` doesn't handle typos; use Elasticsearch or a dedicated search service.
- **Notification is a contractual guarantee**: if sellers must receive every review notification (e.g., SLA), use Kafka with a consumer group instead of HTTP fire-and-forget.

---

## 8. Interview Insights

### Q: Why a conditional native UPDATE for inventory instead of optimistic or pessimistic locking?

**A:** The original design used `@Version` + `@Retryable`. Under concurrent load it produced spurious `ObjectOptimisticLockingFailureException` errors: loading the full `Product` entity in a writable `@Transactional` context causes Hibernate's dirty-check to increment `@Version` even when no application code modifies the entity. After 3 failed retries, valid reservation requests were rejected as `InsufficientStockException`.

The conditional UPDATE solves this entirely. `UPDATE products SET stock_reserved = stock_reserved + :qty WHERE id = :id AND (stock_quantity - stock_reserved) >= :qty` is a single atomic SQL statement — the availability check and the increment happen in the same operation, invisible to any concurrent transaction until it commits. No entity is loaded for the write path; `StockProjection` is used only to read back the new levels (avoiding a managed entity in the same transaction). Read-only paths (browse, search) are unaffected.

Pessimistic locking (`SELECT FOR UPDATE`) would work but serializes concurrent readers while a write is in progress — unacceptable for a read-heavy catalog.

### Q: What happens if the conditional UPDATE returns 0?

**A:** Zero rows updated means either the product doesn't exist or there's insufficient stock. The code disambiguates by calling `findById`: not found → 404; available < qty → `InsufficientStockException` → 409. `StockContentionException` is thrown only as a safety belt for a theoretically unreachable branch (updated == 0 but available >= qty). The caller (order-service) treats 409 from the reserve endpoint as insufficient stock and triggers saga compensation: marks the order `PAYMENT_FAILED` and releases any previously reserved items.

### Q: Why denormalize `avg_rating` onto `products` instead of computing it at query time?

**A:** Product listing queries return pages of 20 products. If `avg_rating` were computed via `AVG(rating) GROUP BY product_id`, every listing would trigger an aggregate join across potentially thousands of review rows. Denormalization trades write overhead (one extra UPDATE per review) for read speed (zero aggregate at query time). Reviews are written rarely compared to reads, so the trade-off is very favorable. Recalculation happens synchronously within the same transaction that saves the review — so the value is always consistent when the transaction commits.

### Q: Why use fire-and-forget for the review notification instead of Kafka?

**A:** The notification is a side effect of the primary action (saving a review). If we used Kafka, we'd need a topic, a consumer group in order-service, and handling for at-least-once delivery. For a single internal HTTP call that takes under 50ms when successful, fire-and-forget via RestTemplate is simpler and operationally lighter. The key property is that failure of the notification must never fail the review creation — catching all exceptions and logging WARN achieves that. If notification reliability became a hard requirement (SLA, compliance), Kafka would be the right tool.

### Q: How does `ratedOnly=true` work? Why not just sort by `avgRating DESC` and filter at the presentation layer?

**A:** Sorting by `avgRating DESC` would put products with `avgRating=null` (no reviews) last — but Postgres sorts `NULL` last in `DESC` order only when using `NULLS LAST`. Even with `NULLS LAST`, the results would include unrated products, and the pagination metadata would be wrong (total count includes unrated products). The `ratedOnly=true` filter with `rating_count > 0` at the database level gives correct pagination and the right total count. The frontend then gets "exactly N rated products" rather than "N total products with most unrated ones hidden."

### Q: The seller ID comes from an HTTP header, not a JWT. Is that secure?

**A:** In this architecture, yes — the header is injected by the Nginx gateway (trusted internal boundary), and the service is not directly reachable from outside. In production you'd strengthen this with: (1) mTLS between gateway and services so only the signed gateway can set the header, or (2) a signed JWT claim that the service verifies independently. The pattern — trusting a gateway-set header on a private network — is standard in internal service meshes where the perimeter is the trust boundary.

### Q: How does the GIN index accelerate full-text search?

**A:** A B-tree index can't answer "which rows contain the word 'running'?" efficiently because it indexes entire values. A GIN (Generalized Inverted Index) pre-builds an inverted index: each stemmed lexeme maps to the list of row IDs containing it. `plainto_tsquery('running shoes')` becomes `'run' & 'shoe'` after English stemming, and the GIN index returns the intersection of both posting lists in O(log N + result count) — essentially the same algorithm Lucene uses.
