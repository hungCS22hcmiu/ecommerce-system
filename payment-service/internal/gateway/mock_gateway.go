package gateway

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/config"
)

var ErrGatewayDeclined = errors.New("gateway: payment declined")

type Gateway interface {
	Charge(ctx context.Context, amount decimal.Decimal, currency, reference string) (txnID string, err error)
}

type mockGateway struct {
	successRate  float64
	minLatencyMs int
	maxLatencyMs int
	rng          *rand.Rand
}

func NewMockGateway(cfg *config.Config) Gateway {
	return &mockGateway{
		successRate:  cfg.GatewaySuccessRate,
		minLatencyMs: cfg.GatewayMinLatencyMs,
		maxLatencyMs: cfg.GatewayMaxLatencyMs,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (g *mockGateway) Charge(ctx context.Context, amount decimal.Decimal, currency, reference string) (string, error) {
	span := g.maxLatencyMs - g.minLatencyMs
	delay := time.Duration(g.rng.Intn(span+1)+g.minLatencyMs) * time.Millisecond

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
	}

	if g.rng.Float64() >= g.successRate {
		return "", ErrGatewayDeclined
	}
	return fmt.Sprintf("MOCK-%s", uuid.NewString()), nil
}
