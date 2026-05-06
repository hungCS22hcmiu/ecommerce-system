package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/gateway"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/service"
)

// ─── Mock repository ────────────────────────────────────────────────────────

type mockRepo struct{ mock.Mock }

var _ repository.PaymentRepository = (*mockRepo)(nil)

func (m *mockRepo) Create(ctx context.Context, p *model.Payment, h *model.PaymentHistory) error {
	return m.Called(ctx, p, h).Error(0)
}
func (m *mockRepo) FindByIdempotencyKey(ctx context.Context, key string) (*model.Payment, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Payment), args.Error(1)
}
func (m *mockRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.Payment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Payment), args.Error(1)
}
func (m *mockRepo) FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Payment, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Payment), args.Error(1)
}
func (m *mockRepo) ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Payment, int64, error) {
	args := m.Called(ctx, userID, limit, offset)
	return args.Get(0).([]model.Payment), args.Get(1).(int64), args.Error(2)
}
func (m *mockRepo) UpdateStatus(ctx context.Context, paymentID uuid.UUID, newStatus model.PaymentStatus, gatewayRef, reason string) error {
	return m.Called(ctx, paymentID, newStatus, gatewayRef, reason).Error(0)
}

// ─── Mock gateway ────────────────────────────────────────────────────────────

type mockGateway struct{ mock.Mock }

var _ gateway.Gateway = (*mockGateway)(nil)

func (m *mockGateway) Charge(ctx context.Context, amount decimal.Decimal, currency, reference string) (string, error) {
	args := m.Called(ctx, amount, currency, reference)
	return args.String(0), args.Error(1)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newInput() service.ProcessPaymentInput {
	return service.ProcessPaymentInput{
		OrderID:        uuid.New(),
		UserID:         uuid.New(),
		Amount:         decimal.NewFromFloat(99.99),
		Currency:       "USD",
		IdempotencyKey: uuid.NewString(),
	}
}

// ─── Tests ───────────────────────────────────────────────────────────────────

// TestProcessPayment_IdempotentReplay verifies that when the DB signals a duplicate
// idempotency key, the service returns the existing payment without calling the gateway.
func TestProcessPayment_IdempotentReplay(t *testing.T) {
	repo := &mockRepo{}
	gw := &mockGateway{}
	svc := service.NewPaymentService(repo, gw)

	in := newInput()
	existing := &model.Payment{
		ID:             uuid.New(),
		IdempotencyKey: in.IdempotencyKey,
		Status:         model.PaymentStatusCompleted,
	}

	repo.On("Create", mock.Anything, mock.AnythingOfType("*model.Payment"), mock.AnythingOfType("*model.PaymentHistory")).
		Return(repository.ErrDuplicateIdempotencyKey)
	repo.On("FindByIdempotencyKey", mock.Anything, in.IdempotencyKey).
		Return(existing, nil)

	result, err := svc.ProcessPayment(context.Background(), in)

	require.NoError(t, err)
	assert.Equal(t, existing.ID, result.ID)
	assert.Equal(t, model.PaymentStatusCompleted, result.Status)
	gw.AssertNotCalled(t, "Charge")
	repo.AssertExpectations(t)
}

// TestProcessPayment_GatewaySuccess verifies the happy path: payment created,
// gateway approves, status moves to COMPLETED.
func TestProcessPayment_GatewaySuccess(t *testing.T) {
	repo := &mockRepo{}
	gw := &mockGateway{}
	svc := service.NewPaymentService(repo, gw)

	in := newInput()
	txnID := "MOCK-abc123"

	repo.On("Create", mock.Anything, mock.AnythingOfType("*model.Payment"), mock.AnythingOfType("*model.PaymentHistory")).
		Return(nil)
	gw.On("Charge", mock.Anything, in.Amount, in.Currency, mock.AnythingOfType("string")).
		Return(txnID, nil)
	repo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("uuid.UUID"), model.PaymentStatusCompleted, txnID, "gateway approved").
		Return(nil)

	completedPayment := &model.Payment{Status: model.PaymentStatusCompleted, GatewayReference: txnID}
	repo.On("FindByID", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return(completedPayment, nil)

	result, err := svc.ProcessPayment(context.Background(), in)

	require.NoError(t, err)
	assert.Equal(t, model.PaymentStatusCompleted, result.Status)
	assert.Equal(t, txnID, result.GatewayReference)
	repo.AssertExpectations(t)
	gw.AssertExpectations(t)
}

// TestProcessPayment_GatewayDecline verifies that a gateway decline moves status
// to FAILED and does not return an error to the caller (the saga handles FAILED).
func TestProcessPayment_GatewayDecline(t *testing.T) {
	repo := &mockRepo{}
	gw := &mockGateway{}
	svc := service.NewPaymentService(repo, gw)

	in := newInput()

	repo.On("Create", mock.Anything, mock.AnythingOfType("*model.Payment"), mock.AnythingOfType("*model.PaymentHistory")).
		Return(nil)
	gw.On("Charge", mock.Anything, in.Amount, in.Currency, mock.AnythingOfType("string")).
		Return("", gateway.ErrGatewayDeclined)
	repo.On("UpdateStatus", mock.Anything, mock.AnythingOfType("uuid.UUID"), model.PaymentStatusFailed, "", "gateway declined").
		Return(nil)

	failedPayment := &model.Payment{Status: model.PaymentStatusFailed}
	repo.On("FindByID", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return(failedPayment, nil)

	result, err := svc.ProcessPayment(context.Background(), in)

	require.NoError(t, err)
	assert.Equal(t, model.PaymentStatusFailed, result.Status)
	repo.AssertExpectations(t)
	gw.AssertExpectations(t)
}

// TestProcessPayment_GatewayTimeout verifies that when the 5 s gateway deadline
// fires, the service propagates the error and leaves the payment in PENDING
// (UpdateStatus must NOT be called — Week 11 retry/DLQ will handle PENDING rows).
func TestProcessPayment_GatewayTimeout(t *testing.T) {
	repo := &mockRepo{}
	gw := &mockGateway{}
	svc := service.NewPaymentService(repo, gw)

	in := newInput()

	repo.On("Create", mock.Anything, mock.AnythingOfType("*model.Payment"), mock.AnythingOfType("*model.PaymentHistory")).
		Return(nil)

	// Simulate a context-deadline error as the gateway would return after 5 s
	gw.On("Charge", mock.MatchedBy(func(ctx context.Context) bool { return true }), in.Amount, in.Currency, mock.AnythingOfType("string")).
		Return("", context.DeadlineExceeded).
		After(10 * time.Millisecond) // fast in tests; real timeout is 5 s

	result, err := svc.ProcessPayment(context.Background(), in)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
	repo.AssertNotCalled(t, "UpdateStatus")
	repo.AssertExpectations(t)
	gw.AssertExpectations(t)
}

// TestGetByID_OwnershipEnforced verifies that a non-admin user cannot fetch
// a payment that belongs to a different user.
func TestGetByID_OwnershipEnforced(t *testing.T) {
	repo := &mockRepo{}
	gw := &mockGateway{}
	svc := service.NewPaymentService(repo, gw)

	ownerID := uuid.New()
	callerID := uuid.New()
	paymentID := uuid.New()

	repo.On("FindByID", mock.Anything, paymentID).
		Return(&model.Payment{ID: paymentID, UserID: ownerID}, nil)

	resp, err := svc.GetByID(context.Background(), paymentID, callerID, false)

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrForbidden)
}

// TestGetByID_AdminBypassesOwnership verifies that an admin can fetch any payment.
func TestGetByID_AdminBypassesOwnership(t *testing.T) {
	repo := &mockRepo{}
	gw := &mockGateway{}
	svc := service.NewPaymentService(repo, gw)

	ownerID := uuid.New()
	adminID := uuid.New()
	paymentID := uuid.New()

	p := &model.Payment{ID: paymentID, UserID: ownerID, Status: model.PaymentStatusCompleted}
	repo.On("FindByID", mock.Anything, paymentID).Return(p, nil)

	resp, err := svc.GetByID(context.Background(), paymentID, adminID, true)

	require.NoError(t, err)
	assert.Equal(t, dto.PaymentResponse{}.Status, "")
	assert.NotNil(t, resp)
}
