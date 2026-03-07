package service

import (
	"context"
	"errors"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/password"
)

var (
	ErrDuplicateEmail     = errors.New("email already registered")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account locked")
)

type AuthService interface {
	Register(ctx context.Context, req dto.RegisterRequest) (*model.User, error)
}

type authService struct {
	userRepo repository.UserRepository
}

// NewAuthService is the production constructor.
// GORM's Create handles User + Profile atomically via association.
func NewAuthService(userRepo repository.UserRepository) AuthService {
	return &authService{userRepo: userRepo}
}

// NewAuthServiceWithRepo is an alias used in tests to make the dependency explicit.
var NewAuthServiceWithRepo = NewAuthService

func (s *authService) Register(ctx context.Context, req dto.RegisterRequest) (*model.User, error) {
	// Check for duplicate email
	_, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err == nil {
		return nil, ErrDuplicateEmail
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	// Hash password
	hash, err := password.Hash(req.Password)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Email:        req.Email,
		PasswordHash: hash,
		Role:         "customer",
		Profile: &model.UserProfile{
			FirstName: req.FirstName,
			LastName:  req.LastName,
		},
	}

	// userRepo.Create calls db.Create(user) which GORM wraps in a transaction,
	// creating User + Profile atomically via the Profile association.
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}
