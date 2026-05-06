package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/model"
)

type PaymentResponse struct {
	ID               uuid.UUID       `json:"id"`
	OrderID          uuid.UUID       `json:"orderId"`
	UserID           uuid.UUID       `json:"userId"`
	Amount           decimal.Decimal `json:"amount"`
	Currency         string          `json:"currency"`
	Status           string          `json:"status"`
	Method           string          `json:"method"`
	GatewayReference string          `json:"gatewayReference,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
}

type ProcessPaymentRequest struct {
	OrderID        uuid.UUID       `json:"orderId"        validate:"required"`
	UserID         uuid.UUID       `json:"userId"         validate:"required"`
	Amount         decimal.Decimal `json:"amount"         validate:"required"`
	Currency       string          `json:"currency"       validate:"required,len=3"`
	IdempotencyKey string          `json:"idempotencyKey" validate:"required"`
}

type PaymentHistoryEntry struct {
	OldStatus string    `json:"oldStatus,omitempty"`
	NewStatus string    `json:"newStatus"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func ToPaymentResponse(p *model.Payment) PaymentResponse {
	return PaymentResponse{
		ID:               p.ID,
		OrderID:          p.OrderID,
		UserID:           p.UserID,
		Amount:           p.Amount,
		Currency:         p.Currency,
		Status:           string(p.Status),
		Method:           string(p.Method),
		GatewayReference: p.GatewayReference,
		CreatedAt:        p.CreatedAt,
	}
}
