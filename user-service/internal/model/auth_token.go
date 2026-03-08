package model

import (
	"time"

	"github.com/google/uuid"
)

// AuthToken stores hashed refresh tokens in the database.
// The raw token is never persisted — only its SHA-256 hex digest.
type AuthToken struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID           uuid.UUID `gorm:"type:uuid;not null;index"`
	RefreshTokenHash string    `gorm:"type:varchar(255);not null;uniqueIndex"`
	ExpiresAt        time.Time `gorm:"not null"`
	Revoked          bool      `gorm:"not null;default:false"`
	CreatedAt        time.Time
}
