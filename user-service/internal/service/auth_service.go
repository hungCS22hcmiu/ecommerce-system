package service

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/repository"
	jwtpkg "github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/jwt"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/password"
)

const (
	maxLoginAttempts = 5
	refreshTokenTTL  = 7 * 24 * time.Hour
)

var (
	ErrDuplicateEmail     = errors.New("email already registered")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account locked")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

type AuthService interface {
	Register(ctx context.Context, req dto.RegisterRequest) (*model.User, error)
	Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error)
	Refresh(ctx context.Context, refreshToken string) (*dto.LoginResponse, error)
}

type authService struct {
	userRepo      repository.UserRepository
	authTokenRepo repository.AuthTokenRepository
	db            *gorm.DB
	privateKey    *rsa.PrivateKey
	publicKey     *rsa.PublicKey
}

// NewAuthService wires all production dependencies.
func NewAuthService(
	userRepo repository.UserRepository,
	authTokenRepo repository.AuthTokenRepository,
	db *gorm.DB,
	privateKey *rsa.PrivateKey,
	publicKey *rsa.PublicKey,
) AuthService {
	return &authService{
		userRepo:      userRepo,
		authTokenRepo: authTokenRepo,
		db:            db,
		privateKey:    privateKey,
		publicKey:     publicKey,
	}
}

// NewAuthServiceWithRepo is kept for existing Register-only tests.
var NewAuthServiceWithRepo = func(userRepo repository.UserRepository) AuthService {
	return &authService{userRepo: userRepo}
}

func (s *authService) Register(ctx context.Context, req dto.RegisterRequest) (*model.User, error) {
	_, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err == nil {
		return nil, ErrDuplicateEmail
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

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

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Login authenticates a user and returns access + refresh tokens.
// Uses SELECT ... FOR UPDATE inside a transaction to prevent login-attempt races.
func (s *authService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
	var resp *dto.LoginResponse

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := s.userRepo.FindByEmailForUpdate(ctx, tx, req.Email)
		if errors.Is(err, repository.ErrNotFound) {
			return ErrInvalidCredentials // don't reveal whether email exists
		}
		if err != nil {
			return err
		}

		if user.IsLocked {
			return ErrAccountLocked
		}

		if !password.Compare(user.PasswordHash, req.Password) {
			newAttempts := user.FailedLoginAttempts + 1
			locked := newAttempts >= maxLoginAttempts
			if updateErr := s.userRepo.UpdateLoginAttempts(ctx, tx, user.ID, newAttempts, locked); updateErr != nil {
				return updateErr
			}
			if locked {
				return ErrAccountLocked
			}
			return ErrInvalidCredentials
		}

		// Successful login — reset counter
		if err := s.userRepo.UpdateLoginAttempts(ctx, tx, user.ID, 0, false); err != nil {
			return err
		}

		accessToken, err := jwtpkg.GenerateAccessToken(user.ID.String(), user.Email, user.Role, s.privateKey)
		if err != nil {
			return fmt.Errorf("generate access token: %w", err)
		}

		rawRefresh, err := jwtpkg.GenerateRefreshToken()
		if err != nil {
			return fmt.Errorf("generate refresh token: %w", err)
		}

		authToken := &model.AuthToken{
			UserID:           user.ID,
			RefreshTokenHash: hashToken(rawRefresh),
			ExpiresAt:        time.Now().Add(refreshTokenTTL),
		}
		if err := s.authTokenRepo.Create(ctx, authToken); err != nil {
			return fmt.Errorf("save refresh token: %w", err)
		}

		firstName, lastName := "", ""
		if user.Profile != nil {
			firstName = user.Profile.FirstName
			lastName = user.Profile.LastName
		}

		resp = &dto.LoginResponse{
			AccessToken:  accessToken,
			RefreshToken: rawRefresh,
			User: dto.UserResponse{
				ID:        user.ID.String(),
				Email:     user.Email,
				Role:      user.Role,
				FirstName: firstName,
				LastName:  lastName,
			},
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Refresh validates a refresh token and issues a new access token.
func (s *authService) Refresh(ctx context.Context, refreshToken string) (*dto.LoginResponse, error) {
	authToken, err := s.authTokenRepo.FindByHash(ctx, hashToken(refreshToken))
	if errors.Is(err, repository.ErrTokenNotFound) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.FindByID(ctx, authToken.UserID)
	if err != nil {
		return nil, ErrInvalidToken
	}

	accessToken, err := jwtpkg.GenerateAccessToken(user.ID.String(), user.Email, user.Role, s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	return &dto.LoginResponse{
		AccessToken: accessToken,
		User: dto.UserResponse{
			ID:    user.ID.String(),
			Email: user.Email,
			Role:  user.Role,
		},
	}, nil
}

// hashToken returns the SHA-256 hex digest of a token string.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}
