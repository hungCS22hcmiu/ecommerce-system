package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/model"
)

var (
	ErrDuplicateIdempotencyKey = errors.New("payment: duplicate idempotency key")
	ErrNotFound                = errors.New("payment: not found")
)

type PaymentRepository interface {
	Create(ctx context.Context, p *model.Payment, h *model.PaymentHistory) error
	FindByIdempotencyKey(ctx context.Context, key string) (*model.Payment, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.Payment, error)
	FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Payment, error)
	ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Payment, int64, error)
	UpdateStatus(ctx context.Context, paymentID uuid.UUID, newStatus model.PaymentStatus, gatewayRef, reason string) error
}

type paymentRepository struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) PaymentRepository {
	return &paymentRepository{db: db}
}

// Create inserts a payment and its initial history row in a single transaction.
// Returns ErrDuplicateIdempotencyKey if the idempotency_key (or order_id) already exists.
func (r *paymentRepository) Create(ctx context.Context, p *model.Payment, h *model.PaymentHistory) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			if isDuplicateKey(err) {
				return ErrDuplicateIdempotencyKey
			}
			return err
		}
		h.PaymentID = p.ID
		return tx.Create(h).Error
	})
}

func (r *paymentRepository) FindByIdempotencyKey(ctx context.Context, key string) (*model.Payment, error) {
	var p model.Payment
	err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (r *paymentRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Payment, error) {
	var p model.Payment
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (r *paymentRepository) FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Payment, error) {
	var p model.Payment
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (r *paymentRepository) ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Payment, int64, error) {
	var payments []model.Payment
	var total int64

	base := r.db.WithContext(ctx).Model(&model.Payment{}).Where("user_id = ?", userID)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := base.Order("created_at DESC").Limit(limit).Offset(offset).Find(&payments).Error; err != nil {
		return nil, 0, err
	}
	return payments, total, nil
}

// UpdateStatus updates the payment's status and gateway reference, and appends a history row.
func (r *paymentRepository) UpdateStatus(ctx context.Context, paymentID uuid.UUID, newStatus model.PaymentStatus, gatewayRef, reason string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.Payment
		if err := tx.First(&current, "id = ?", paymentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		if err := tx.Model(&model.Payment{}).Where("id = ?", paymentID).Updates(map[string]any{
			"status":            newStatus,
			"gateway_reference": gatewayRef,
			"updated_at":        gorm.Expr("NOW()"),
		}).Error; err != nil {
			return err
		}

		oldStatus := current.Status
		history := model.PaymentHistory{
			PaymentID: paymentID,
			OldStatus: &oldStatus,
			NewStatus: newStatus,
			Reason:    reason,
		}
		return tx.Create(&history).Error
	})
}

// isDuplicateKey returns true when err is a PostgreSQL unique-violation (SQLSTATE 23505).
func isDuplicateKey(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
