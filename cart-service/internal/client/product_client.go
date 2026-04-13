package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	baseURL string
	httpClient  *http.Client
}

func NewProductClient(baseURL string) ProductClient {
	return &productClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *productClient) GetProduct(ctx context.Context, productID int64) (*ProductInfo, error) {
	url := fmt.Sprintf("%s/api/v1/products/%d", c.baseURL, productID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}

	if resp.StatusCode >= 500 {
		return nil, ErrServiceUnavailable
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Success bool        `json:"success"`
		Data    ProductInfo `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success {
		return nil, fmt.Errorf("product service returned success=false")
	}

	if result.Data.Status != "ACTIVE" {
		return nil, ErrNotFound
	}

	return &result.Data, nil
}
