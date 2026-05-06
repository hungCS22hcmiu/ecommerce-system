package event

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// OrderItemEvent mirrors order-service OrderCreatedEvent.OrderItemEvent (Spring/Jackson camelCase).
type OrderItemEvent struct {
	ProductID int64           `json:"productId"`
	Quantity  int             `json:"quantity"`
	UnitPrice decimal.Decimal `json:"unitPrice"`
}

// OrderCreatedEvent mirrors order-service OrderCreatedEvent published to orders.created.
// Spring Jackson serialises UUIDs as strings and BigDecimal as JSON numbers;
// uuid.UUID and decimal.Decimal both unmarshal those forms natively.
type OrderCreatedEvent struct {
	OrderID     uuid.UUID        `json:"orderId"`
	UserID      uuid.UUID        `json:"userId"`
	TotalAmount decimal.Decimal  `json:"totalAmount"`
	Items       []OrderItemEvent `json:"items"`
}

// PaymentCompletedEvent is published to payments.completed.
// order-service PaymentEventConsumer expects fields: orderId, paymentId, amount.
type PaymentCompletedEvent struct {
	OrderID   uuid.UUID       `json:"orderId"`
	PaymentID uuid.UUID       `json:"paymentId"`
	Amount    decimal.Decimal `json:"amount"`
}

// PaymentFailedEvent is published to payments.failed.
// order-service PaymentEventConsumer expects fields: orderId, reason.
type PaymentFailedEvent struct {
	OrderID uuid.UUID `json:"orderId"`
	Reason  string    `json:"reason"`
}
