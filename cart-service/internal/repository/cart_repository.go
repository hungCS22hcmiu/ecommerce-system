package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/model"
	"gorm.io/gorm"
)

type CartRepository interface {
	UpsertCart(ctx context.Context, userID uuid.UUID) (*model.Cart, error)
	ReplaceItems(ctx context.Context, cartID uuid.UUID, items []model.CartItem) error
	GetCartWithItems(ctx context.Context, userID uuid.UUID) (*model.Cart, error)
	MarkCheckedOut(ctx context.Context, cartID uuid.UUID) error
	ClearCart(ctx context.Context, userID uuid.UUID) error
}

type cartRepository struct {
	db *gorm.DB
}

func NewCartRepository(db *gorm.DB) CartRepository {
	return &cartRepository{db: db}
}

func (r *cartRepository) UpsertCart(ctx context.Context, userID uuid.UUID) (*model.Cart, error) {
	var cart model.Cart
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, "ACTIVE").
		FirstOrCreate(&cart, model.Cart{
			UserID: userID,
			Status: "ACTIVE",
		}).Error

	if err != nil {
		return nil, err
	}
	return &cart, nil
}

func (r *cartRepository) ReplaceItems(ctx context.Context, cartID uuid.UUID, items []model.CartItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete existing items
		if err := tx.Where("cart_id = ?", cartID).Delete(&model.CartItem{}).Error; err != nil {
			return err
		}

		// If no items to add, just return
		if len(items) == 0 {
			return nil
		}

		// Ensure CartID is set correctly for all items
		for i := range items {
			items[i].CartID = cartID
		}

		// Bulk insert new items
		return tx.Create(&items).Error
	})
}

func (r *cartRepository) GetCartWithItems(ctx context.Context, userID uuid.UUID) (*model.Cart, error) {
	var cart model.Cart
	err := r.db.WithContext(ctx).
		Preload("Items").
		Where("user_id = ? AND status = ?", userID, "ACTIVE").
		First(&cart).Error

	if err != nil {
		return nil, err
	}
	return &cart, nil
}

func (r *cartRepository) MarkCheckedOut(ctx context.Context, cartID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&model.Cart{}).
		Where("id = ?", cartID).
		Update("status", "CHECKED_OUT").Error
}

func (r *cartRepository) ClearCart(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cart model.Cart
		if err := tx.Where("user_id = ? AND status = ?", userID, "ACTIVE").First(&cart).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		if err := tx.Where("cart_id = ?", cart.ID).Delete(&model.CartItem{}).Error; err != nil {
			return err
		}

		return tx.Delete(&cart).Error
	})
}
