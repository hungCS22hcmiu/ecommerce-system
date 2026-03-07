package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email               string         `gorm:"type:varchar(255);uniqueIndex;not null"`
	PasswordHash        string         `gorm:"type:varchar(255);not null"`
	Role                string         `gorm:"type:varchar(50);not null;default:'customer'"`
	IsLocked            bool           `gorm:"not null;default:false"`
	FailedLoginAttempts int            `gorm:"not null;default:0"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           gorm.DeletedAt `gorm:"index"`
	Profile             *UserProfile   `gorm:"foreignKey:UserID"`
	Addresses           []UserAddress  `gorm:"foreignKey:UserID"`
}
