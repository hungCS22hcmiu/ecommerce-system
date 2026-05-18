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
- Optimistic-locking inventory with `@Version` + `@Retryable` (3 attempts, 100ms backoff)
- Append-only `stock_movements` audit trail for every reserve/release
- Reviews: create (one per order item), update, delete; recalculates `avg_rating` + `rating_count` synchronously
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
GET /api/v1/products/ai-search?q=comfortable+shoes&limit=10
    │
    ├─ @Cacheable("aiSearch") key={q, limit}  ← 1-min TTL
    │     ├─ Cache HIT → return cached AISearchResponse
    │     └─ Cache MISS
    │           ├─ embeddingClient.embed(q)
    │           │     └─ POST ai-service:8000/embed → float[384]
    │           │        On failure → throw AIServiceException (cache write skipped)
    │           ├─ SET LOCAL ivfflat.probes = 10  ← improves recall at slight latency cost
    │           ├─ productRepository.findIdsBySemanticSimilarity(vectorLiteral, limit)
    │           │     └─ SELECT id, 1 - (embedding <=> ?) AS score
    │           │          FROM products WHERE status='ACTIVE'
    │           │          ORDER BY embedding <=> ? LIMIT ?
    │           ├─ productRepository.findAllById(ids) → load full entities in ID order
    │           └─ Return AISearchResponse{query, results[], scores[], mode="ai"}
```

### Reserve Stock (the critical path)
```
POST /api/v1/inventory/{productId}/reserve  body:{quantity, referenceId}
    │
    ├─ @Retryable(ObjectOptimisticLockingFailureException, maxAttempts=3, delay=100ms)
    │
    ├─ productRepository.findById(productId)       ← loads current @Version value
    ├─ if stockQuantity - stockReserved < quantity → throw InsufficientStockException → 409
    ├─ product.stockReserved += quantity
    ├─ productRepository.save(product)
    │     └─ Hibernate: UPDATE products SET stock_reserved=?, version=? WHERE id=? AND version=?
    │           ├─ version matches → committed ✓
    │           └─ version mismatch → ObjectOptimisticLockingFailureException
    │                 └─ @Retryable → reload product fresh, retry (up to 3×)
    ├─ stockMovementRepository.save(RESERVE movement)
    └─ Return StockResponse{stockQuantity, stockReserved, availableStock}
```

### Create Review (with seller notification)
```
POST /api/v1/products/{productId}/reviews  Authorization: Bearer <JWT>
    body:{orderItemId, rating, comment?}
    │
    ├─ Validate: product must be ACTIVE; rating 1–5
    ├─ Check UNIQUE(userId, orderItemId) → 409 ALREADY_REVIEWED if exists
    ├─ Verify orderItemId belongs to userId (order-service or product purchase check)
    ├─ Persist Review{productId, userId, orderItemId, rating, comment}
    │
    ├─ recalculateAndEvict(product):
    │     ├─ SELECT AVG(rating), COUNT(*) FROM reviews WHERE product_id=?
    │     ├─ product.avgRating = avg; product.ratingCount = count
    │     ├─ productRepository.save(product)        ← updates @Version too
    │     └─ @CacheEvict("product", key=productId)  ← stale cache cleared
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
    └─ @Async("taskExecutor") — runs in background thread pool, doesn't block startup
          ├─ findTop100ByStatusOrderByUpdatedAtDesc(ACTIVE) — one DB query
          └─ For each product: getProduct(id) → triggers @Cacheable → writes to Redis
             Server accepts traffic immediately; top 100 products are warm within ~2 seconds
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
| `ProductServiceImpl` | CRUD, cache annotations, seller ownership checks, `ratedOnly` branching |
| `InventoryServiceImpl` | Stock reserve/release with `@Retryable`; writes `StockMovement` in same TX |
| `ReviewServiceImpl` | Create/update/delete reviews; recalculates `avgRating`/`ratingCount`; calls `OrderServiceClient` |
| `ProductEmbeddingService` | `@Async` write-through: embeds on create/update; failures logged WARN |
| `AISearchServiceImpl` | Query embed → pgvector → ranked results; cached 1min; skips cache on `AIServiceException` |
| `OrderServiceClient` | Fire-and-forget HTTP to order-service; catches all exceptions; never rethrows |
| `CacheWarmupService` | `@EventListener(ApplicationReadyEvent)` — warms Redis on startup |
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
  version         INT DEFAULT 0       ← @Version field — optimistic lock vector
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

### Optimistic locking — the version check in Hibernate

```java
// InventoryServiceImpl.java
@Retryable(
    retryFor = ObjectOptimisticLockingFailureException.class,
    maxAttempts = 3,
    backoff = @Backoff(delay = 100)
)
@Transactional
public StockResponse reserveStock(Long productId, int qty, String referenceId) {
    Product p = productRepository.findById(productId)
        .orElseThrow(() -> new ProductNotFoundException(productId));

    if (p.getStockQuantity() - p.getStockReserved() < qty) {
        throw new InsufficientStockException(productId, qty);
    }

    p.setStockReserved(p.getStockReserved() + qty);
    productRepository.save(p);
    // Hibernate generates:
    //   UPDATE products SET stock_reserved=5, version=6 WHERE id=42 AND version=5
    // If another thread already committed version 6 → Hibernate throws OOLF → @Retryable

    stockMovementRepository.save(StockMovement.of(productId, RESERVE, qty, referenceId));
    return toStockResponse(p);
}
```

### Review creation with rating recalculation and fire-and-forget notification

```java
// ReviewServiceImpl.java
@Transactional
public ReviewResponse createReview(Long productId, UUID userId, CreateReviewRequest req) {
    Product product = productRepository.findById(productId)
        .orElseThrow(() -> new ProductNotFoundException(productId));

    if (reviewRepository.existsByUserIdAndOrderItemId(userId, req.getOrderItemId())) {
        throw new AlreadyReviewedException();
    }

    Review review = Review.builder()
        .productId(productId).userId(userId).orderItemId(req.getOrderItemId())
        .rating(req.getRating()).comment(req.getComment()).build();
    reviewRepository.save(review);

    recalculateAndEvict(product);  // UPDATE avg_rating, rating_count + cache evict

    // Fire-and-forget: seller gets notified, but review creation never fails because of this
    String notifBody = req.getRating() + "/5 stars"
        + (req.getComment() != null && !req.getComment().isBlank()
           ? " — " + req.getComment().substring(0, Math.min(req.getComment().length(), 80)) : "");
    orderServiceClient.notifySellerReview(
        product.getSellerId(), product.getId(),
        "New review on " + product.getName(), notifBody);

    return toResponse(review);
}

private void recalculateAndEvict(Product product) {
    ReviewStats stats = reviewRepository.getStats(product.getId());
    product.setAvgRating(stats.avg());
    product.setRatingCount(stats.count());
    productRepository.save(product);
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

### Full-text search with GIN index

```java
// ProductRepository.java
@Query("""
    SELECT p FROM Product p
    WHERE p.status = 'ACTIVE'
      AND to_tsvector('english', p.name || ' ' || COALESCE(p.description, ''))
          @@ plainto_tsquery('english', :query)
    ORDER BY ts_rank(
        to_tsvector('english', p.name || ' ' || COALESCE(p.description, '')),
        plainto_tsquery('english', :query)) DESC
    """)
Page<Product> searchActive(@Param("query") String query, Pageable pageable);
// GIN index on tsvector makes @@ operator O(log N + results) instead of O(N)
```

---

## 6. Trade-offs

### Optimistic vs. pessimistic locking for inventory

| | Optimistic (`@Version`) | Pessimistic (`SELECT FOR UPDATE`) |
|---|---|---|
| Read performance | Reads never block | Readers wait while a writer holds the lock |
| Write performance | Fast when contention is low | Consistent cost regardless of contention |
| Failure mode | Retries exhaust → 409 Conflict | Lock wait timeout → slow failure |
| **Our choice** | ✅ Inventory: mostly reads, moderate writes | Order state: catastrophic to lose a transition |

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

### Q: Why optimistic locking for inventory instead of pessimistic?

**A:** The catalog is read-heavy — hundreds of reads per write in a browsing pattern. Pessimistic locking (`SELECT FOR UPDATE`) holds a DB row lock for the full duration of the reservation update, blocking concurrent reads. Optimistic locking only checks for conflicts at commit time — reads are never blocked. The cost is retries on version conflicts, which resolve in 1–2 attempts under normal concurrency. Flash sale scenarios (thousands of concurrent reservations) would need a queue or Redis atomic decrement.

### Q: What happens when all 3 `@Retryable` attempts are exhausted?

**A:** Spring Retry re-throws the last `ObjectOptimisticLockingFailureException`. `GlobalExceptionHandler` maps it to 409 Conflict. The caller (order-service) treats 409 from the inventory endpoint as `InsufficientStockException` and triggers the compensation path: it releases any already-reserved items and marks the order `PAYMENT_FAILED`. No stock is orphaned in a reserved state.

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
