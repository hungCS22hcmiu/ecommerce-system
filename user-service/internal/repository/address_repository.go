package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
)

var ErrAddressNotFound = errors.New("address not found")

type AddressRepository interface {
	Create(ctx context.Context, addr *model.UserAddress) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.UserAddress, error)
	Update(ctx context.Context, addr *model.UserAddress) error
	Delete(ctx context.Context, id uuid.UUID) error
	// SetDefault atomically clears is_default for all user addresses,
	// then sets is_default=true on the given address — wrapped in a DB transaction.
	SetDefault(ctx context.Context, userID, addressID uuid.UUID) error
}

type addressRepository struct {
	db *gorm.DB
}

func NewAddressRepository(db *gorm.DB) AddressRepository {
	return &addressRepository{db: db}
}

func (r *addressRepository) Create(ctx context.Context, addr *model.UserAddress) error {
	return r.db.WithContext(ctx).Create(addr).Error
}

func (r *addressRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.UserAddress, error) {
	var addr model.UserAddress
	err := r.db.WithContext(ctx).First(&addr, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAddressNotFound
	}
	return &addr, err
}

func (r *addressRepository) Update(ctx context.Context, addr *model.UserAddress) error {
	return r.db.WithContext(ctx).Save(addr).Error
}

func (r *addressRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.UserAddress{}, "id = ?", id).Error
}

func (r *addressRepository) SetDefault(ctx context.Context, userID, addressID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UserAddress{}).
			Where("user_id = ?", userID).
			Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&model.UserAddress{}).
			Where("id = ?", addressID).
			Update("is_default", true).Error
	})
}
