# How Cart Service Works

## Overview

Cart-service is a Go microservice (Gin + GORM) running on port 8002. It manages shopping carts with a **Redis-first** storage strategy: all cart operations hit Redis for low latency, while PostgreSQL serves as durable backup kept in sync by a background goroutine.

---

## Request Flow

```
Client
  │
  │  HTTP request with Authorization: Bearer <token>
  ▼
Gin Router
  │
  ├── GET  /health/live    ─── no auth, always 200
  ├── GET  /health/ready   ─── no auth, checks Postgres + Redis
  │
  └── /api/v1/cart (all routes)
        │
        ▼
     Auth Middleware          ← validates JWT from user-service
        │
        ▼
     Logger Middleware        ← structured JSON log per request
        │
        ▼
     CartHandler              ← parses request, calls service
        │
        ▼
     CartService              ← business logic
       /         \
  RedisRepo    ProductClient
  (primary)    (HTTP → :8081)
       │
  CartRepo (Postgres)
  (sync / clear)
```

---

## Storage Strategy: Redis-First

### Redis (Primary)

Every cart is stored as a **Redis Hash** at key `cart:{userID}`:
- Each hash **field** is the `productID` (int64 as string)
- Each hash **value** is a JSON blob: `{"product_name":"...","quantity":2,"unit_price":9.99}`
- TTL is **7 days**, refreshed on every write
- All writes use **WATCH/MULTI/EXEC** for optimistic concurrency (retries 3×, returns `ErrConcurrentUpdate` on exhaustion)

```
Redis key:    cart:550e8400-e29b-41d4-a716-446655440000
Hash field:   "1"  → {"product_name":"Widget","quantity":2,"unit_price":9.99}
Hash field:   "5"  → {"product_name":"Gadget","quantity":1,"unit_price":24.99}
```

### PostgreSQL (Durable Backup)

Schema: `carts` (one row per user, status = ACTIVE/CHECKED_OUT/ABANDONED) and `cart_items` (one row per product in cart). A unique index on `(cart_id, product_id)` prevents duplicates.

Postgres is **not** written on every API call. It is updated by:
1. **Background sync** — every 30 seconds, full replace of items from Redis
2. **ClearCart** — deletes both Redis key and Postgres rows immediately (can't wait for sync)

If Redis is restarted, the next sync pass re-populates it from Postgres is not automatic — but Postgres retains the last synced state, limiting data loss to at most 30 seconds of activity.

---

## How JWT Works (Integration with user-service)

### The Trust Chain

Cart-service never issues tokens. It only **validates** tokens that user-service already issued. The trust is established by sharing the RSA public key — both services mount the same key directory.

```
user-service                         cart-service
────────────                         ────────────
private.pem  ──── signs JWT ────►   (never sees private key)
public.pem   ─── mounted via ───►   public.pem
              docker volume          validates JWT signature
```

In `docker-compose.yml`:
```yaml
cart-service:
  volumes:
    - ./user-service/keys:/app/keys:ro   # same public key as user-service
```

### Token Structure

User-service signs RS256 JWTs with this payload (mirrored in `pkg/jwt/claims.go`):

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "email":   "customer@example.com",
  "role":    "CUSTOMER",
  "exp":     1712345678,
  "iat":     1712344778
}
```

`Claims.UserID` is a **string** in the JWT payload (not a UUID type).

### Validation Flow in Auth Middleware (`internal/middleware/auth.go`)

```
Request arrives with:  Authorization: Bearer eyJhbGc...

1. Extract token string after "Bearer "
2. jwtpkg.ValidateToken(tokenStr, publicKey)
      └─ jwt.ParseWithClaims → verifies RS256 signature using public.pem
      └─ checks expiry (exp claim) automatically
      └─ rejects any token not signed with the matching private key
3. uuid.Parse(claims.UserID)  ← convert string → uuid.UUID
4. c.Set("userID", userID)    ← stored as uuid.UUID in Gin context
5. c.Set("role", claims.Role)
6. c.Next()                   ← pass to handler

On failure → 401 UNAUTHORIZED, request aborted
```

Key difference from user-service: **no blacklist check**. Cart-service does not call Redis to check if the token was revoked. Design rationale: carts are low-security compared to auth — the 15-minute access token TTL is sufficient protection.

### How Handlers Read the User ID

```go
// In CartHandler — every route starts with this:
val, exists := c.Get("userID")          // set by Auth middleware
userID, ok  := val.(uuid.UUID)          // type-assert to uuid.UUID
```

The type assertion works only because the middleware stored it as `uuid.UUID` (not as `string`). If the assertion fails, the handler returns 500.

---

## Integration with product-service

When adding an item (`POST /api/v1/cart/items`), cart-service makes a synchronous HTTP call to product-service to:
1. Confirm the product exists
2. Confirm it is `ACTIVE` (not deleted or inactive)
3. Fetch the current price and name to snapshot into the cart

```go
// internal/client/product_client.go
GET {PRODUCT_SERVICE_URL}/api/v1/products/{id}
Timeout: 5 seconds
```

Response mapping:
- `200 + status=ACTIVE` → use the price/name
- `200 + status≠ACTIVE` → treated as 404 (`ErrNotFound`)
- `404` → `ErrNotFound` → handler returns 404 to client
- `5xx` or timeout → `ErrServiceUnavailable` → handler returns 503 to client

**Why synchronous?** Price and existence must be validated at the moment of add. Adding with a stale price is worse than a brief 503 — the user retries when the product-service recovers.

**Note on UpdateItem:** Quantity updates do **not** call product-service. The price and name are already snapshotted in Redis from when the item was originally added.

---

## Background Sync Worker (`internal/cache/sync.go`)

Runs as a goroutine started in `main.go`, cancelled on graceful shutdown.

```
Every 30 seconds:
  SCAN "cart:*" keys in Redis (cursor-based, batches of 100)
    For each key "cart:{uuid}":
      1. Parse UUID from key suffix
      2. GetCart → map[productID]CartItemValue from Redis
      3. If empty → skip (avoids ghost Postgres rows)
      4. UpsertCart(userID) → get or create Postgres carts row
      5. Convert map → []model.CartItem
      6. ReplaceItems(cartID, items) → DELETE existing + bulk INSERT in one TX
      7. On error → log and continue (best-effort, goroutine never dies)
```

`SyncOnce` is also exported for integration tests to trigger a sync pass on demand without waiting for the ticker.

---

## Wiring in `cmd/server/main.go`

Startup sequence:
1. Connect PostgreSQL (GORM) — fail-fast
2. Run database migrations (`golang-migrate`) — apply pending `.up.sql` files
3. Connect Redis — ping check, fail-fast
4. Load RSA public key from `cfg.JWTPublicKeyPath` — fail-fast
5. Build dependency graph: `RedisRepo → CartRepo → ProductClient → CartService`
6. Start background sync goroutine with cancellable context
7. Register Gin routes with Auth + Logger + Recovery middleware
8. Start HTTP server on `PORT` (default 8002)
9. On SIGINT/SIGTERM: graceful 30s shutdown, close DB and Redis connections

---

## Error Handling

| Service error | HTTP status | Code |
|---|---|---|
| `ErrProductNotFound` | 404 | `NOT_FOUND` |
| `ErrProductServiceUnavailable` | 503 | `SERVICE_UNAVAILABLE` |
| `ErrItemNotInCart` | 404 | `NOT_FOUND` |
| `ErrConcurrentUpdate` | 409 | `CONCURRENT_UPDATE` |
| Any other error | 500 | `INTERNAL_ERROR` |
| Missing/invalid JWT | 401 | `UNAUTHORIZED` |
| Validation failure | 400 | `VALIDATION_ERROR` |

All responses use the standard envelope:
```json
{ "success": true,  "data": { ... } }
{ "success": false, "error": { "code", "message", "timestamp", "path" } }
```

---

## Concurrency Model

| Operation | Mechanism | Reason |
|---|---|---|
| Cart read | `HGETALL` | Atomic hash read |
| Cart write | `WATCH / MULTI / EXEC` (3 retries) | Prevents lost updates from concurrent requests |
| Sync write | Full `ReplaceItems` in Postgres TX | Consistent snapshot, avoids partial updates |
| Clear cart | Delete Redis + Postgres immediately | Cannot wait 30s for sync — user expects instant clear |
