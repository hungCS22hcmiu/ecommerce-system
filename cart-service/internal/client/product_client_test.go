package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mapCache is a test double for productCache backed by an in-memory map.
type mapCache struct{ m map[int64]*ProductInfo }

func (c *mapCache) get(_ context.Context, id int64) (*ProductInfo, error) {
	return c.m[id], nil // nil value → cache miss
}

func (c *mapCache) set(_ context.Context, id int64, info *ProductInfo, _ time.Duration) error {
	c.m[id] = info
	return nil
}

func TestGetProduct_CacheHit_SkipsHTTP(t *testing.T) {
	httpCalls := 0
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer stub.Close()

	info := &ProductInfo{ID: 1, Name: "Cached Product", Price: 9.99, Status: "ACTIVE", StockAvailable: 5, SellerID: "s1"}
	pc := &productClient{
		baseURL:    stub.URL,
		httpClient: stub.Client(),
		cb:         NewCircuitBreaker(5, 30*time.Second),
		cache:      &mapCache{m: map[int64]*ProductInfo{1: info}},
	}

	result, err := pc.GetProduct(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, info, result)
	assert.Equal(t, 0, httpCalls, "cache hit must not make any HTTP call")
}

func TestGetProduct_CacheMiss_PopulatesCache(t *testing.T) {
	product := ProductInfo{ID: 2, Name: "Fresh Product", Price: 19.99, Status: "ACTIVE", StockAvailable: 10, SellerID: "s2"}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": product})
	}))
	defer stub.Close()

	cache := &mapCache{m: map[int64]*ProductInfo{}}
	pc := &productClient{
		baseURL:    stub.URL,
		httpClient: stub.Client(),
		cb:         NewCircuitBreaker(5, 30*time.Second),
		cache:      cache,
	}

	result, err := pc.GetProduct(context.Background(), 2)
	require.NoError(t, err)
	assert.Equal(t, "Fresh Product", result.Name)
	assert.NotNil(t, cache.m[2], "successful HTTP response must populate the cache")
	assert.Equal(t, "Fresh Product", cache.m[2].Name)
}
