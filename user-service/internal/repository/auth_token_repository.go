package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
)

var ErrTokenNotFound = errors.New("token not found")

type AuthTokenRepository interface {
	// Create inserts an auth token. Pass the active transaction (tx) when called
	// inside a GORM transaction; pass nil to use the default connection.
	Create(ctx context.Context, tx *gorm.DB, token *model.AuthToken) error
	FindByHash(ctx context.Context, hash string) (*model.AuthToken, error)
	RevokeByUserID(ctx context.Context, userID uuid.UUID) error
}

type authTokenRepository struct {
	db *gorm.DB
}

func NewAuthTokenRepository(db *gorm.DB) AuthTokenRepository {
	return &authTokenRepository{db: db}
}

func (r *authTokenRepository) Create(ctx context.Context, tx *gorm.DB, token *model.AuthToken) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Create(token).Error
}

func (r *authTokenRepository) FindByHash(ctx context.Context, hash string) (*model.AuthToken, error) {
	var token model.AuthToken
	err := r.db.WithContext(ctx).
		Where("refresh_token_hash = ? AND revoked = false AND expires_at > ?", hash, time.Now()).
		First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenNotFound
	}
	return &token, err
}

func (r *authTokenRepository) RevokeByUserID(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.AuthToken{}).
		Where("user_id = ? AND revoked = false", userID).
		Update("revoked", true).Error
}
