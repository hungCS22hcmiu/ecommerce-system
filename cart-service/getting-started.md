# Cart Service — Implementation Guide

## Context

Cart service is the bridge between browsing (product-service) and ordering (order-service). The design is **Redis-first**: all cart state lives in Redis for low-latency reads/writes, with PostgreSQL serving as a durable backup via a background goroutine. This follows the six-month-plan Phase 2 (Weeks 5–8).

**Design rationale:** Carts are per-user, write-heavy (frequent add/remove/update), and can tolerate brief inconsistency — exactly the workload Redis excels at. PostgreSQL durability prevents data loss on Redis restart.

---

## Architecture

```
Client → Gin Router → Auth Middleware → CartHandler
                                             ↓
                                       CartService
                                       /         \
                            CartRepository     ProductClient
                           (Redis + Postgres)  (HTTP to :8081)
                                  ↓
                    Background goroutine: Redis → Postgres sync
```

**Redis key:** `cart:{userId}` → Hash field `{productId}` → JSON value `{"quantity":2,"unit_price":9.99,"product_name":"..."}`  
**Concurrency:** WATCH/MULTI/EXEC on `cart:{userId}` for atomic updates

---

## File Structure to Build Out

All directories already exist (empty). Files to create:

```
cart-service/
├── internal/
│   ├── model/
│   │   └── cart.go                   # Cart + CartItem GORM models
│   ├── dto/
│   │   └── cart_dto.go               # AddItemRequest, UpdateItemRequest, CartResponse
│   ├── repository/
│   │   ├── cart_repository.go        # Interface + Postgres implementation
│   │   └── redis_cart_repository.go  # Redis WATCH/MULTI/EXEC operations
│   ├── client/
│   │   └── product_client.go         # HTTP client → product-service (5s timeout)
│   ├── service/
│   │   └── cart_service.go           # Interface + business logic
│   ├── handler/
│   │   └── cart_handler.go           # Gin HTTP handlers
│   ├── middleware/
│   │   ├── auth.go                   # JWT RS256 validation
│   │   ├── logger.go                 # Structured request logging (from user-service)
│   │   └── recovery.go              # Panic recovery (from user-service)
│   └── cache/
│       └── sync.go                   # Background goroutine: Redis → Postgres sync
└── pkg/
    └── response/
        └── response.go               # Response envelope (copy from user-service)
```

---

## Phase 1 — Core Layer (Week 5)

**Goal:** Cart CRUD working in Redis with WATCH/MULTI/EXEC, validated against product-service.

### Step 1: Models

**`internal/model/cart.go`** — match the existing migration schema exactly:

```go
type Cart struct {
    ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
    UserID    uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex"`
    Status    string     `gorm:"type:cart_status;not null;default:'ACTIVE'"`
    ExpiresAt time.Time
    CreatedAt time.Time
    UpdatedAt time.Time
    Items     []CartItem `gorm:"foreignKey:CartID;constraint:OnDelete:CASCADE"`
}

type CartItem struct {
    ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
    CartID      uuid.UUID `gorm:"type:uuid;not null;index:idx_cart_items_cart"`
    ProductID   int64     `gorm:"not null"`           // BIGINT in schema — NOT uuid
    ProductName string    `gorm:"type:varchar(200)"`
    Quantity    int       `gorm:"not null;check:quantity>0"`
    UnitPrice   float64   `gorm:"type:decimal(10,2);check:unit_price>=0"`
    AddedAt     time.Time `gorm:"not null;default:now()"`
}
```

> **Critical:** `product_id` is `BIGINT` in the migration, not UUID. Use `int64`.

### Step 2: DTOs

**`internal/dto/cart_dto.go`**:

```go
type AddItemRequest struct {
    ProductID int64 `json:"product_id" validate:"required,min=1"`
    Quantity  int   `json:"quantity"   validate:"required,min=1,max=999"`
}

type UpdateItemRequest struct {
    Quantity int `json:"quantity" validate:"required,min=1,max=999"`
}

type CartItemResponse struct {
    ProductID   int64   `json:"product_id"`
    ProductName string  `json:"product_name"`
    Quantity    int     `json:"quantity"`
    UnitPrice   float64 `json:"unit_price"`
    Subtotal    float64 `json:"subtotal"`
}

type CartResponse struct {
    UserID    string             `json:"user_id"`
    Status    string             `json:"status"`
    Items     []CartItemResponse `json:"items"`
    Total     float64            `json:"total"`
    UpdatedAt string             `json:"updated_at"`
}
```

### Step 3: Redis Repository (primary store)

**`internal/repository/redis_cart_repository.go`**

```go
// CartItemValue is the JSON value stored in Redis hash field
type CartItemValue struct {
    ProductName string  `json:"product_name"`
    Quantity    int     `json:"quantity"`
    UnitPrice   float64 `json:"unit_price"`
}

type RedisCartRepository interface {
    GetCart(ctx context.Context, userID uuid.UUID) (map[int64]CartItemValue, error)
    AddOrUpdateItem(ctx context.Context, userID uuid.UUID, productID int64, val CartItemValue) error
    RemoveItem(ctx context.Context, userID uuid.UUID, productID int64) error
    ClearCart(ctx context.Context, userID uuid.UUID) error
}
```

WATCH/MULTI/EXEC pattern for `AddOrUpdateItem`:

```go
key := fmt.Sprintf("cart:%s", userID)
field := strconv.FormatInt(productID, 10)

for retries := 0; retries < 3; retries++ {
    err := rdb.Watch(ctx, func(tx *redis.Tx) error {
        pipe := tx.TxPipeline()
        pipe.HSet(ctx, key, field, jsonVal)
        pipe.Expire(ctx, key, 7*24*time.Hour)
        _, err := pipe.Exec(ctx)
        return err
    }, key)
    if err == nil {
        return nil
    }
    if !errors.Is(err, redis.TxFailedErr) {
        return err
    }
}
return ErrConcurrentUpdate
```

### Step 4: Postgres Repository (durable backup)

**`internal/repository/cart_repository.go`**

```go
type CartRepository interface {
    UpsertCart(ctx context.Context, userID uuid.UUID) (*model.Cart, error)
    ReplaceItems(ctx context.Context, cartID uuid.UUID, items []model.CartItem) error
    GetCartWithItems(ctx context.Context, userID uuid.UUID) (*model.Cart, error)
    MarkCheckedOut(ctx context.Context, cartID uuid.UUID) error
}
```

`ReplaceItems` deletes all existing items then bulk-inserts new ones in a single transaction. Used by the background sync goroutine.

### Step 5: Product Client

**`internal/client/product_client.go`**

```go
type ProductInfo struct {
    ID     int64   `json:"id"`
    Name   string  `json:"name"`
    Price  float64 `json:"price"`
    Status string  `json:"status"`
}

type ProductClient interface {
    GetProduct(ctx context.Context, productID int64) (*ProductInfo, error)
}
```

- HTTP GET `{PRODUCT_SERVICE_URL}/api/v1/products/{id}`
- 5s timeout via `http.Client{Timeout: 5 * time.Second}`
- 404 → `ErrProductNotFound`
- 5xx or timeout → `ErrProductServiceUnavailable`
- Reject items where `Status != "ACTIVE"`

### Step 6: Cart Service

**`internal/service/cart_service.go`**

```go
var (
    ErrProductNotFound           = errors.New("product not found")
    ErrProductServiceUnavailable = errors.New("product service unavailable")
    ErrItemNotInCart             = errors.New("item not in cart")
    ErrConcurrentUpdate          = errors.New("concurrent cart update, please retry")
)

type CartService interface {
    GetCart(ctx context.Context, userID uuid.UUID) (*dto.CartResponse, error)
    AddItem(ctx context.Context, userID uuid.UUID, req dto.AddItemRequest) (*dto.CartResponse, error)
    UpdateItem(ctx context.Context, userID uuid.UUID, productID int64, req dto.UpdateItemRequest) (*dto.CartResponse, error)
    RemoveItem(ctx context.Context, userID uuid.UUID, productID int64) error
    ClearCart(ctx context.Context, userID uuid.UUID) error
}
```

`AddItem` flow:
1. `productClient.GetProduct(ctx, req.ProductID)` — reject if not found or not ACTIVE
2. `redisRepo.AddOrUpdateItem(ctx, userID, productID, {name, qty, price})`
3. Return updated cart via `GetCart`

### Step 7: Handler

**`internal/handler/cart_handler.go`**

```
POST   /api/v1/cart/items               → AddItem
DELETE /api/v1/cart/items/:productId    → RemoveItem
PUT    /api/v1/cart/items/:productId    → UpdateItem
GET    /api/v1/cart                     → GetCart
DELETE /api/v1/cart                     → ClearCart
```

All routes protected by JWT auth middleware. Extract userID with `c.Get("userID")`.

Error mapping:
```go
switch {
case errors.Is(err, service.ErrProductNotFound):
    response.NotFound(c, "product")
case errors.Is(err, service.ErrProductServiceUnavailable):
    response.Error(c, 503, "SERVICE_UNAVAILABLE", "product service unavailable", nil)
case errors.Is(err, service.ErrItemNotInCart):
    response.NotFound(c, "cart item")
case errors.Is(err, service.ErrConcurrentUpdate):
    response.Error(c, 409, "CONCURRENT_UPDATE", "concurrent update detected, please retry", nil)
default:
    response.InternalError(c)
}
```

### Step 8: Middleware

Copy `logger.go` and `recovery.go` from `user-service/internal/middleware/` verbatim.

**`internal/middleware/auth.go`** — simplified (no blacklist needed for cart):
- Extract Bearer token from `Authorization` header
- Validate RS256 JWT using public key from `cfg.JWTPublicKeyPath`
- Set `c.Set("userID", claims.UserID)` as `uuid.UUID`
- Return 401 on missing/invalid token

### Step 9: Wire `cmd/server/main.go`

Additions to the existing scaffold:

```go
// JWT public key
pubKeyBytes, _ := os.ReadFile(cfg.JWTPublicKeyPath)
publicKey, _   := jwt.ParseRSAPublicKeyFromPEM(pubKeyBytes)

// Repositories
redisRepo    := repository.NewRedisCartRepository(rdb)
cartRepo     := repository.NewCartRepository(db)

// Client + service
productClient := client.NewProductClient(cfg.ProductServiceURL)
cartSvc       := service.NewCartService(redisRepo, cartRepo, productClient)

// Background sync
syncCtx, syncCancel := context.WithCancel(context.Background())
go cache.StartSyncWorker(syncCtx, rdb, redisRepo, cartRepo)
defer syncCancel()

// Routes
cartHandler := handler.NewCartHandler(cartSvc)
authMW      := middleware.Auth(publicKey)

v1   := router.Group("/api/v1")
cart := v1.Group("/cart")
cart.Use(authMW)
cart.GET("",                     cartHandler.GetCart)
cart.DELETE("",                  cartHandler.ClearCart)
cart.POST("/items",              cartHandler.AddItem)
cart.PUT("/items/:productId",    cartHandler.UpdateItem)
cart.DELETE("/items/:productId", cartHandler.RemoveItem)
```

---

## Phase 2 — Background Sync (Week 6)

**`internal/cache/sync.go`** — Postgres durability:

```go
// Every 30 seconds, SCAN "cart:*" keys in Redis, write each cart to Postgres.
// Best-effort: log failures and continue.
func StartSyncWorker(ctx context.Context, rdb *redis.Client,
    redisRepo RedisCartRepository, cartRepo CartRepository) {

    ticker := time.NewTicker(30 * time.Second)
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

Use `rdb.Scan(ctx, 0, "cart:*", 100)` to iterate keys. For each key:
1. Parse `userID` from key suffix
2. `redisRepo.GetCart(ctx, userID)` → get items
3. `cartRepo.UpsertCart(ctx, userID)` → ensure Postgres row exists
4. `cartRepo.ReplaceItems(ctx, cartID, items)` → full replace

On `ClearCart`: delete both Redis key and Postgres rows immediately (don't wait for sync).

---

## Phase 3 — Testing (Week 6–7)

### Unit Tests
**`internal/service/cart_service_test.go`** — Mockito-style with testify/mock:

| Test | Expected |
|------|----------|
| AddItem, product exists | item in cart |
| AddItem, product 404 | ErrProductNotFound |
| AddItem, product service down | ErrProductServiceUnavailable |
| UpdateItem, item not in cart | ErrItemNotInCart |
| RemoveItem | cart updated |
| GetCart empty | empty slice (not nil) |

### Integration Tests
**`internal/integration/cart_integration_test.go`** — real Redis + Postgres via Testcontainers:

| Test | What to verify |
|------|----------------|
| WATCH/MULTI/EXEC concurrency | 10 goroutines updating same cart → consistent final state |
| Redis → Postgres sync | add items → trigger sync → Postgres matches Redis |
| Cart TTL | `cart:{userId}` expires after 7 days |
| Product 404 via httptest | mock product-service returns 404 → cart rejects |

```bash
go test -tags=integration -v -race ./internal/integration/
```

---

## Key Design Decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Primary store | Redis Hash | Per-user access, high write frequency, sub-ms latency |
| Atomicity | WATCH/MULTI/EXEC | Prevents lost updates on concurrent modifications |
| Durability | Postgres sync goroutine | Survives Redis restart without losing cart state |
| Product validation | Synchronous HTTP call | Price/existence must be current at add-time |
| Product service down | 503 to client | Don't silently add stale items; let caller retry |
| Cart TTL in Redis | 7 days | Matches `expires_at` default in Postgres schema |
| Auth | RS256 JWT, no blacklist | Lower security risk than auth — skip blacklist overhead |

---

## Reference Files

| File | Why |
|------|-----|
| `user-service/internal/handler/auth_handler.go` | Handler pattern to replicate |
| `user-service/internal/service/auth_service.go` | Sentinel error pattern |
| `user-service/internal/repository/user_repository.go` | Repository + ErrNotFound mapping |
| `user-service/internal/middleware/auth_middleware.go` | JWT middleware to adapt |
| `user-service/pkg/response/response.go` | **Copy verbatim** — identical envelope |
| `cart-service/migrations/000001_baseline_schema.up.sql` | Column types to match in GORM models |
| `cart-service/cmd/server/main.go` | Add wiring here (DB + Redis already set up) |
| `cart-service/config/config.go` | `PRODUCT_SERVICE_URL` + `JWT_PUBLIC_KEY_PATH` already present |

---

## Dependencies to Add

```bash
cd cart-service
go get github.com/golang-jwt/jwt/v5
```

Everything else is already in `go.mod`: Gin, GORM, `redis/go-redis/v9`, `go-playground/validator/v10`.

---

## Quick Verification

```bash
# Start dependencies
docker compose up -d postgres redis product-service

# Run service
cd cart-service && go run ./cmd/server/main.go

# Health check
curl http://localhost:8002/health/ready

# Login and get token (sample_users.sql: customer@example.com / Password123!)
TOKEN=$(curl -s -X POST http://localhost:8001/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"customer@example.com","password":"Password123!"}' \
  | jq -r '.data.access_token')

# Add item to cart
curl -X POST http://localhost:8002/api/v1/cart/items \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"product_id": 1, "quantity": 2}'

# Get cart
curl http://localhost:8002/api/v1/cart \
  -H "Authorization: Bearer $TOKEN"

# Unit tests
go test -race ./...

# Integration tests (requires Docker)
go test -tags=integration -v -race ./internal/integration/
```
