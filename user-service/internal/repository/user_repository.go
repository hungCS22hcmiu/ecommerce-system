package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
)

var ErrNotFound = errors.New("user not found")

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	// FindByEmailForUpdate acquires a row-level lock (SELECT ... FOR UPDATE).
	// Must be called inside a GORM transaction.
	FindByEmailForUpdate(ctx context.Context, tx *gorm.DB, email string) (*model.User, error)
	// UpdateLoginAttempts updates the failed_login_attempts counter and is_locked flag.
	// Must be called inside a GORM transaction.
	UpdateLoginAttempts(ctx context.Context, tx *gorm.DB, userID uuid.UUID, attempts int, isLocked bool) error
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &user, err
}

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &user, err
}

// FindByEmailForUpdate uses SELECT ... FOR UPDATE to prevent concurrent login races.
// The caller must pass an active *gorm.DB transaction as tx.
func (r *userRepository) FindByEmailForUpdate(ctx context.Context, tx *gorm.DB, email string) (*model.User, error) {
	var user model.User
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("email = ?", email).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &user, err
}

// UpdateLoginAttempts persists the new failed_login_attempts count and is_locked state.
func (r *userRepository) UpdateLoginAttempts(ctx context.Context, tx *gorm.DB, userID uuid.UUID, attempts int, isLocked bool) error {
	return tx.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"failed_login_attempts": attempts,
			"is_locked":             isLocked,
		}).Error
}
