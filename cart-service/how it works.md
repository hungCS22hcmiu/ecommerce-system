# cart-service: How It Works

---

## 1. What Is It?

The `cart-service` is a Go microservice (Gin + GORM) that manages **shopping carts** for every user in the ecommerce platform.

**Analogy:** Think of it as a supermarket basket with two layers of memory. The basket itself (Redis) is what you carry — fast, right in your hands, immediately updated on every item change. A backup clipboard (PostgreSQL) follows you around and quietly writes down everything in your basket every 30 seconds. If you drop the basket (Redis restart), the clipboard has a recent-enough snapshot. The basket also checks with the product shelf (product-service) before accepting a new item — if the product is out of stock or discontinued, the basket refuses it.

**Responsibilities:**
- Redis-first cart storage: every cart operation is a Redis Hash operation, never a direct Postgres write
- WATCH/MULTI/EXEC optimistic locking on every cart write — no lost updates under concurrent requests
- Synchronous product validation (existence, active status, current price) on `AddItem`
- Circuit-breaking product-service calls: 5 consecutive failures → 30s OPEN state, fast-fail without waiting
- Background goroutine syncing Redis → PostgreSQL every 30 seconds for durability
- JWT validation using the same RSA public key issued by user-service

---

## 2. Why It Matters

### In this project
- Carts are written on nearly every user action (add, update, remove, clear). Hitting Postgres directly on every operation would be slow and would create unnecessary write load on the shared DB. Redis Hash operations are sub-millisecond and O(1).
- Price and product name are snapshotted into Redis at the moment of `AddItem`. UpdateItem skips the product-service call entirely — the price doesn't change retroactively for items already in a cart, just as a real price tag doesn't change while it's in your basket.
- The circuit breaker ensures cart reads (`GetCart`, `UpdateItem`, `RemoveItem`) keep working even when product-service is down. Only `AddItem` is blocked during an outage, because that's the only operation that actually needs product validation.

### In real-world systems
- Amazon, Shopify, and most e-commerce platforms store session/cart data in Redis precisely because write frequency is high and read latency matters for checkout conversion.
- The Redis-first + Postgres durability pattern (write-through or async sync) is standard: Redis for speed, relational DB for recovery and analytics.
- Circuit breakers are essential in microservices — without one, a slow product-service causes cart-service threads to pile up waiting for 5-second timeouts, cascading into a cart-service outage.

---

## 3. How It Works — Step-by-Step Flows

### AddItem (the most complex path)
```
POST /api/v1/cart/items  Authorization: Bearer <token>  body:{product_id, quantity}
    │
    ├─ Auth middleware: validate JWT signature → extract userID → inject into Gin ctx
    ├─ CartHandler.AddItem: parse + validate body (@validator)
    │
    ├─ CartService.AddItem:
    │     ├─ cb.Allow()? → false (circuit OPEN) → return ErrProductServiceUnavailable → 503
    │     │
    │     ├─ productClient.GetProduct(productID)
    │     │     ├─ HTTP GET product-service:8081/api/v1/products/{id}  timeout=5s
    │     │     ├─ On 5xx / timeout: retry up to 3× (100ms, 200ms backoff)
    │     │     ├─ All 3 fail → cb.RecordFailure() → (at 5 failures: circuit opens)
    │     │     ├─ 404 → ErrNotFound (no retry, no CB failure)
    │     │     └─ 200 + status≠ACTIVE → ErrNotFound
    │     │
    │     ├─ product found: snapshot name + price into CartItemValue
    │     │
    │     └─ redisRepo.AddOrUpdateItem(userID, productID, CartItemValue)
    │           ├─ WATCH cart:{userID}
    │           ├─ HGETALL → current state
    │           ├─ MULTI
    │           │    └─ HSET cart:{userID} {productID} <json>
    │           │    └─ EXPIRE cart:{userID} 7 days   (refresh TTL)
    │           ├─ EXEC
    │           │    ├─ OK → written ✓
    │           │    └─ nil (watched key changed) → retry up to 3× → ErrConcurrentUpdate
    │           └─ Returns error after 3 failed WATCH cycles
    │
    └─ GetCart(userID) → return current cart state
```

### Background Sync (every 30 seconds)
```
StartSyncWorker goroutine (started in main.go, cancelled on SIGTERM)
    │
    ├─ ticker fires every 30s
    │
    └─ SCAN "cart:*" in Redis (cursor-based, batch=100)
          For each key "cart:{uuid}":
            1. Parse UUID from key suffix
            2. redisRepo.GetCart(uuid) → map[productID]CartItemValue
            3. Skip if empty (avoid ghost Postgres rows)
            4. cartRepo.UpsertCart(uuid) → get-or-create carts row (status=ACTIVE)
            5. Convert map → []model.CartItem
            6. cartRepo.ReplaceItems(cartID, items)
                  BEGIN TX
                    DELETE FROM cart_items WHERE cart_id=?
                    INSERT INTO cart_items (bulk)
                  COMMIT
            7. On error: log + continue (sync is best-effort, goroutine never dies)
```

### Request Authentication (JWT middleware)
```
Authorization: Bearer eyJhbGc...
    │
    ├─ Extract token string after "Bearer "
    ├─ jwtpkg.ValidateToken(token, publicKey)  ← RS256 verify with shared public.pem
    │     └─ Verifies signature + expiry automatically
    ├─ uuid.Parse(claims.UserID) → typed uuid.UUID
    ├─ c.Set("userID", userID)   ← stored as uuid.UUID, not string
    └─ c.Next()

No blacklist check — cart is low-security; 15-min token TTL is sufficient.
```

---

## 4. System Design — Components & Architecture

```
                  ┌──────────────────────────────────────────────────────┐
                  │                   cart-service                        │
                  │                                                       │
HTTP ─────────────┤  middleware: Auth(JWT) → Logger → Recovery           │
(Bearer token)    │                    │                                  │
                  │              CartHandler                              │
                  │                    │                                  │
                  │              CartService                              │
                  │             /           \                             │
                  │    RedisCartRepo     ProductClient                   │
                  │    (primary)         (HTTP+CB+retry)                 │
                  │         │                                             │
                  │    CartRepo(Postgres)     ← sync only, not hot path  │
                  │                                                       │
                  │    ┌─ Background goroutine: SyncWorker (30s) ─────┐  │
                  │    │  SCAN redis → upsert postgres                 │  │
                  │    └──────────────────────────────────────────────┘  │
                  └──────────────────────────────────────────────────────┘
                         │               │                │
               ┌─────────┴──────┐  ┌────┴──────┐  ┌──────┴────────────────┐
               │   PostgreSQL    │  │  Redis    │  │   product-service:8081 │
               │                 │  │           │  │                        │
               │ carts           │  │cart:{uid} │  │ GET /products/{id}     │
               │ cart_items      │  │  Hash     │  │ (circuit-broken)       │
               └─────────────────┘  └───────────┘  └────────────────────────┘
```

### Key packages and files

| File | Role |
|---|---|
| `internal/repository/redis_cart_repository.go` | WATCH/MULTI/EXEC cart operations; all Redis Hash logic |
| `internal/repository/cart_repository.go` | GORM Postgres repo; used only by sync worker and ClearCart |
| `internal/service/cart_service.go` | Business logic; coordinates product validation + Redis writes |
| `internal/client/product_client.go` | HTTP client with 5s timeout, 3-attempt retry, circuit breaker |
| `internal/client/circuit_breaker.go` | Three-state CB: CLOSED → OPEN (5 failures) → HALF_OPEN (30s) |
| `internal/cache/sync.go` | Background goroutine; Redis SCAN → Postgres upsert every 30s |
| `internal/middleware/auth.go` | RS256 JWT validation; injects `uuid.UUID` into Gin context |
| `pkg/jwt/` | LoadPublicKey + ValidateToken (shared pattern with user-service) |

### Redis data model
```
Key:    cart:{userID}          (type: Hash, TTL: 7 days, refreshed on every write)
Field:  {productID as string}
Value:  {"product_name":"Widget","quantity":2,"unit_price":9.99}  (JSON)
```

### Circuit breaker states
```
CLOSED ──(5 consecutive failures)──► OPEN ──(30s elapsed)──► HALF_OPEN
  ▲                                                               │
  └──────────────── success ─────────────────────────────────────┘
                                        │
                                    failure → OPEN (reset 30s timer)
```

---

## 5. Code Examples

### WATCH/MULTI/EXEC — optimistic cart write

```go
// redis_cart_repository.go
func (r *redisCartRepository) AddOrUpdateItem(
    ctx context.Context, userID uuid.UUID, productID int64, val CartItemValue,
) error {
    key := fmt.Sprintf("cart:%s", userID)

    for range 3 { // retry up to 3 times on concurrent modification
        err := r.rdb.Watch(ctx, func(tx *redis.Tx) error {
            data, _ := json.Marshal(val)
            _, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
                pipe.HSet(ctx, key, strconv.FormatInt(productID, 10), string(data))
                pipe.Expire(ctx, key, 7*24*time.Hour) // refresh TTL
                return nil
            })
            return err
        }, key) // WATCH key — if it changes before EXEC, TxPipelined returns TxFailedErr

        if err == nil { return nil }
        if !errors.Is(err, redis.TxFailedErr) { return err }
        // redis.TxFailedErr means concurrent write: loop retries
    }
    return ErrConcurrentUpdate
}
```

### Circuit breaker — wrapping HTTP calls

```go
// product_client.go (simplified)
func (c *productClient) GetProduct(ctx context.Context, productID int64) (*ProductInfo, error) {
    if !c.cb.Allow() {
        return nil, ErrServiceUnavailable // fail fast — no HTTP call
    }

    var lastErr error
    for attempt := range 3 {
        if attempt > 0 {
            time.Sleep(time.Duration(100<<attempt) * time.Millisecond) // 200ms, 400ms
        }
        resp, err := c.httpClient.Do(req)
        if err == nil && resp.StatusCode < 500 {
            c.cb.RecordSuccess()
            return parseResponse(resp)
        }
        lastErr = ErrServiceUnavailable
    }
    c.cb.RecordFailure() // only count after all retries exhausted
    return nil, lastErr
}
```

### Background sync with graceful shutdown

```go
// cmd/server/main.go
syncCtx, syncCancel := context.WithCancel(context.Background())
go cache.StartSyncWorker(syncCtx, rdb, redisRepo, cartRepo)
defer syncCancel() // triggered on SIGTERM — stops the goroutine cleanly

// sync.go
func StartSyncWorker(ctx context.Context, ...) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C: syncAll(ctx, ...)
        case <-ctx.Done(): return // clean exit on SIGTERM
        }
    }
}
```

---

## 6. Trade-offs

### Redis-first vs. write-through to Postgres

| | Redis-first + async sync | Write-through (Redis + Postgres on every call) |
|---|---|---|
| Write latency | Sub-millisecond (Redis only) | ~5–15ms (Postgres write blocks) |
| Durability | Up to 30s of data loss if Redis dies | Zero data loss |
| Complexity | Sync goroutine, potential drift | Simpler — one write path |
| **Our choice** | ✅ Carts tolerate 30s loss (user just re-adds items) | Needed for financial data (orders, payments) |

### WATCH/MULTI/EXEC vs. Lua scripting

| | WATCH/MULTI/EXEC | Lua script (`EVAL`) |
|---|---|---|
| Atomicity | Optimistic — retries on conflict | Pessimistic — executes atomically, no retry needed |
| Network round-trips | 3 (WATCH + pipeline + result) | 1 |
| Complexity | Simpler to read and debug | More portable; harder to test |
| **Our choice** | ✅ Low contention (one user per cart key) | Better for shared counters under high concurrency |

Per-user cart keys have negligible contention (one user session at a time), so WATCH/MULTI/EXEC retries essentially never trigger in practice.

### Circuit breaker threshold: 5 failures, 30s open

Five failures (not 1) avoids false-positives from transient network blips. Thirty seconds (not 5 minutes) keeps the degradation window short — product-service restarts quickly in Docker. If the threshold were 1, any transient timeout would prevent all cart additions for 30 seconds.

---

## 7. When to Use / Avoid

### Use this pattern when:
- **High write frequency, low durability requirement**: carts are ephemeral — a user who loses 30 seconds of cart activity re-adds items and moves on. Redis absorbs the write rate effortlessly.
- **Per-user isolation**: Redis Hash `cart:{userID}` means each cart is its own key. No cross-user contention, so WATCH conflicts are nearly theoretical.
- **Dependent service can go down**: circuit breaker + price snapshotting means the cart keeps working (reads, updates, removes) even when product-service is unavailable.

### Avoid when:
- **You need full durability on every write**: if losing 30 seconds of cart data is unacceptable (e.g., high-value B2B orders), write to Postgres synchronously on every cart change.
- **Carts are shared across sessions / devices**: this design assumes one active cart per user. Multi-device sync (merge two concurrent carts) requires conflict resolution beyond WATCH/MULTI/EXEC.
- **Redis is a single point of failure without persistence**: make sure Redis is configured with AOF (`appendonly yes`) or RDB snapshots — otherwise a restart clears all carts before the sync worker can restore from Postgres.

---

## 8. Interview Insights

### Q: Why use Redis as the primary store for carts instead of just a cache?

**A:** Two reasons: access pattern and write frequency. Cart operations are per-user — `HSET cart:{userId}` — so there's no cross-user contention and no need for SQL joins or relational consistency. And carts are written constantly: every add, update, and remove triggers a write. Postgres would become the bottleneck under high concurrent users. Redis Hash operations are O(1) and sub-millisecond. Postgres is only in the path for durability (via background sync) and analytics — not the hot path.

### Q: Explain WATCH/MULTI/EXEC. Why not just use a Redis lock?

**A:** WATCH is Redis's optimistic concurrency primitive. You watch a key, do some computation, then open a transaction (MULTI) and execute it (EXEC). If the watched key changed between WATCH and EXEC — because another client modified it — EXEC returns nil and the whole transaction is discarded. You loop and retry. It's analogous to PostgreSQL's `@Version` optimistic lock: no blocking, no lock acquisition, just detect-and-retry. A Redis distributed lock (SETNX) would work but adds round-trips, TTL management, and lock release complexity. For per-user cart keys with low contention, WATCH is simpler and correct.

### Q: How does the circuit breaker prevent a cascade failure?

**A:** Without a circuit breaker, if product-service starts returning errors or timing out, every `AddItem` request in cart-service waits for the full 5-second HTTP timeout. If 100 users add items concurrently, 100 goroutines are now blocked for 5 seconds each — exhausting the goroutine pool and making the cart-service unresponsive to everything, including reads. The circuit breaker detects the failure pattern (5 consecutive errors) and trips to OPEN state: subsequent `AddItem` calls return 503 immediately without touching the network. Reads (`GetCart`, `UpdateItem`, `RemoveItem`) continue working normally because they don't call product-service at all — prices are already snapshotted in Redis.

### Q: Why does AddItem snapshot the price at time of addition instead of fetching the current price at checkout?

**A:** This is a deliberate product design decision, not a technical constraint. Showing users the price they saw when they added the item avoids a jarring experience where a price changes between browsing and checkout. It also decouples the cart from the product-service at checkout time — the cart doesn't need to make N HTTP calls for N items. The trade-off: the cart may show a stale price if a seller changes the price later. Order-service fetches the live price from product-service at order creation to reconcile any difference.

### Q: What happens if the background sync goroutine crashes?

**A:** The `syncAll` function wraps each individual cart sync in error recovery — if one cart fails, it logs and moves on; the goroutine itself never panics. The `Recovery` middleware in Gin handles panics in HTTP handlers, but the sync goroutine is separate. In production you'd add a `defer recover()` inside the goroutine, and a supervisor (like `tomb` or a restart loop) to restart it if it exits unexpectedly. For now, if it dies, Redis carts remain functional but Postgres falls out of sync until the service restarts.
