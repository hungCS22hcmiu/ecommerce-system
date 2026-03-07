package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/service"
)

// ─── Mock repository ─────────────────────────────────────────────────────────

type mockUserRepo struct {
	mock.Mock
}

func (m *mockUserRepo) Create(ctx context.Context, user *model.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *mockUserRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func validRegisterRequest() dto.RegisterRequest {
	return dto.RegisterRequest{
		Email:     "john@example.com",
		Password:  "secret123",
		FirstName: "John",
		LastName:  "Doe",
	}
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestRegister_Success(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthServiceWithRepo(repo)

	repo.On("FindByEmail", mock.Anything, "john@example.com").
		Return(nil, repository.ErrNotFound)
	repo.On("Create", mock.Anything, mock.MatchedBy(func(u *model.User) bool {
		return u.Email == "john@example.com" && u.Role == "customer" && u.PasswordHash != ""
	})).Return(nil)

	user, err := svc.Register(context.Background(), validRegisterRequest())

	require.NoError(t, err)
	assert.Equal(t, "john@example.com", user.Email)
	assert.Equal(t, "customer", user.Role)
	assert.NotEmpty(t, user.PasswordHash)
	assert.NotEqual(t, "secret123", user.PasswordHash) // must be hashed
	require.NotNil(t, user.Profile)
	assert.Equal(t, "John", user.Profile.FirstName)
	assert.Equal(t, "Doe", user.Profile.LastName)
	repo.AssertExpectations(t)
}

func TestRegister_DuplicateEmail_ReturnsErrDuplicateEmail(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthServiceWithRepo(repo)

	existing := &model.User{Email: "john@example.com"}
	repo.On("FindByEmail", mock.Anything, "john@example.com").
		Return(existing, nil)

	_, err := svc.Register(context.Background(), validRegisterRequest())

	assert.ErrorIs(t, err, service.ErrDuplicateEmail)
	repo.AssertNotCalled(t, "Create")
}

func TestRegister_RepoFindError_ReturnsError(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthServiceWithRepo(repo)

	dbErr := errors.New("connection refused")
	repo.On("FindByEmail", mock.Anything, "john@example.com").
		Return(nil, dbErr)

	_, err := svc.Register(context.Background(), validRegisterRequest())

	assert.ErrorIs(t, err, dbErr)
	repo.AssertNotCalled(t, "Create")
}

func TestRegister_RepoCreateError_ReturnsError(t *testing.T) {
	repo := new(mockUserRepo)
	svc := service.NewAuthServiceWithRepo(repo)

	dbErr := errors.New("insert failed")
	repo.On("FindByEmail", mock.Anything, "john@example.com").
		Return(nil, repository.ErrNotFound)
	repo.On("Create", mock.Anything, mock.Anything).Return(dbErr)

	_, err := svc.Register(context.Background(), validRegisterRequest())

	assert.Error(t, err)
}
