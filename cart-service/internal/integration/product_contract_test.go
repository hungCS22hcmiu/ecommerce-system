//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/client"
)

// fullProductResponse is the complete shape the Java product-service returns.
// It has 15 fields; ProductInfo only captures 4 — this tests Go's forward-compat with unknown fields.
var fullProductResponse = map[string]any{
	"success": true,
	"data": map[string]any{
		"id":             float64(1), // JSON numbers decode as float64
		"name":           "Test Widget",
		"description":    "A test product",
		"price":          9.99,
		"categoryId":     float64(100),
		"categoryName":   "Electronics",
		"sellerId":       "550e8400-e29b-41d4-a716-446655440000",
		"status":         "ACTIVE",
		"stockQuantity":  float64(50),
		"stockReserved":  float64(5),
		"stockAvailable": float64(45),
		"version":        float64(1),
		"images":         []any{},
		"createdAt":      "2026-03-02T10:00:00Z",
		"updatedAt":      "2026-03-02T10:00:00Z",
	},
}

func serveJSON(v any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
}

// TestProductContract_FullResponseParsed verifies the client correctly maps the 4 fields it
// cares about from a full 15-field Java response, and silently drops the rest.
func TestProductContract_FullResponseParsed(t *testing.T) {
	srv := httptest.NewServer(serveJSON(fullProductResponse))
	defer srv.Close()

	c := client.NewProductClient(srv.URL)
	result, err := c.GetProduct(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, int64(1), result.ID)
	assert.Equal(t, "Test Widget", result.Name)
	assert.Equal(t, 9.99, result.Price)
	assert.Equal(t, "ACTIVE", result.Status)
}

// TestProductContract_ExtraFieldsIgnored proves the client handles future product-service
// schema additions without breaking — the consumer contract only cares about id/name/price/status.
func TestProductContract_ExtraFieldsIgnored(t *testing.T) {
	resp := map[string]any{
		"success": true,
		"data": map[string]any{
			"id":          float64(1),
			"name":        "Test Widget",
			"description": "A test product",
			"price":       9.99,
			"categoryId":  float64(100),
			"status":      "ACTIVE",
			"futureField": "v2-value", // hypothetical new field in a future sprint
			"anotherNew":  float64(42),
		},
	}
	srv := httptest.NewServer(serveJSON(resp))
	defer srv.Close()

	c := client.NewProductClient(srv.URL)
	result, err := c.GetProduct(context.Background(), 1)

	require.NoError(t, err, "client must tolerate unknown fields (forward-compatibility)")
	assert.Equal(t, int64(1), result.ID)
	assert.Equal(t, "Test Widget", result.Name)
}

func TestProductContract_404ReturnsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
	}))
	defer srv.Close()

	c := client.NewProductClient(srv.URL)
	result, err := c.GetProduct(context.Background(), 999)

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, client.ErrNotFound), "404 response must map to ErrNotFound, got: %v", err)
}

func TestProductContract_InactiveStatusReturnsErrNotFound(t *testing.T) {
	resp := map[string]any{
		"success": true,
		"data": map[string]any{
			"id": float64(1), "name": "Widget", "price": 9.99, "status": "INACTIVE",
		},
	}
	srv := httptest.NewServer(serveJSON(resp))
	defer srv.Close()

	c := client.NewProductClient(srv.URL)
	result, err := c.GetProduct(context.Background(), 1)

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, client.ErrNotFound), "INACTIVE product must map to ErrNotFound")
}

func TestProductContract_PendingStatusReturnsErrNotFound(t *testing.T) {
	resp := map[string]any{
		"success": true,
		"data": map[string]any{
			"id": float64(1), "name": "Widget", "price": 9.99, "status": "PENDING",
		},
	}
	srv := httptest.NewServer(serveJSON(resp))
	defer srv.Close()

	c := client.NewProductClient(srv.URL)
	result, err := c.GetProduct(context.Background(), 1)

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, client.ErrNotFound), "any non-ACTIVE status must map to ErrNotFound")
}

// TestProductContract_5xxReturnsErrServiceUnavailable verifies the 3-attempt retry policy.
// Note: this test takes ~600ms due to retry backoffs (200ms + 400ms between attempts).
func TestProductContract_5xxReturnsErrServiceUnavailable(t *testing.T) {
	var hitCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := client.NewProductClient(srv.URL)
	result, err := c.GetProduct(context.Background(), 1)

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, client.ErrServiceUnavailable))
	assert.Equal(t, int32(3), hitCount.Load(), "client must retry exactly 3 times on 5xx before giving up")
}

// TestProductContract_CircuitBreakerOpens verifies the circuit breaker opens after 5 exhausted
// retry cycles and stops sending HTTP requests until the cool-down period elapses.
// Note: 5 retry cycles × ~600ms backoff ≈ 3 seconds total.
func TestProductContract_CircuitBreakerOpens(t *testing.T) {
	var hitCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := client.NewProductClient(srv.URL)

	// Each call exhausts 3 retries and calls RecordFailure once.
	// After 5 such calls the circuit opens (threshold=5).
	for i := 0; i < 5; i++ {
		_, _ = c.GetProduct(context.Background(), 1)
	}
	hitsBeforeSixth := hitCount.Load()

	// With the circuit open the client must short-circuit immediately.
	result, err := c.GetProduct(context.Background(), 1)

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, client.ErrServiceUnavailable), "open circuit must return ErrServiceUnavailable")
	assert.Equal(t, hitsBeforeSixth, hitCount.Load(), "open circuit must not send any HTTP requests")
}

// TestProductContract_SuccessEnvelopeFalse verifies that a 200 response with success=false
// is treated as an error (the Java service can return this for application-level errors).
func TestProductContract_SuccessEnvelopeFalse(t *testing.T) {
	resp := map[string]any{
		"success": false,
		"data": map[string]any{
			"id": float64(1), "name": "Widget", "price": 9.99, "status": "ACTIVE",
		},
	}
	srv := httptest.NewServer(serveJSON(resp))
	defer srv.Close()

	c := client.NewProductClient(srv.URL)
	result, err := c.GetProduct(context.Background(), 1)

	assert.Nil(t, result)
	assert.NotNil(t, err, "success=false envelope must return a non-nil error")
}
