# product-service: How It Works

---

## 1. What Is It?

The `product-service` is a Java/Spring Boot microservice that owns the **product catalog and inventory** for the entire ecommerce platform.

**Analogy:** Think of it as a warehouse with a reservation desk. The warehouse shelves (catalog) let anyone browse what's available — served from a fast display case (Redis cache) rather than the back stockroom. The reservation desk (inventory) handles stock locks before orders ship. The desk uses a version stamp on every shelf slot: if two workers try to grab the last unit simultaneously, exactly one wins and the other is told to retry — no overselling, no DB row locks.

**Responsibilities:**
- Product CRUD with seller ownership enforcement via `X-Seller-Id` header
- Full-text search across names and descriptions (PostgreSQL GIN index, `plainto_tsquery`)
- Redis cache-aside: 30-min per-product TTL, 3-min paginated list TTL
- Async cache warmup on startup — top 100 active products pre-loaded before traffic arrives
- Optimistic-locking inventory with `@Version` + `@Retryable` (3 attempts, 100ms backoff)
- Append-only `stock_movements` audit trail for every reserve/release

---

## 2. Why It Matters

### In this project
- Cart-service validates price and existence here synchronously before adding items. Order-service reserves and releases stock here as part of the distributed saga. This service is a **leaf node** — it calls nobody else.
- The cache-aside layer absorbs the read traffic that dominates a catalog workload. Without it, every product page and search hits Postgres, which cannot sustain high browse concurrency.
- The `@Version` column is the correctness guarantee for inventory. Without it, two concurrent reservations of the last unit read `available=1`, both pass the check, and both decrement — overselling by one. With it, the second writer's `UPDATE WHERE version=N` finds `version=N+1` and fails deterministically.

### In real-world systems
- Shopify, Amazon, and every high-scale catalog use cache-aside (or cache-on-write) to move product reads off the primary DB onto distributed caches.
- Optimistic locking is the standard choice for inventory at moderate contention: it keeps reads unblocked (no shared locks) and only serializes at the moment of conflict.
- PostgreSQL full-text search with `tsvector` / GIN index handles catalogs up to millions of products at sub-20ms latency — no Elasticsearch deployment, no sync pipeline, no separate infrastructure to operate.

---

## 3. How It Works — Step-by-Step Flows

### Create Product
```
POST /api/v1/products  (X-Seller-Id: <UUID>)
    │
    ├─ @Valid validates CreateProductRequest fields
    ├─ Resolve category (findById → 404 if provided but missing)
    ├─ Persist Product (status=ACTIVE, stockReserved=0, version=0)
    ├─ Persist ProductImage rows (ordered by sortOrder)
    ├─ @CacheEvict("productList", allEntries=true) ← new product invalidates all list pages
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
    │           ├─ Found → map to ProductResponse → write to Redis → return 200
    │           └─ Not found → throw ProductNotFoundException → 404
    │                          (404 is NOT cached — prevents negative cache poisoning)
    └─ GlobalExceptionHandler maps exception → ApiResponse.error(...)
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
                    ┌─────────────────────────────────────────────────────┐
                    │                  product-service                     │
                    │                                                      │
  HTTP ─────────────┤  ProductController      InventoryController         │
  (X-Seller-Id hdr) │       │                       │                    │
                    │  ProductServiceImpl      InventoryServiceImpl       │
                    │    @Cacheable              @Retryable               │
                    │    @CachePut               @Transactional           │
                    │    @CacheEvict                 │                    │
                    │       │                        │                    │
                    │  ProductRepo  CategoryRepo  StockMovementRepo       │
                    │       │                        │                    │
                    └───────┼────────────────────────┼────────────────────┘
                            │                        │
               ┌────────────┴──────────┐   ┌─────────┴──────────────┐
               │     PostgreSQL         │   │         Redis           │
               │                       │   │                         │
               │ products (@Version)   │   │ product::{id}  30min    │
               │ categories (tree)     │   │ productList::* 3min     │
               │ product_images        │   │ prefix: product-service:│
               │ stock_movements       │   └─────────────────────────┘
               │  (append-only log)    │
               └───────────────────────┘
```

### Key components

| Component | Role |
|---|---|
| `ProductServiceImpl` | CRUD, cache annotations, seller ownership checks |
| `InventoryServiceImpl` | Stock reserve/release with `@Retryable`; writes `StockMovement` in same TX |
| `CacheWarmupService` | Async `@EventListener(ApplicationReadyEvent)` — warms Redis on startup |
| `RedisConfig` | `RedisCacheManager` with per-cache TTLs, Jackson JSON serializer, `DefaultTyping` for polymorphic types |
| `RetryConfig` | `@EnableRetry` — activates `@Retryable` across the application context |
| `GlobalExceptionHandler` | `@RestControllerAdvice` maps all domain exceptions to `ApiResponse.error()` envelopes |

### Data model

```
products
  id              BIGSERIAL PK
  seller_id       UUID NOT NULL
  name            VARCHAR NOT NULL
  price           NUMERIC(12,2)
  stock_quantity  INT DEFAULT 0
  stock_reserved  INT DEFAULT 0       ← availableStock = stock_quantity - stock_reserved
  version         INT DEFAULT 0       ← @Version field — optimistic lock vector
  status          product_status      ← ACTIVE | INACTIVE | DELETED (soft delete)
  search_vector   TSVECTOR            ← GIN-indexed for plainto_tsquery

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
```
The `product-service::` prefix prevents Redis key collisions if the instance shares a Redis cluster with other services.

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
    // ownership check first — throws 403 before any write
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
| Correctness | Guaranteed if retries succeed | Guaranteed always |
| **Our choice** | ✅ Inventory: mostly reads, moderate writes | Order state: catastrophic to lose a transition |

Three retries at 100ms handles normal concurrency bursts. Flash sale scenarios (thousands of concurrent reservations) would need a queue or Redis atomic decrement approach.

### Short (3-min) TTL for product lists

Accepts brief staleness (a new product appears in search up to 3 minutes late) in exchange for a high cache hit rate on paginated listing pages — the dominant traffic pattern. `allEntries=true` eviction on writes is a best-effort pre-emptive flush; the TTL is the safety net.

### PostgreSQL FTS vs. Elasticsearch

| | PostgreSQL GIN + tsvector | Elasticsearch |
|---|---|---|
| Infrastructure | None — same DB | Separate cluster to operate |
| Latency at 1M rows | 5–20ms | 1–5ms |
| Features | Stemming, ranking, phrase | Fuzzy, autocomplete, facets, typo-tolerance |
| **Our choice** | ✅ Catalog up to ~5M products | Needed beyond that or for fuzzy/autocomplete |

---

## 7. When to Use / Avoid

### Use this pattern when:
- **Read-heavy catalog** (browse >> writes): cache-aside with a short list TTL and a long per-item TTL captures most requests without cache thrash.
- **Moderate inventory contention** (tens of concurrent reservations, not thousands): `@Retryable` handles version conflicts without serializing readers.
- **No search infrastructure budget**: PostgreSQL FTS is production-grade for catalogs up to single-digit millions of products.

### Avoid when:
- **Flash sales / high-burst reservations** — optimistic retry storms exhaust the retry budget; use a Redis `DECR` atomic counter or a reservation queue instead.
- **Real-time inventory accuracy on listing pages** — the 3-min list cache means sold-out items remain visible briefly; not acceptable for limited-edition drops.
- **Fuzzy / autocomplete search** — `plainto_tsquery` doesn't handle typos or partial prefixes; use Elasticsearch or a dedicated search service.

---

## 8. Interview Insights

### Q: Why optimistic locking for inventory instead of pessimistic?

**A:** The catalog is read-heavy — hundreds of reads per write in a browsing pattern. Pessimistic locking (`SELECT FOR UPDATE`) holds a DB row lock for the full duration of the reservation update, which would block any concurrent read that touches the products table. Optimistic locking only checks for conflicts at commit time — reads are never blocked. The cost is retries on version conflicts, which resolve in 1–2 attempts under normal concurrency. The break-even point is roughly when concurrent writes outnumber reads — which happens in a flash sale, not a catalog browse.

### Q: What happens when all 3 `@Retryable` attempts are exhausted?

**A:** Spring Retry re-throws the last `ObjectOptimisticLockingFailureException` after the third attempt. `GlobalExceptionHandler` maps it to 409 Conflict. The caller (order-service) treats 409 from the inventory endpoint as `InsufficientStockException` and triggers the compensation path: it releases any already-reserved items and marks the order `PAYMENT_FAILED`. No stock is orphaned in a reserved state.

### Q: How does the GIN index accelerate full-text search?

**A:** A B-tree index can't answer "which rows contain the word 'running'?" efficiently because it indexes entire values. A GIN (Generalized Inverted Index) pre-builds an inverted index: each stemmed lexeme maps to the list of row IDs containing it. `plainto_tsquery('running shoes')` becomes `'run' & 'shoe'` after English stemming, and the GIN index returns the intersection of both posting lists in O(log N + result count) — essentially the same algorithm Lucene uses. Without the index, it's a full table scan with `to_tsvector()` called per row.

### Q: Explain the cache warmup strategy and why it matters.

**A:** On cold start, every request is a cache miss that hits Postgres. For a high-traffic catalog this creates a "thundering herd" at restart: all concurrent requests land on the DB simultaneously. The `CacheWarmupService` fires after the Spring context is ready (`ApplicationReadyEvent`) and pre-loads the top 100 products into Redis asynchronously — the server accepts traffic immediately while the most popular items warm up in the background. This converts the burst of cache misses at restart into a handful of pre-emptive DB reads.

### Q: Why does `@CacheEvict(allEntries=true)` run on every product update?

**A:** The list cache key is a composite of page number, size, category, and status filter. When a product changes price or status, any of thousands of possible list pages could be stale. Tracking exactly which pages contain the updated product is impractical. `allEntries=true` is a blunt but correct solution — it trades brief extra DB load (all list pages are cold for the next 3 minutes of TTL refill) for correctness. If list cache freshness were critical, the right approach is a short TTL alone, not eviction.

### Q: The seller ID comes from an HTTP header, not a JWT. Is that secure?

**A:** In this architecture, yes — the header is injected by the Nginx gateway (trusted internal boundary), and the service is not directly reachable from outside. In production you'd strengthen this with: (1) mTLS between gateway and services so only the signed gateway can set the header, or (2) a signed JWT claim that the service verifies independently. The pattern — trusting a gateway-set header on a private network — is standard in internal service meshes where the perimeter is the trust boundary.
