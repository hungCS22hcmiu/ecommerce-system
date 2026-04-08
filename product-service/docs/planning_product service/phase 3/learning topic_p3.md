# Caching Strategies for Distributed E-Commerce Systems

> A practical guide covering patterns, Redis data types, Spring Cache abstraction, and when caching actually hurts.

---

## Table of Contents

1. [Cache-Aside Pattern](#1-cache-aside-pattern)
2. [Cache Invalidation Strategies](#2-cache-invalidation-strategies)
3. [Redis Data Types in Practice](#3-redis-data-types-in-practice)
4. [Spring Cache Abstraction](#4-spring-cache-abstraction)
5. [When Caching Hurts](#5-when-caching-hurts)
6. [Quick Reference Cheat Sheet](#6-quick-reference-cheat-sheet)

---

## 1. Cache-Aside Pattern

Also called **Lazy Loading**, this is the most common caching pattern. The application owns the cache — it manually checks, loads, and writes.

### Flow

```
READ REQUEST
     │
     ▼
┌─────────────┐     HIT      ┌─────────────┐
│ Check Cache │─────────────▶│ Return Data │
└─────────────┘              └─────────────┘
     │ MISS
     ▼
┌─────────────┐
│  Query DB   │
└─────────────┘
     │
     ▼
┌─────────────────────┐
│ Write result to     │
│ Cache (with TTL)    │
└─────────────────────┘
     │
     ▼
┌─────────────┐
│ Return Data │
└─────────────┘
```

### Java Implementation

```java
public Product getProduct(Long productId) {
    // 1. Check cache
    String cacheKey = "product:" + productId;
    Product cached = (Product) redisTemplate.opsForValue().get(cacheKey);

    // 2. Cache HIT — return immediately
    if (cached != null) {
        return cached;
    }

    // 3. Cache MISS — load from DB
    Product product = productRepository.findById(productId)
        .orElseThrow(() -> new ProductNotFoundException(productId));

    // 4. Write to cache with TTL
    redisTemplate.opsForValue().set(cacheKey, product, Duration.ofMinutes(30));

    return product;
}
```

### Write Path (Invalidation on Update)

```java
public Product updateProduct(Long productId, ProductUpdateRequest request) {
    Product updated = productRepository.save(/* apply changes */);

    // Evict stale cache entry — next read triggers a fresh DB load
    redisTemplate.delete("product:" + productId);

    return updated;
}
```

### Characteristics

| Aspect | Detail |
|---|---|
| **Cache population** | Lazy — only on first read miss |
| **Consistency** | Eventual — brief window of staleness after writes |
| **Fault tolerance** | High — app works even if Redis is down (just hits DB) |
| **Cold start** | First requests after deploy or flush are slow |
| **Best for** | Read-heavy data, product catalogs, user profiles |

### Cache Stampede (Thundering Herd)

When a popular key expires, many simultaneous requests all miss and hammer the DB at once.

**Mitigation — probabilistic early expiry:**

```java
// Re-fetch slightly before TTL expires using random jitter
public Product getProductSafe(Long productId) {
    String key = "product:" + productId;
    ValueOperations<String, Product> ops = redisTemplate.opsForValue();

    Product cached = ops.get(key);
    Long ttl = redisTemplate.getExpire(key, TimeUnit.SECONDS);

    // If within 60s of expiry, 10% chance of early refresh
    boolean earlyRefresh = ttl != null && ttl < 60 && Math.random() < 0.1;

    if (cached != null && !earlyRefresh) {
        return cached;
    }

    Product fresh = productRepository.findById(productId).orElseThrow();
    ops.set(key, fresh, Duration.ofMinutes(30));
    return fresh;
}
```

---

## 2. Cache Invalidation Strategies

> "There are only two hard things in Computer Science: cache invalidation and naming things." — Phil Karlton

### 2.1 TTL-Based (Time-To-Live)

The cache entry automatically expires after a fixed duration. No explicit invalidation needed.

```java
// Redis: key expires after 30 minutes regardless of DB changes
redisTemplate.opsForValue().set("product:42", product, Duration.ofMinutes(30));
```

**Decision matrix:**

| Scenario | Recommended TTL |
|---|---|
| Product price (changes frequently) | 5 – 10 min |
| Product description / images | 1 – 6 hours |
| Category tree | 12 – 24 hours |
| Exchange rates | 1 – 5 min |
| User profile | 15 – 30 min |

**Pros:**
- Zero infrastructure — no event bus needed
- Simple to reason about and debug
- Automatic self-healing from bugs

**Cons:**
- Stale window is always `TTL` seconds long — you may serve old prices
- Not suitable when consistency matters (flash sales, inventory counts)

---

### 2.2 Event-Based Invalidation

When data changes in the DB, an event is published and the cache is proactively evicted or refreshed.

```
DB Write ──▶ Publish Event ──▶ Cache Listener ──▶ Evict / Refresh Key
```

**With Spring Events:**

```java
// 1. Publish event on save
@Service
public class ProductService {
    @Autowired private ApplicationEventPublisher eventPublisher;

    public Product updateProduct(Long id, ProductUpdateRequest req) {
        Product updated = productRepository.save(/* apply */);
        eventPublisher.publishEvent(new ProductUpdatedEvent(id));
        return updated;
    }
}

// 2. Listen and evict
@Component
public class ProductCacheInvalidator {
    @Autowired private RedisTemplate<String, Object> redisTemplate;

    @EventListener
    public void onProductUpdated(ProductUpdatedEvent event) {
        redisTemplate.delete("product:" + event.getProductId());
        // Optionally pre-warm: load from DB and re-cache
    }
}
```

**With Kafka (distributed, cross-service):**

```java
// Producer (Inventory Service)
kafkaTemplate.send("product-events", new ProductChangedEvent(productId, "UPDATED"));

// Consumer (any service that caches products)
@KafkaListener(topics = "product-events")
public void handleProductEvent(ProductChangedEvent event) {
    cacheManager.getCache("products").evict(event.getProductId());
}
```

**Pros:**
- Near-real-time consistency
- TTL can be set very long (reduces DB load)
- Works across microservices via Kafka/RabbitMQ

**Cons:**
- Requires event bus infrastructure
- Complex failure scenarios (what if the eviction event is lost?)
- Risk of cache and DB diverging if events are missed

---

### TTL vs Event-Based — Decision Guide

```
Is your data updated frequently (many times/hour)?
    YES ──▶ Can you tolerate X seconds of stale data?
               YES ──▶ TTL (pick TTL = acceptable staleness window)
               NO  ──▶ Event-Based
    NO  ──▶ TTL with a long window (hours/days) is fine
                     Add event-based as a bonus optimization
```

**Hybrid approach (best of both):**

```java
// Short TTL as safety net + event-based for immediate invalidation
redisTemplate.opsForValue().set(key, product, Duration.ofHours(1)); // safety net TTL
// + Kafka listener evicts immediately on ProductUpdated event
```

---

## 3. Redis Data Types in Practice

### 3.1 String — Single Product Lookup

Best for simple key → value where the value is a serialized object.

```java
// Store
redisTemplate.opsForValue().set("product:1001", product, Duration.ofMinutes(30));

// Retrieve
Product p = (Product) redisTemplate.opsForValue().get("product:1001");
```

**Redis CLI:**
```bash
SET product:1001 '{"id":1001,"name":"Laptop","price":999.99}' EX 1800
GET product:1001
```

**Use when:** You always read the full object. Simple and fast — O(1).

---

### 3.2 Hash — Product Fields

A Hash stores a map of fields inside one Redis key. Ideal when you often read only a subset of fields (e.g., just price and stock).

```java
// Store individual fields
HashOperations<String, String, Object> hash = redisTemplate.opsForHash();
hash.put("product:hash:1001", "name",     "Gaming Laptop");
hash.put("product:hash:1001", "price",    "999.99");
hash.put("product:hash:1001", "stock",    "42");
hash.put("product:hash:1001", "category", "Electronics");
redisTemplate.expire("product:hash:1001", Duration.ofMinutes(30));

// Read only the fields you need
List<Object> fields = hash.multiGet("product:hash:1001", List.of("price", "stock"));

// Update a single field without re-caching the whole object
hash.put("product:hash:1001", "stock", "41");  // Inventory decremented
```

**Redis CLI:**
```bash
HSET product:hash:1001 name "Gaming Laptop" price 999.99 stock 42
HGET product:hash:1001 price
HMGET product:hash:1001 price stock
HINCRBY product:hash:1001 stock -1   # atomic decrement
```

**Use when:**
- You need partial field reads (saves bandwidth)
- Individual fields update independently (price, stock)
- Building a "product detail page" with multiple data sources

**String vs Hash:**

| | String | Hash |
|---|---|---|
| Read full object | ✅ Fast | ✅ Fast (`HGETALL`) |
| Read one field | ❌ Deserialize all | ✅ `HGET` |
| Update one field | ❌ Re-serialize all | ✅ `HSET` |
| Memory | Slightly less | Slightly more |

---

### 3.3 Sorted Set — Rankings & Leaderboards

A Sorted Set stores members with a `score`. Members are auto-sorted by score. Perfect for best-sellers, trending products, search rankings.

```java
ZSetOperations<String, String> zset = redisTemplate.opsForZSet();

// Track sales: increment score each time product is purchased
zset.incrementScore("rankings:bestsellers:electronics", "product:1001", 1.0);
zset.incrementScore("rankings:bestsellers:electronics", "product:1002", 1.0);

// Get top 10 best-sellers (highest score = most sold)
Set<String> top10 = zset.reverseRange("rankings:bestsellers:electronics", 0, 9);

// Get rank of a specific product (0-indexed)
Long rank = zset.reverseRank("rankings:bestsellers:electronics", "product:1001");

// Get products with scores (for display with sale count)
Set<ZSetOperations.TypedTuple<String>> topWithScores =
    zset.reverseRangeWithScores("rankings:bestsellers:electronics", 0, 9);
```

**Redis CLI:**
```bash
ZINCRBY rankings:bestsellers:electronics 1 "product:1001"
ZREVRANGE rankings:bestsellers:electronics 0 9          # top 10 IDs
ZREVRANGEBYSCORE rankings:bestsellers:electronics +inf -inf LIMIT 0 10 WITHSCORES
ZRANK rankings:bestsellers:electronics "product:1001"    # rank (low = worst)
ZREVRANK rankings:bestsellers:electronics "product:1001" # rank (low = best)
```

**Use when:**
- Best-seller / trending lists
- Search result ranking
- User loyalty points leaderboard
- Price-range product filtering (`ZRANGEBYSCORE`)

---

## 4. Spring Cache Abstraction

Spring Cache lets you add caching via annotations — no manual Redis calls needed for standard patterns.

### Setup

```java
// pom.xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-data-redis</artifactId>
</dependency>
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-cache</artifactId>
</dependency>
```

```java
// Application config
@SpringBootApplication
@EnableCaching  // Required!
public class EcommerceApplication { ... }
```

```yaml
# application.yml
spring:
  cache:
    type: redis
  redis:
    host: localhost
    port: 6379
  data:
    redis:
      repositories:
        enabled: false
```

---

### 4.1 `@Cacheable` — Read Through

Checks the cache first; only calls the method on a miss. Subsequent calls return the cached value.

```java
@Service
public class ProductService {

    // Cache key: "products::1001"
    @Cacheable(value = "products", key = "#productId")
    public Product getProduct(Long productId) {
        // Only executes on cache MISS
        return productRepository.findById(productId).orElseThrow();
    }

    // Conditional caching — only cache active products
    @Cacheable(value = "products", key = "#productId", condition = "#result?.active == true")
    public Product getActiveProduct(Long productId) {
        return productRepository.findById(productId).orElseThrow();
    }

    // Unless — don't cache null results
    @Cacheable(value = "products", key = "#productId", unless = "#result == null")
    public Product findProduct(Long productId) {
        return productRepository.findById(productId).orElse(null);
    }
}
```

---

### 4.2 `@CacheEvict` — Invalidation

Removes an entry (or all entries) from the cache. Use on write operations.

```java
@Service
public class ProductService {

    // Evict single product after update
    @CacheEvict(value = "products", key = "#productId")
    public Product updateProduct(Long productId, ProductUpdateRequest request) {
        return productRepository.save(applyUpdate(productId, request));
    }

    // Evict product AND any listing caches that may contain it
    @Caching(evict = {
        @CacheEvict(value = "products",         key = "#productId"),
        @CacheEvict(value = "product-listings", allEntries = true)
    })
    public void deleteProduct(Long productId) {
        productRepository.deleteById(productId);
    }

    // Clear entire cache (use sparingly — affects all users)
    @CacheEvict(value = "products", allEntries = true)
    public void clearProductCache() { }

    // Evict BEFORE the method executes (rare — useful for write-then-read patterns)
    @CacheEvict(value = "products", key = "#product.id", beforeInvocation = true)
    public Product replaceProduct(Product product) {
        return productRepository.save(product);
    }
}
```

---

### 4.3 `@CachePut` — Always Write

Always executes the method AND updates the cache. Unlike `@Cacheable`, it never skips the method call. Use for write-through caching.

```java
@Service
public class ProductService {

    // Always runs, always writes result to cache — no skip
    @CachePut(value = "products", key = "#result.id")
    public Product createProduct(ProductCreateRequest request) {
        return productRepository.save(buildProduct(request));
    }

    // Update DB and refresh cache atomically
    @CachePut(value = "products", key = "#productId")
    public Product refreshProduct(Long productId) {
        return productRepository.findById(productId).orElseThrow();
    }
}
```

---

### Annotation Comparison

| Annotation | Executes Method? | Cache Behavior | Typical Use |
|---|---|---|---|
| `@Cacheable` | Only on miss | Read from cache if hit, populate on miss | GET endpoints |
| `@CacheEvict` | Yes | Removes entry after (or before) method | DELETE / UPDATE |
| `@CachePut` | Always | Writes result to cache after every call | CREATE / force-refresh |

---

### Custom TTL per Cache

```java
@Configuration
public class CacheConfig {

    @Bean
    public RedisCacheManager cacheManager(RedisConnectionFactory factory) {
        Map<String, RedisCacheConfiguration> configs = new HashMap<>();

        configs.put("products",      ttl(Duration.ofMinutes(30)));
        configs.put("categories",    ttl(Duration.ofHours(6)));
        configs.put("user-profiles", ttl(Duration.ofMinutes(15)));
        configs.put("flash-sales",   ttl(Duration.ofMinutes(2)));

        return RedisCacheManager.builder(factory)
            .cacheDefaults(ttl(Duration.ofMinutes(10)))
            .withInitialCacheConfigurations(configs)
            .build();
    }

    private RedisCacheConfiguration ttl(Duration duration) {
        return RedisCacheConfiguration.defaultCacheConfig()
            .entryTtl(duration)
            .serializeValuesWith(
                RedisSerializationContext.SerializationPair.fromSerializer(
                    new GenericJackson2JsonRedisSerializer()
                )
            );
    }
}
```

---

## 5. When Caching Hurts

Caching is not always beneficial. Below are patterns where adding a cache makes things **worse**.

---

### 5.1 Low-Cardinality Queries

If a query returns the same result for every possible input variation, caching provides almost no benefit — the cache entry is hit so rarely it never pays off, but you still pay the write cost.

```java
// ❌ Bad: only 2 possible inputs → cache size is 2 entries, nearly useless
@Cacheable(value = "orders", key = "#status")
public List<Order> getOrdersByStatus(String status) {
    // status is either "ACTIVE" or "INACTIVE"
    // With 1M orders, this returns tens of thousands of records
    return orderRepository.findByStatus(status);
}
```

**Why it hurts:**
- You're caching large result sets with only 2 possible keys
- Any single order status change requires full cache eviction
- Memory footprint is enormous relative to benefit

**Fix:** Cache at a finer granularity, or don't cache at all — use a DB read replica instead.

---

### 5.2 Write-Heavy Data

When data is updated more often than it's read, invalidation overhead exceeds the savings from cache hits.

```java
// ❌ Bad: inventory changes on every purchase/restock
@Cacheable(value = "inventory", key = "#productId")
public Integer getStock(Long productId) {
    return inventoryRepository.findStockByProductId(productId);
}

@CacheEvict(value = "inventory", key = "#productId")
public void updateStock(Long productId, int delta) {
    inventoryRepository.updateStock(productId, delta);
    // In a busy warehouse this fires hundreds of times per minute
    // Cache hit rate is near 0% — all pain, no gain
}
```

**Why it hurts:**
- High eviction rate → cache is almost always cold
- Extra round-trips to Redis on every write with near-zero cache hits
- Adds latency and complexity with no throughput gain

**Fix:** Use Redis as the **source of truth** for counters, not a cache:

```java
// ✅ Better: use Redis INCRBY as atomic counter, no eviction needed
public void decrementStock(Long productId, int quantity) {
    Long remaining = redisTemplate.opsForValue()
        .increment("stock:" + productId, -quantity);

    if (remaining < 0) {
        // Compensate — rollback and throw
        redisTemplate.opsForValue().increment("stock:" + productId, quantity);
        throw new InsufficientStockException(productId);
    }
    // Async: persist to DB
    stockEventPublisher.publish(new StockChangedEvent(productId, remaining));
}
```

---

### 5.3 Other Situations Where Caching Hurts

**Unique or near-unique queries (high cardinality keys, low reuse):**

```java
// ❌ Bad: each user's filtered+sorted order history is unique
@Cacheable(value = "orders", key = "#userId + #filter + #sort + #page")
public Page<Order> searchOrders(Long userId, OrderFilter filter,
                                 Sort sort, Pageable page) {
    // Cache key space is enormous — entries are almost never reused
    // Memory fills up fast, eviction pressure is constant
}
```

**Strong consistency requirements:**

```java
// ❌ Bad: stale prices = wrong charges = chargebacks
@Cacheable(value = "prices", key = "#productId")
public BigDecimal getCheckoutPrice(Long productId) {
    // If price changed 10 seconds ago, customer is charged the old price
    // For checkout, always read from DB directly
}
```

**Small datasets (fits in DB cache anyway):**

```java
// ❌ Pointless: 5-row lookup table — DB serves this from its own buffer pool
@Cacheable("shippingZones")
public List<ShippingZone> getAllZones() {
    return shippingZoneRepository.findAll(); // returns 5 rows
}
```

---

### Decision Framework

```
Should I cache this?
        │
        ▼
Is it read more than written?
        │
    NO ──▶ DON'T CACHE (write-heavy data)
        │
       YES
        ▼
Does it have high key cardinality AND low reuse?
        │
    YES ──▶ DON'T CACHE (unique queries, no hit rate)
        │
        NO
        ▼
Does it require strong consistency? (e.g., checkout price, auth)
        │
    YES ──▶ DON'T CACHE (or use event-based with very short TTL)
        │
        NO
        ▼
    ✅ CACHE IT
```

---

## 6. Quick Reference Cheat Sheet

### Cache-Aside Flow

```
Read  → check cache → HIT: return | MISS: DB → write cache → return
Write → update DB → evict cache key
```

### Invalidation

| Strategy | Best For | Staleness Window |
|---|---|---|
| TTL | Tolerable staleness, simple setup | Up to TTL duration |
| Event-Based | Near-real-time consistency | Near zero |
| Hybrid | Critical data with high read load | Near zero (+ TTL safety net) |

### Redis Data Types

| Type | Command | E-Commerce Use |
|---|---|---|
| String | `GET` / `SET` | Full product object |
| Hash | `HGET` / `HSET` | Partial product fields, stock level |
| Sorted Set | `ZINCRBY` / `ZREVRANGE` | Best-sellers, trending, rankings |
| List | `LPUSH` / `LRANGE` | Recently viewed items |
| Set | `SADD` / `SMEMBERS` | Product tags, user wishlists |

### Spring Cache Annotations

| Annotation | Use Case |
|---|---|
| `@Cacheable` | Read operations — return from cache if present |
| `@CacheEvict` | Write/delete operations — remove stale cache entry |
| `@CachePut` | Always write to cache (write-through pattern) |
| `@Caching` | Combine multiple cache operations on one method |
| `@EnableCaching` | Enable Spring Cache on your `@SpringBootApplication` |

### Cache Anti-Patterns to Avoid

| Anti-Pattern | Symptom | Fix |
|---|---|---|
| Caching write-heavy data | Hit rate < 20%, constant evictions | Use Redis as primary store (INCR/DECR) |
| Low-cardinality cache keys | Few keys, huge values | Cache individual items or use read replica |
| No TTL | Memory fills up, stale data forever | Always set TTL, even if long |
| Caching at checkout | Wrong prices charged | Skip cache for financial-critical reads |
| Over-broad eviction (`allEntries=true`) | Cache stampede on writes | Evict specific keys only |
| Not handling cache-aside stampede | DB hammered on expiry | Add jitter to TTL or use probabilistic refresh |

---

*Generated for a distributed e-commerce system architecture study.*