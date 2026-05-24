# Redis-First Cart Architecture with Circuit Breaker

## Overview

Cart performance is a critical path: every user interaction — browsing, adding items, updating quantities — touches the cart. Routing those operations through PostgreSQL would introduce database round-trips on every request. The cart-service avoids this by making Redis the primary store for all read and write operations, reducing PostgreSQL to a durable backup that is updated asynchronously.

Two independent design decisions make this architecture work under failure conditions:

1. **Redis-first storage** — all cart state lives in a Redis hash keyed by user ID. Reads and mutations never block on Postgres. A background goroutine syncs Redis to Postgres every 30 seconds.
2. **Circuit breaker on the product-service client** — `AddItem` is the only cart operation that calls product-service (to validate stock and price). If product-service goes down, the circuit breaker opens after 5 failures and begins fast-failing `POST /cart/items` with HTTP 503. All other cart operations (`GET /cart`, `UpdateItem`, `RemoveItem`, `ClearCart`) bypass the product client entirely and continue serving from Redis at full speed.

---

## 1. Redis as Primary Store

### Data model

Each user's cart is stored as a Redis hash at key `cart:{userID}`. Each field is a `productID` (string); the value is a JSON-encoded `CartItemValue`:

`cart-service/internal/repository/redis_cart_repository.go`
```go
type CartItemValue struct {
    ProductName string  `json:"product_name"`
    Quantity    int     `json:"quantity"`
    UnitPrice   float64 `json:"unit_price"`
}
```

The hash is set with a 7-day TTL on every write, so abandoned carts expire without manual cleanup:

`cart-service/internal/repository/redis_cart_repository.go` — `AddOrUpdateItem()`
```go
_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
    pipe.HSet(ctx, key, field, jsonVal)
    pipe.Expire(ctx, key, 7*24*time.Hour)   // reset TTL on every write
    return nil
})
```

### Reads serve directly from Redis

`GetCart` reads the entire hash with a single `HGETALL` — no Postgres involved:

`cart-service/internal/service/cart_service.go` — `GetCart()`
```go
func (s *cartService) GetCart(ctx context.Context, userID uuid.UUID) (*dto.CartResponse, error) {
    items, err := s.redisRepo.GetCart(ctx, userID)
    if err != nil {
        return nil, err
    }
    // Compute totals and return — no product-service call, no Postgres call.
    var total float64
    itemResponses := make([]dto.CartItemResponse, 0, len(items))
    for productID, val := range items {
        subtotal := val.UnitPrice * float64(val.Quantity)
        total += subtotal
        itemResponses = append(itemResponses, dto.CartItemResponse{...})
    }
    return &dto.CartResponse{...}, nil
}
```

`cart-service/internal/repository/redis_cart_repository.go` — `GetCart()`
```go
func (r *redisCartRepository) GetCart(ctx context.Context, userID uuid.UUID) (map[int64]CartItemValue, error) {
    key := fmt.Sprintf("cart:%s", userID)
    res, err := r.rdb.HGetAll(ctx, key).Result()
    // ...
    return cart, nil
}
```

### Writes use WATCH/MULTI/EXEC for optimistic concurrency

`AddOrUpdateItem` wraps the `HSET` + `EXPIRE` in a Redis `WATCH/MULTI/EXEC` transaction. If another writer modifies the same key between `WATCH` and `EXEC`, the transaction aborts with `TxFailedErr` and the operation retries up to 3 times before returning `ErrConcurrentUpdate`:

`cart-service/internal/repository/redis_cart_repository.go` — `AddOrUpdateItem()`
```go
for retries := 0; retries < 3; retries++ {
    err := r.rdb.Watch(ctx, func(tx *redis.Tx) error {
        _, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
            pipe.HSet(ctx, key, field, jsonVal)
            pipe.Expire(ctx, key, 7*24*time.Hour)
            return nil
        })
        return err
    }, key)

    if err == nil {
        return nil
    }
    if !errors.Is(err, redis.TxFailedErr) {
        return err   // hard error — don't retry
    }
    // TxFailedErr: watched key was modified — retry
}
return ErrConcurrentUpdate
```

`RemoveItem` and `ClearCart` use single-command operations (`HDEL`, `DEL`) that are inherently atomic on a single Redis key and do not need WATCH:

`cart-service/internal/repository/redis_cart_repository.go`
```go
func (r *redisCartRepository) RemoveItem(ctx context.Context, userID uuid.UUID, productID int64) error {
    key := fmt.Sprintf("cart:%s", userID)
    field := strconv.FormatInt(productID, 10)
    return r.rdb.HDel(ctx, key, field).Err()
}

func (r *redisCartRepository) ClearCart(ctx context.Context, userID uuid.UUID) error {
    key := fmt.Sprintf("cart:%s", userID)
    return r.rdb.Del(ctx, key).Err()
}
```

---

## 2. PostgreSQL as Durable Backup

PostgreSQL holds a replica of Redis cart state persisted asynchronously. It is used for durability — if Redis is flushed or a cart key expires, Postgres has the last synced snapshot.

### Background sync goroutine

A `StartSyncWorker` goroutine runs for the lifetime of the process. Every 30 seconds it scans all `cart:*` keys from Redis and bulk-replaces the corresponding Postgres rows:

`cart-service/internal/cache/sync.go` — `StartSyncWorker()`
```go
func StartSyncWorker(
    ctx context.Context,
    rdb *redis.Client,
    redisRepo repository.RedisCartRepository,
    cartRepo repository.CartRepository,
) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            syncAll(ctx, rdb, redisRepo, cartRepo)
        case <-ctx.Done():
            return
        }
    }
}
```

`syncAll` scans keys in pages of 100, calls `syncOne` for each, logs per-key failures without stopping the rest:

`cart-service/internal/cache/sync.go` — `syncAll()`
```go
for {
    keys, next, err := rdb.Scan(ctx, cursor, "cart:*", 100).Result()
    // ...
    for _, key := range keys {
        if err := syncOne(ctx, key, redisRepo, cartRepo); err != nil {
            slog.Error("cart sync: failed to sync cart", "key", key, "error", err)
            failed++
        } else {
            synced++
        }
    }
    cursor = next
    if cursor == 0 { break }
}
```

`syncOne` performs a full replace — it deletes all existing Postgres items for the cart and bulk-inserts the current Redis state in a single transaction:

`cart-service/internal/cache/sync.go` — `syncOne()`
```go
// Full replace — delete all existing items then bulk-insert
return cartRepo.ReplaceItems(ctx, cart.ID, items)
```

`cart-service/internal/repository/cart_repository.go` — `ReplaceItems()`
```go
func (r *cartRepository) ReplaceItems(ctx context.Context, cartID uuid.UUID, items []model.CartItem) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Where("cart_id = ?", cartID).Delete(&model.CartItem{}).Error; err != nil {
            return err
        }
        if len(items) == 0 {
            return nil
        }
        for i := range items {
            items[i].CartID = cartID
        }
        return tx.Create(&items).Error   // bulk insert
    })
}
```

### ClearCart syncs both stores immediately

Unlike other mutations (which defer Postgres writes to the next sync tick), `ClearCart` deletes from both Redis and Postgres synchronously to avoid the Postgres replica growing stale after an explicit clear:

`cart-service/internal/service/cart_service.go` — `ClearCart()`
```go
func (s *cartService) ClearCart(ctx context.Context, userID uuid.UUID) error {
    // First clear Redis
    if err := s.redisRepo.ClearCart(ctx, userID); err != nil {
        return err
    }
    // Also clear Postgres immediately
    return s.cartRepo.ClearCart(ctx, userID)
}
```

### Goroutine lifecycle

`cart-service/cmd/server/main.go`
```go
syncCtx, syncCancel := context.WithCancel(context.Background())
go cache.StartSyncWorker(syncCtx, rdb, redisRepo, cartRepo)
defer syncCancel()   // cancelled on SIGINT/SIGTERM before HTTP shutdown
```

The `defer syncCancel()` fires before the HTTP server shuts down, so no new sync passes begin after the shutdown signal.

---

## 3. Product-Service Client with Redis Validation Cache

`AddItem` is the only cart operation that must call product-service. It needs the product's current name, price, and available stock to populate the cart item. The client adds a second Redis cache layer — product data is cached at `product:v:{id}` for 5 seconds to avoid redundant HTTP calls when the same product is added by multiple concurrent users:

`cart-service/internal/client/product_client.go`
```go
const productCacheTTL = 5 * time.Second

func (c *productClient) GetProduct(ctx context.Context, productID int64) (*ProductInfo, error) {
    // Cache hit: return immediately, skip circuit breaker check (no network involved).
    if c.cache != nil {
        if cached, err := c.cache.get(ctx, productID); err == nil && cached != nil {
            return cached, nil
        }
    }

    if !c.cb.Allow() {
        return nil, ErrServiceUnavailable
    }
    // ... HTTP call ...
}
```

On a successful response, the product data is written back into Redis so subsequent callers within the 5-second window hit the cache:

`cart-service/internal/client/product_client.go`
```go
c.cb.RecordSuccess()
_ = c.cache.set(ctx, productID, &result.Data, productCacheTTL)
return &result.Data, nil
```

The cache key format is `product:v:{id}` — the `v` namespace is distinct from `cart:{uuid}` keys, so scans with `cart:*` in the sync worker never match product cache entries.

---

## 4. Circuit Breaker

### State machine

The circuit breaker is a custom three-state machine: `CLOSED → OPEN → HALF_OPEN → CLOSED`.

`cart-service/internal/client/circuit_breaker.go`
```go
type cbState int

const (
    cbClosed   cbState = iota // normal operation — all requests pass through
    cbOpen                    // failing fast — requests rejected immediately
    cbHalfOpen                // one probe request allowed to test recovery
)
```

`cart-service/internal/client/circuit_breaker.go` — `Allow()`
```go
func (cb *CircuitBreaker) Allow() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    switch cb.state {
    case cbClosed:
        return true
    case cbOpen:
        if time.Now().After(cb.openUntil) {
            cb.state = cbHalfOpen
            return true   // first caller after cool-down gets the probe
        }
        return false
    case cbHalfOpen:
        return false   // block all callers until probe resolves
    }
    return false
}
```

`cart-service/internal/client/circuit_breaker.go`
```go
func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    cb.failures = 0
    cb.state = cbClosed
}

func (cb *CircuitBreaker) RecordFailure() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    cb.failures++
    if cb.failures >= cb.threshold {
        cb.state = cbOpen
        cb.openUntil = time.Now().Add(cb.timeout)
    }
}
```

### Configuration

`cart-service/internal/client/product_client.go` — `NewProductClient()`
```go
cb: NewCircuitBreaker(5, 30*time.Second),
// Open after 5 consecutive failures; stay open for 30 seconds.
```

| Parameter | Value | Meaning |
|---|---|---|
| `threshold` | 5 | 5 consecutive `RecordFailure` calls open the circuit |
| `timeout` | 30s | Circuit stays OPEN for 30 seconds before moving to HALF_OPEN |

### Retry behavior inside the client

Before the circuit breaker applies, each call through the HTTP client already attempts up to 3 times with exponential backoff (200ms, 400ms) for network/timeout errors and 5xx responses:

`cart-service/internal/client/product_client.go` — `GetProduct()`, retry loop
```go
const maxAttempts = 3

for attempt := range maxAttempts {
    if attempt > 0 {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(time.Duration(100<<attempt) * time.Millisecond): // 200ms, 400ms
        }
    }
    // ...
    if resp.StatusCode >= 500 {
        resp.Body.Close()
        lastErr = ErrServiceUnavailable
        continue   // retry
    }
    // ...
}
// All attempts exhausted
c.cb.RecordFailure()
return nil, lastErr
```

A 404 from product-service is treated as a definitive "not found" — it triggers `RecordSuccess` (not a failure) and returns `ErrNotFound` without retry:

`cart-service/internal/client/product_client.go`
```go
if resp.StatusCode == http.StatusNotFound {
    resp.Body.Close()
    c.cb.RecordSuccess()   // 404 is not a service failure
    return nil, ErrNotFound
}
```

This means the circuit counter only increments on actual service unavailability, not on legitimate "product not found" responses.

### Degraded mode: read operations bypass the circuit entirely

`GetCart`, `UpdateItem`, `RemoveItem`, and `ClearCart` never call `productClient.GetProduct`. They operate directly on the Redis hash and return immediately, regardless of product-service state:

`cart-service/internal/service/cart_service.go`
```go
// GetCart, UpdateItem, RemoveItem, ClearCart — no product client call.

func (s *cartService) AddItem(ctx context.Context, userID uuid.UUID, req dto.AddItemRequest) (*dto.CartResponse, error) {
    product, err := s.productClient.GetProduct(ctx, req.ProductID)   // ONLY AddItem calls product-service
    if err != nil {
        if errors.Is(err, client.ErrNotFound) {
            return nil, ErrProductNotFound
        }
        if errors.Is(err, client.ErrServiceUnavailable) {
            return nil, ErrProductServiceUnavailable   // 503 to caller
        }
        return nil, err
    }
    // ...
}
```

---

## 5. Test Evidence

### Unit tests: circuit breaker state machine

`cart-service/internal/client/circuit_breaker_test.go` — `TestCircuitBreaker_FullCycle`

The full `CLOSED → OPEN → HALF_OPEN → CLOSED` cycle is exercised with a 50ms timeout so the test runs in ~100ms:

```go
cb := NewCircuitBreaker(5, 50*time.Millisecond)

// Record 5 failures → trips to OPEN
for i := 0; i < 5; i++ {
    assert.True(t, cb.Allow())
    cb.RecordFailure()
}

// OPEN — fast-fail
assert.False(t, cb.Allow(), "OPEN state must fast-fail")

// After cool-down → HALF_OPEN, probe allowed
time.Sleep(60 * time.Millisecond)
assert.True(t, cb.Allow(), "HALF_OPEN must allow the probe request")
assert.False(t, cb.Allow(), "HALF_OPEN must block subsequent callers until probe resolves")

// Probe succeeds → CLOSED, failures counter reset
cb.RecordSuccess()
assert.True(t, cb.Allow(), "after RecordSuccess the breaker is CLOSED again")
cb.RecordFailure()
assert.True(t, cb.Allow(), "failures counter resets; one failure is not enough to re-open")
```

`TestCircuitBreaker_HalfOpen_ProbeFails_ReturnsToOpen` — a failed probe in HALF_OPEN drives the state back to OPEN:

```go
cb := NewCircuitBreaker(3, 50*time.Millisecond)
// Drive to OPEN, wait, allow probe
time.Sleep(60 * time.Millisecond)
assert.True(t, cb.Allow(), "HALF_OPEN allows probe")
// Probe fails
cb.RecordFailure()
assert.False(t, cb.Allow(), "after probe failure, breaker is OPEN again")
```

`TestCircuitBreaker_BelowThreshold_StaysClosed` — below-threshold failures never open the circuit:

```go
cb := NewCircuitBreaker(5, 1*time.Second)
for i := 0; i < 4; i++ {
    cb.Allow(); cb.RecordFailure()
}
assert.True(t, cb.Allow(), "4 failures < threshold(5) — must stay CLOSED")
```

### Integration test: degraded mode with circuit open

`cart-service/internal/integration/cart_integration_test.go` — `TestDegradedMode_CircuitOpen_ReadOpsStillWork`

The test wires a broken product server that always returns 500, drives the circuit open, then asserts that all read/mutation paths except `AddItem` continue to work:

```go
// Drive the circuit open: 5 calls × 3 retries = 15 server hits
for i := 0; i < 5; i++ {
    _, _ = brokenClient.GetProduct(ctx, 1)
}
require.Equal(t, int32(15), hitCount.Load(), "5 calls × 3 retries must produce exactly 15 server hits")

// AddItem must fail fast — circuit OPEN, no HTTP request made
_, err := brokenSvc.AddItem(ctx, userID, dto.AddItemRequest{ProductID: 3, Quantity: 1})
assert.ErrorIs(t, err, service.ErrProductServiceUnavailable)
assert.Equal(t, hitsToOpen, hitCount.Load(), "open circuit: AddItem must not hit the product server")

// GetCart, UpdateItem, RemoveItem, ClearCart all succeed
cart, err := brokenSvc.GetCart(ctx, userID)
require.NoError(t, err)
assert.Len(t, cart.Items, 2)

cart, err = brokenSvc.UpdateItem(ctx, userID, 1, dto.UpdateItemRequest{Quantity: 5})
require.NoError(t, err)

err = brokenSvc.RemoveItem(ctx, userID, 1)
require.NoError(t, err)

err = brokenSvc.ClearCart(ctx, userID)
require.NoError(t, err)
```

### Integration test: Redis-to-Postgres sync

`cart-service/internal/integration/cart_integration_test.go` — `TestRedisToPostgresSync_Integration`

```go
doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 1, "quantity": 3}, userID)
doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 2, "quantity": 1}, userID)

// Trigger sync manually (bypasses the 30s ticker for test speed)
cache.SyncOnce(context.Background(), testRDB, testRedisRepo, testCartRepo)

var count int64
testDB.Raw(`
    SELECT COUNT(ci.*) FROM cart_items ci
    JOIN carts c ON c.id = ci.cart_id
    WHERE c.user_id = ?
`, userID).Scan(&count)

assert.Equal(t, int64(2), count)   // both Redis items now in Postgres
```

`TestCartTTL_Integration` — verifies the 7-day Redis key TTL is set correctly on every write:

```go
doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 1, "quantity": 1}, userID)

ttl, err := testRDB.TTL(context.Background(), key).Result()
require.NoError(t, err)

sevenDays := 7 * 24 * time.Hour
assert.Greater(t, ttl, sevenDays-10*time.Second)
assert.LessOrEqual(t, ttl, sevenDays)
```

### Chaos test: circuit breaker opens + degraded read performance

`script/test/chaos_cb_cart.sh` pauses product-service with `docker compose pause` and fires 10 `POST /cart/items` sequentially. After 5 calls exhaust their 3 retries and trip the breaker, subsequent calls fast-fail without waiting for the 5-second HTTP timeout:

```bash
docker compose pause product-service

for i in $(seq 1 10); do
  code=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST http://localhost:8002/api/v1/cart/items ...)
done
```

**Observed result (Phase 3, 2026-05-21):**

| Metric | Expected | Observed |
|---|---|---|
| HTTP status sequence | `000 000 000 000 000 503 503 503 503 503` | **exact match** |
| Requests before CB opens | 5 | **5** |
| Requests after CB opens (503) | 5 | **5** |

With the circuit open, `script/k6/cart_get_degraded.js` then measures read performance at 10 RPS for 30 seconds while product-service remains paused:

**Observed result:**

| Metric | Target | Observed |
|---|---|---|
| `GET /cart` P95 (product-service down) | < 20ms | **4ms** |
| Error rate | < 1% | **0.00%** |

`GET /cart` returns in 4ms because it reads a single Redis hash with `HGETALL` — no downstream service involved.

### Load test: `POST /cart/items` at 500 RPS

`script/k6/cart_ops.js` runs `POST /cart/items` at 500 RPS for 60 seconds. The 409 status (Redis WATCH conflict on concurrent writes) is treated as expected:

**Observed result (Phase 2, 2026-05-21):**

| Metric | Target | Observed |
|---|---|---|
| P95 latency | < 40ms | **36ms** |
| Throughput | ≥ 500 RPS | **496 RPS** |
| Error rate | < 5% | **0.00%** |

---

## Summary

| Operation | Storage path | Product-service call | Works when product-service is down? |
|---|---|---|---|
| `GET /cart` | Redis `HGETALL` | No | Yes |
| `POST /cart/items` | Redis `WATCH/MULTI/EXEC` + CB check | **Yes** (validate stock/price) | No — returns 503 when CB open |
| `PUT /cart/items/:id` | Redis `WATCH/MULTI/EXEC` | No | Yes |
| `DELETE /cart/items/:id` | Redis `HDEL` | No | Yes |
| `DELETE /cart` | Redis `DEL` + Postgres transaction | No | Yes |
| Background sync | Redis `SCAN` → Postgres `ReplaceItems` | No | Yes |

The design concentrates the dependency on product-service to a single write path (`AddItem`). Four of five user-visible operations are pure Redis and remain fully functional during a product-service outage.
