//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/cache"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/client"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/handler"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/middleware"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/service"
	jwtpkg "github.com/hungCS22hcmiu/ecommrece-system/cart-service/pkg/jwt"
)

// ─── Shared test infrastructure ───────────────────────────────────────────────

var (
	testServer     *httptest.Server
	testProductSrv *httptest.Server
	testRDB        *redis.Client
	testDB         *gorm.DB
	testRSAKey     *rsa.PrivateKey
	testRedisRepo  repository.RedisCartRepository
	testCartRepo   repository.CartRepository
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	// ── RSA key pair for test JWTs ──────────────────────────────────────────
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Printf("SKIP: cannot generate RSA key: %v\n", err)
		os.Exit(0)
	}
	testRSAKey = privateKey

	// ── Postgres ─────────────────────────────────────────────────────────────
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "5432"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", "postgres"),
		getEnv("DB_NAME", "ecommerce_carts"),
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Printf("SKIP: cannot connect to postgres: %v\n", err)
		os.Exit(0)
	}
	testDB = db

	// ── Redis ─────────────────────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", getEnv("REDIS_HOST", "localhost"), getEnv("REDIS_PORT", "6379")),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       1, // use DB 1 to avoid polluting DB 0 used by real carts
	})
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		fmt.Printf("SKIP: cannot connect to redis: %v\n", err)
		os.Exit(0)
	}
	testRDB = rdb

	// ── Mock product-service ──────────────────────────────────────────────────
	testProductSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve a fixed active product for any product ID
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/products/404":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":     1,
					"name":   "Test Widget",
					"price":  9.99,
					"status": "ACTIVE",
				},
			})
		}
	}))

	// ── Repositories + service ────────────────────────────────────────────────
	testRedisRepo = repository.NewRedisCartRepository(rdb)
	testCartRepo = repository.NewCartRepository(db)
	productClient := client.NewProductClient(testProductSrv.URL)
	cartSvc := service.NewCartService(testRedisRepo, testCartRepo, productClient)

	// ── Router ────────────────────────────────────────────────────────────────
	router := gin.New()
	cartHandler := handler.NewCartHandler(cartSvc)
	authMW := middleware.Auth(&privateKey.PublicKey)

	v1 := router.Group("/api/v1")
	cart := v1.Group("/cart")
	cart.Use(authMW)
	cart.GET("", cartHandler.GetCart)
	cart.DELETE("", cartHandler.ClearCart)
	cart.POST("/items", cartHandler.AddItem)
	cart.PUT("/items/:productId", cartHandler.UpdateItem)
	cart.DELETE("/items/:productId", cartHandler.RemoveItem)

	testServer = httptest.NewServer(router)

	// ── Run tests ─────────────────────────────────────────────────────────────
	code := m.Run()

	// ── Cleanup ───────────────────────────────────────────────────────────────
	testServer.Close()
	testProductSrv.Close()
	rdb.FlushDB(context.Background())
	rdb.Close()

	os.Exit(code)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// signToken creates a signed JWT for the given userID using the test RSA key.
func signToken(userID uuid.UUID) string {
	claims := jwtpkg.Claims{
		UserID: userID.String(),
		Email:  "test@example.com",
		Role:   "CUSTOMER",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, _ := token.SignedString(testRSAKey)
	return signed
}

// doRequest sends an authenticated request and returns the response.
func doRequest(method, path string, body any, userID uuid.UUID) *http.Response {
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req, _ := http.NewRequest(method, testServer.URL+path, reqBody)
	req.Header.Set("Authorization", "Bearer "+signToken(userID))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := http.DefaultClient.Do(req)
	return resp
}

// cleanupUser removes all test data for a given userID.
func cleanupUser(userID uuid.UUID) {
	testRDB.Del(context.Background(), fmt.Sprintf("cart:%s", userID))
	testDB.Exec("DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id = ?)", userID)
	testDB.Exec("DELETE FROM carts WHERE user_id = ?", userID)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestAddItem_Integration(t *testing.T) {
	userID := uuid.New()
	defer cleanupUser(userID)

	resp := doRequest("POST", "/api/v1/cart/items",
		map[string]any{"product_id": 1, "quantity": 2}, userID)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Redis key must exist
	key := fmt.Sprintf("cart:%s", userID)
	exists, err := testRDB.Exists(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)
}

func TestGetCart_Integration(t *testing.T) {
	userID := uuid.New()
	defer cleanupUser(userID)

	// Add two items (mock product-service returns same product for any ID)
	doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 1, "quantity": 2}, userID)
	doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 2, "quantity": 3}, userID)

	resp := doRequest("GET", "/api/v1/cart", nil, userID)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Items []any   `json:"items"`
			Total float64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.True(t, result.Success)
	assert.Len(t, result.Data.Items, 2)
	assert.Greater(t, result.Data.Total, 0.0)
}

func TestUpdateItem_Integration(t *testing.T) {
	userID := uuid.New()
	defer cleanupUser(userID)

	doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 1, "quantity": 2}, userID)

	resp := doRequest("PUT", "/api/v1/cart/items/1", map[string]any{"quantity": 7}, userID)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Data struct {
			Items []struct {
				ProductID int64 `json:"product_id"`
				Quantity  int   `json:"quantity"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.Len(t, result.Data.Items, 1)
	assert.Equal(t, 7, result.Data.Items[0].Quantity)
}

func TestRemoveItem_Integration(t *testing.T) {
	userID := uuid.New()
	defer cleanupUser(userID)

	doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 1, "quantity": 2}, userID)

	resp := doRequest("DELETE", "/api/v1/cart/items/1", nil, userID)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Redis hash field must be gone
	key := fmt.Sprintf("cart:%s", userID)
	count, err := testRDB.HLen(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestClearCart_Integration(t *testing.T) {
	userID := uuid.New()
	defer cleanupUser(userID)

	doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 1, "quantity": 2}, userID)
	doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 2, "quantity": 1}, userID)

	// Trigger sync so Postgres has rows to delete
	cache.SyncOnce(context.Background(), testRDB, testRedisRepo, testCartRepo)

	resp := doRequest("DELETE", "/api/v1/cart", nil, userID)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Redis key must be gone
	key := fmt.Sprintf("cart:%s", userID)
	exists, _ := testRDB.Exists(context.Background(), key).Result()
	assert.Equal(t, int64(0), exists)

	// Postgres rows must be gone
	var count int64
	testDB.Raw("SELECT COUNT(*) FROM carts WHERE user_id = ?", userID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestProductNotFound_Integration(t *testing.T) {
	userID := uuid.New()
	defer cleanupUser(userID)

	// product_id 404 makes the mock product-service return 404
	resp := doRequest("POST", "/api/v1/cart/items",
		map[string]any{"product_id": 404, "quantity": 1}, userID)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestConcurrentAdd_Integration(t *testing.T) {
	userID := uuid.New()
	defer cleanupUser(userID)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			doRequest("POST", "/api/v1/cart/items",
				map[string]any{"product_id": 1, "quantity": 1}, userID)
		}()
	}
	wg.Wait()

	// Redis must have exactly 1 field for product 1 (last writer wins — no lost updates)
	key := fmt.Sprintf("cart:%s", userID)
	count, err := testRDB.HLen(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestRedisToPostgresSync_Integration(t *testing.T) {
	userID := uuid.New()
	defer cleanupUser(userID)

	// Add items via HTTP
	doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 1, "quantity": 3}, userID)
	doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 2, "quantity": 1}, userID)

	// Run sync manually
	cache.SyncOnce(context.Background(), testRDB, testRedisRepo, testCartRepo)

	// Postgres must now have 2 items for this user
	var count int64
	testDB.Raw(`
		SELECT COUNT(ci.*) FROM cart_items ci
		JOIN carts c ON c.id = ci.cart_id
		WHERE c.user_id = ?
	`, userID).Scan(&count)

	assert.Equal(t, int64(2), count)
}

func TestCartTTL_Integration(t *testing.T) {
	userID := uuid.New()
	defer cleanupUser(userID)

	doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 1, "quantity": 1}, userID)

	key := fmt.Sprintf("cart:%s", userID)
	ttl, err := testRDB.TTL(context.Background(), key).Result()
	require.NoError(t, err)

	// TTL should be close to 7 days (within a 10-second margin)
	sevenDays := 7 * 24 * time.Hour
	assert.Greater(t, ttl, sevenDays-10*time.Second)
	assert.LessOrEqual(t, ttl, sevenDays)
}

// TestDegradedMode_CircuitOpen_ReadOpsStillWork proves the degraded-mode guarantee:
// when the product-service circuit breaker is OPEN, AddItem fails fast (no HTTP made),
// but GetCart / UpdateItem / RemoveItem / ClearCart continue to work because they are
// pure Redis paths that bypass the product client entirely.
//
// The shared testRedisRepo / testCartRepo vars mean items added through the working
// testServer are immediately visible to the broken service instance.
//
// Note: opening the circuit requires 5 × 3-retry calls (~3 s total due to backoffs).
func TestDegradedMode_CircuitOpen_ReadOpsStillWork(t *testing.T) {
	userID := uuid.New()
	defer cleanupUser(userID)
	ctx := context.Background()

	// Step 1: Pre-populate cart via the working testServer (circuit closed, products resolve).
	resp := doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 1, "quantity": 2}, userID)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "pre-populate product 1")
	resp.Body.Close()

	resp = doRequest("POST", "/api/v1/cart/items", map[string]any{"product_id": 2, "quantity": 3}, userID)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "pre-populate product 2")
	resp.Body.Close()

	// Step 2: Build a broken product service that always returns 500.
	var hitCount atomic.Int32
	brokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hitCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer brokenSrv.Close()

	// Step 3: Build a cart service that uses the broken client but shares repos with testServer.
	brokenClient := client.NewProductClient(brokenSrv.URL)
	brokenSvc := service.NewCartService(testRedisRepo, testCartRepo, brokenClient)

	// Step 4: Open the circuit — 5 calls × 3 retries each = RecordFailure × 5 → OPEN.
	for i := 0; i < 5; i++ {
		_, _ = brokenClient.GetProduct(ctx, 1)
	}
	hitsToOpen := hitCount.Load()
	require.Equal(t, int32(15), hitsToOpen, "5 calls × 3 retries must produce exactly 15 server hits")

	// Step 5: AddItem must fail fast — circuit is OPEN, no HTTP request made.
	_, err := brokenSvc.AddItem(ctx, userID, dto.AddItemRequest{ProductID: 3, Quantity: 1})
	assert.ErrorIs(t, err, service.ErrProductServiceUnavailable, "AddItem must return ErrProductServiceUnavailable when circuit is open")
	assert.Equal(t, hitsToOpen, hitCount.Load(), "open circuit: AddItem must not hit the product server")

	// Step 6: GetCart must succeed — pure Redis read, no product client involved.
	cart, err := brokenSvc.GetCart(ctx, userID)
	require.NoError(t, err, "GetCart must work in degraded mode")
	assert.Len(t, cart.Items, 2, "GetCart must return the 2 pre-populated items")

	// Step 7: UpdateItem must succeed — pure Redis update, no product client involved.
	cart, err = brokenSvc.UpdateItem(ctx, userID, 1, dto.UpdateItemRequest{Quantity: 5})
	require.NoError(t, err, "UpdateItem must work in degraded mode")
	var found bool
	for _, item := range cart.Items {
		if item.ProductID == 1 {
			assert.Equal(t, 5, item.Quantity, "product 1 quantity must be updated to 5")
			found = true
		}
	}
	assert.True(t, found, "product 1 must still be in cart after UpdateItem")

	// Step 8: RemoveItem must succeed — pure Redis delete.
	err = brokenSvc.RemoveItem(ctx, userID, 1)
	require.NoError(t, err, "RemoveItem must work in degraded mode")
	cart, err = brokenSvc.GetCart(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, cart.Items, 1, "cart must have 1 item after removing product 1")

	// Step 9: ClearCart must succeed — clears both Redis and Postgres.
	err = brokenSvc.ClearCart(ctx, userID)
	require.NoError(t, err, "ClearCart must work in degraded mode")
	cart, err = brokenSvc.GetCart(ctx, userID)
	require.NoError(t, err)
	assert.Empty(t, cart.Items, "cart must be empty after ClearCart")
}
