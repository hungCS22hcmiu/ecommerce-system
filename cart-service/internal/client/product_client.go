package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

var (
	ErrNotFound           = errors.New("product not found")
	ErrServiceUnavailable = errors.New("product service unavailable")
)

type ProductInfo struct {
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	Price  float64 `json:"price"`
	Status string  `json:"status"`
}

type ProductClient interface {
	GetProduct(ctx context.Context, productID int64) (*ProductInfo, error)
}

type productClient struct {
	baseURL    string
	httpClient *http.Client
	cb         *CircuitBreaker
}

func NewProductClient(baseURL string) ProductClient {
	return &productClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		// Open after 5 consecutive failures; stay open for 30 seconds.
		cb: NewCircuitBreaker(5, 30*time.Second),
	}
}

func (c *productClient) GetProduct(ctx context.Context, productID int64) (*ProductInfo, error) {
	if !c.cb.Allow() {
		return nil, ErrServiceUnavailable
	}

	url := fmt.Sprintf("%s/api/v1/products/%d", c.baseURL, productID)

	const maxAttempts = 3
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(100<<attempt) * time.Millisecond): // 200ms, 400ms
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Retry on network/timeout errors.
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				lastErr = ErrServiceUnavailable
				continue
			}
			lastErr = ErrServiceUnavailable
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			// 404 is definitive — don't retry, don't count as circuit failure.
			c.cb.RecordSuccess()
			return nil, ErrNotFound
		}

		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = ErrServiceUnavailable
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			// Non-retryable unexpected status.
			c.cb.RecordSuccess()
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		var result struct {
			Success bool        `json:"success"`
			Data    ProductInfo `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			c.cb.RecordSuccess()
			return nil, err
		}
		resp.Body.Close()

		if !result.Success {
			c.cb.RecordSuccess()
			return nil, fmt.Errorf("product service returned success=false")
		}

		if result.Data.Status != "ACTIVE" {
			c.cb.RecordSuccess()
			return nil, ErrNotFound
		}

		c.cb.RecordSuccess()
		return &result.Data, nil
	}

	// All attempts exhausted — record failure and return.
	c.cb.RecordFailure()
	return nil, lastErr
}
