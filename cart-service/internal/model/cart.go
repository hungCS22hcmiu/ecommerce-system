package model

import (
	"time"

	"github.com/google/uuid"
)

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
	ProductID   int64     `gorm:"not null"` // BIGINT in schema — NOT uuid
	ProductName string    `gorm:"type:varchar(200)"`
	Quantity    int       `gorm:"not null;check:quantity>0"`
	UnitPrice   float64   `gorm:"type:decimal(10,2);check:unit_price>=0"`
	AddedAt     time.Time `gorm:"not null;default:now()"`
}
