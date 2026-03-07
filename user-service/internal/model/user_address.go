package model

import (
	"time"

	"github.com/google/uuid"
)

type UserAddress struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index"`
	Label        string    `gorm:"type:varchar(50)"`  // e.g. "home", "work"
	AddressLine1 string    `gorm:"type:varchar(255);not null"`
	AddressLine2 string    `gorm:"type:varchar(255)"`
	City         string    `gorm:"type:varchar(100);not null"`
	State        string    `gorm:"type:varchar(100)"`
	Country      string    `gorm:"type:varchar(100);not null"`
	PostalCode   string    `gorm:"type:varchar(20)"`
	IsDefault    bool      `gorm:"not null;default:false"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
