package model

import (
	"time"

	"github.com/google/uuid"
)

type UserProfile struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	FirstName string    `gorm:"type:varchar(100);not null"`
	LastName  string    `gorm:"type:varchar(100);not null"`
	Phone     string    `gorm:"type:varchar(20)"`
	AvatarURL string    `gorm:"type:varchar(500)"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
