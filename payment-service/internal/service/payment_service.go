package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/gateway"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/repository"
)

var (
	ErrForbidden = errors.New("payment: access denied")
	ErrNotFound  = errors.New("payment: not found")
)

type ProcessPaymentInput struct {
	OrderID        uuid.UUID
	UserID         uuid.UUID
	Amount         decimal.Decimal
	Currency       string
	IdempotencyKey string
}

type PaymentService interface {
	ProcessPayment(ctx context.Context, in ProcessPaymentInput) (*model.Payment, error)
	GetByID(ctx context.Context, paymentID, userID uuid.UUID, isAdmin bool) (*dto.PaymentResponse, error)
	GetByOrderID(ctx context.Context, orderID, userID uuid.UUID, isAdmin bool) (*dto.PaymentResponse, error)
	ListByUser(ctx context.Context, userID uuid.UUID, page, size int) ([]dto.PaymentResponse, int64, error)
}

type paymentService struct {
	repo repository.PaymentRepository
	gw   gateway.Gateway
}

func NewPaymentService(repo repository.PaymentRepository, gw gateway.Gateway) PaymentService {
	return &paymentService{repo: repo, gw: gw}
}

func (s *paymentService) ProcessPayment(ctx context.Context, in ProcessPaymentInput) (*model.Payment, error) {
	p := &model.Payment{
		ID:             uuid.New(),
		OrderID:        in.OrderID,
		UserID:         in.UserID,
		Amount:         in.Amount,
		Currency:       in.Currency,
		Status:         model.PaymentStatusPending,
		Method:         model.PaymentMethodMockCard,
		IdempotencyKey: in.IdempotencyKey,
	}
	h := &model.PaymentHistory{
		NewStatus: model.PaymentStatusPending,
		Reason:    "created from orders.created event",
	}

	if err := s.repo.Create(ctx, p, h); err != nil {
		if errors.Is(err, repository.ErrDuplicateIdempotencyKey) {
			return s.repo.FindByIdempotencyKey(ctx, in.IdempotencyKey)
		}
		return nil, err
	}

	gwCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	txnID, err := s.gw.Charge(gwCtx, p.Amount, p.Currency, p.ID.String())
	if err != nil {
		if errors.Is(err, gateway.ErrGatewayDeclined) {
			if updateErr := s.repo.UpdateStatus(ctx, p.ID, model.PaymentStatusFailed, "", "gateway declined"); updateErr != nil {
				return nil, updateErr
			}
		} else {
			// transient error (e.g. context deadline) — leave payment PENDING for retry/DLQ in Week 11
			return nil, err
		}
	} else {
		if updateErr := s.repo.UpdateStatus(ctx, p.ID, model.PaymentStatusCompleted, txnID, "gateway approved"); updateErr != nil {
			return nil, updateErr
		}
	}

	return s.repo.FindByID(ctx, p.ID)
}

func (s *paymentService) GetByID(ctx context.Context, paymentID, userID uuid.UUID, isAdmin bool) (*dto.PaymentResponse, error) {
	p, err := s.repo.FindByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if p.UserID != userID && !isAdmin {
		return nil, ErrForbidden
	}
	resp := dto.ToPaymentResponse(p)
	return &resp, nil
}

func (s *paymentService) GetByOrderID(ctx context.Context, orderID, userID uuid.UUID, isAdmin bool) (*dto.PaymentResponse, error) {
	p, err := s.repo.FindByOrderID(ctx, orderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if p.UserID != userID && !isAdmin {
		return nil, ErrForbidden
	}
	resp := dto.ToPaymentResponse(p)
	return &resp, nil
}

func (s *paymentService) ListByUser(ctx context.Context, userID uuid.UUID, page, size int) ([]dto.PaymentResponse, int64, error) {
	if page < 1 {
		page = 1
	}
	payments, total, err := s.repo.ListByUserID(ctx, userID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.PaymentResponse, len(payments))
	for i, p := range payments {
		out[i] = dto.ToPaymentResponse(&p)
	}
	return out, total, nil
}
