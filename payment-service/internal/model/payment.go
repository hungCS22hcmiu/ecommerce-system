package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "PENDING"
	PaymentStatusCompleted PaymentStatus = "COMPLETED"
	PaymentStatusFailed    PaymentStatus = "FAILED"
	PaymentStatusRefunded  PaymentStatus = "REFUNDED"
)

func (s PaymentStatus) Value() (driver.Value, error) {
	return string(s), nil
}

func (s *PaymentStatus) Scan(src interface{}) error {
	str, ok := src.(string)
	if !ok {
		return fmt.Errorf("PaymentStatus.Scan: expected string, got %T", src)
	}
	*s = PaymentStatus(str)
	return nil
}

type PaymentMethod string

const (
	PaymentMethodMockCard   PaymentMethod = "MOCK_CARD"
	PaymentMethodMockWallet PaymentMethod = "MOCK_WALLET"
)

func (m PaymentMethod) Value() (driver.Value, error) {
	return string(m), nil
}

func (m *PaymentMethod) Scan(src interface{}) error {
	str, ok := src.(string)
	if !ok {
		return fmt.Errorf("PaymentMethod.Scan: expected string, got %T", src)
	}
	*m = PaymentMethod(str)
	return nil
}

type Payment struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey"`
	OrderID          uuid.UUID       `gorm:"type:uuid;uniqueIndex"`
	UserID           uuid.UUID       `gorm:"type:uuid;index"`
	Amount           decimal.Decimal `gorm:"type:numeric(10,2)"`
	Currency         string          `gorm:"type:char(3);default:USD"`
	Status           PaymentStatus   `gorm:"type:payment_status"`
	Method           PaymentMethod   `gorm:"type:payment_method"`
	IdempotencyKey   string          `gorm:"type:varchar(255);uniqueIndex;not null"`
	GatewayReference string          `gorm:"type:varchar(255)"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type PaymentHistory struct {
	ID        int64          `gorm:"primaryKey;autoIncrement"`
	PaymentID uuid.UUID      `gorm:"type:uuid;not null;index"`
	OldStatus *PaymentStatus `gorm:"type:payment_status"` // nullable on initial PENDING insert
	NewStatus PaymentStatus  `gorm:"type:payment_status;not null"`
	Reason    string         `gorm:"type:text"`
	CreatedAt time.Time
}

func (PaymentHistory) TableName() string { return "payment_history" }
