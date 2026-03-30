package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/blacklist"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/email"
	jwtpkg "github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/jwt"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/loginattempt"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/password"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/session"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/verification"
)

const (
	maxLoginAttempts   = 5
	refreshTokenTTL    = 7 * 24 * time.Hour
	sessionTTL         = 30 * time.Minute
	verificationTTL    = 15 * time.Minute
	resendCooldown     = 60 * time.Second
	maxVerifyAttempts  = 5
)

var (
	ErrDuplicateEmail        = errors.New("email already registered")
	ErrUserNotFound          = errors.New("user not found")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrAccountLocked         = errors.New("account locked")
	ErrInvalidToken          = errors.New("invalid or expired token")
	ErrEmailNotVerified      = errors.New("email not verified")
	ErrInvalidCode           = errors.New("invalid or expired verification code")
	ErrAlreadyVerified       = errors.New("email already verified")
	ErrResendCooldown        = errors.New("resend on cooldown")
	ErrTooManyVerifyAttempts = errors.New("too many verification attempts")
)

type AuthService interface {
	Register(ctx context.Context, req dto.RegisterRequest) (*model.User, error)
	Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error)
	Refresh(ctx context.Context, refreshToken string) (*dto.LoginResponse, error)
	Logout(ctx context.Context, accessToken string) error
	VerifyEmail(ctx context.Context, req dto.VerifyEmailRequest) error
	ResendVerification(ctx context.Context, req dto.ResendVerificationRequest) error
}

type authService struct {
	userRepo          repository.UserRepository
	authTokenRepo     repository.AuthTokenRepository
	db                *gorm.DB
	bl                blacklist.Blacklist
	sessionCache      session.Cache
	attemptCounter    loginattempt.Counter
	verificationStore verification.Store
	emailSender       email.Sender
	privateKey        *rsa.PrivateKey
	publicKey         *rsa.PublicKey
}

// NewAuthService wires all production dependencies.
func NewAuthService(
	userRepo repository.UserRepository,
	authTokenRepo repository.AuthTokenRepository,
	db *gorm.DB,
	bl blacklist.Blacklist,
	sessionCache session.Cache,
	attemptCounter loginattempt.Counter,
	verificationStore verification.Store,
	emailSender email.Sender,
	privateKey *rsa.PrivateKey,
	publicKey *rsa.PublicKey,
) AuthService {
	return &authService{
		userRepo:          userRepo,
		authTokenRepo:     authTokenRepo,
		db:                db,
		bl:                bl,
		sessionCache:      sessionCache,
		attemptCounter:    attemptCounter,
		verificationStore: verificationStore,
		emailSender:       emailSender,
		privateKey:        privateKey,
		publicKey:         publicKey,
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

	// Generate and send verification code
	if s.verificationStore != nil && s.emailSender != nil {
		code := generateVerificationCode()
		if err := s.verificationStore.SetCode(ctx, user.Email, code, verificationTTL); err != nil {
			slog.Error("failed to store verification code", "email", user.Email, "error", err)
		} else {
			if err := s.emailSender.SendVerificationCode(ctx, user.Email, code); err != nil {
				slog.Error("failed to send verification email", "email", user.Email, "error", err)
			}
		}
	}

	return user, nil
}

// Login authenticates a user and returns access + refresh tokens.
// Flow:
//  1. Redis pre-check: if attempt counter >= max → ErrAccountLocked (no DB hit)
//  2. DB transaction: SELECT FOR UPDATE → verify password → update counters → generate tokens
//     Auth-layer errors (wrong password, locked) are stored in loginErr; the TX
//     always commits so that UpdateLoginAttempts writes are not rolled back.
//  3. Post-TX: update Redis counter; on success cache session profile
func (s *authService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
	// 1. Redis pre-check (fast path before touching the DB)
	if s.attemptCounter != nil {
		count, err := s.attemptCounter.Get(ctx, req.Email)
		if err == nil && count >= maxLoginAttempts {
			return nil, ErrAccountLocked
		}
	}

	var (
		resp        *dto.LoginResponse
		loginErr    error // auth error; TX commits even when set
		badPassword bool  // signals post-TX Redis INCR
	)

	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := s.userRepo.FindByEmailForUpdate(ctx, tx, req.Email)
		if errors.Is(err, repository.ErrNotFound) {
			loginErr = ErrInvalidCredentials
			return nil // no row to update — commit the no-op TX
		}
		if err != nil {
			return err // real DB error → rollback
		}

		if user.IsLocked {
			loginErr = ErrAccountLocked
			return nil
		}

		if !password.Compare(user.PasswordHash, req.Password) {
			newAttempts := user.FailedLoginAttempts + 1
			locked := newAttempts >= maxLoginAttempts
			if updateErr := s.userRepo.UpdateLoginAttempts(ctx, tx, user.ID, newAttempts, locked); updateErr != nil {
				return updateErr // real DB error → rollback
			}
			badPassword = true
			if locked {
				loginErr = ErrAccountLocked
			} else {
				loginErr = ErrInvalidCredentials
			}
			return nil // ← commit so the counter update persists
		}

		// Successful password — reset DB counter
		if err := s.userRepo.UpdateLoginAttempts(ctx, tx, user.ID, 0, false); err != nil {
			return err
		}

		// Check email verification before issuing tokens
		if !user.IsVerified {
			loginErr = ErrEmailNotVerified
			return nil // commit TX — counter reset persists
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
		if err := s.authTokenRepo.Create(ctx, tx, authToken); err != nil {
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

	// 3. Post-TX Redis updates (outside transaction)
	if badPassword && s.attemptCounter != nil {
		s.attemptCounter.Increment(ctx, req.Email) //nolint:errcheck — best-effort
	}

	if txErr != nil {
		return nil, txErr
	}
	if loginErr != nil {
		return nil, loginErr
	}

	// Success: clear Redis counter and cache session profile
	if s.attemptCounter != nil {
		s.attemptCounter.Delete(ctx, req.Email) //nolint:errcheck — best-effort
	}
	if s.sessionCache != nil {
		userID, _ := uuid.Parse(resp.User.ID)
		s.sessionCache.Set(ctx, userID, resp.User, sessionTTL) //nolint:errcheck — best-effort
	}

	return resp, nil
}

// Refresh validates a refresh token and issues a new access token.
// Tries the session cache before falling back to a DB lookup.
func (s *authService) Refresh(ctx context.Context, refreshToken string) (*dto.LoginResponse, error) {
	authToken, err := s.authTokenRepo.FindByHash(ctx, hashToken(refreshToken))
	if errors.Is(err, repository.ErrTokenNotFound) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}

	// Try session cache first
	var userResp *dto.UserResponse
	if s.sessionCache != nil {
		userResp, _ = s.sessionCache.Get(ctx, authToken.UserID)
	}

	if userResp == nil {
		// Cache miss — fetch from DB and warm the cache
		user, err := s.userRepo.FindByID(ctx, authToken.UserID)
		if err != nil {
			return nil, ErrInvalidToken
		}
		userResp = &dto.UserResponse{
			ID:    user.ID.String(),
			Email: user.Email,
			Role:  user.Role,
		}
		if s.sessionCache != nil {
			s.sessionCache.Set(ctx, authToken.UserID, *userResp, sessionTTL) //nolint:errcheck
		}
	}

	accessToken, err := jwtpkg.GenerateAccessToken(userResp.ID, userResp.Email, userResp.Role, s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	return &dto.LoginResponse{
		AccessToken: accessToken,
		User:        *userResp,
	}, nil
}

// Logout parses the access token, blacklists its jti in Redis,
// revokes all refresh tokens in DB, and deletes the session cache entry.
func (s *authService) Logout(ctx context.Context, accessToken string) error {
	claims, err := jwtpkg.ValidateToken(accessToken, s.publicKey)
	if err != nil {
		return ErrInvalidToken
	}

	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl > 0 {
		if err := s.bl.Add(ctx, claims.ID, ttl); err != nil {
			return err
		}
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return fmt.Errorf("invalid user id in token: %w", err)
	}

	if s.sessionCache != nil {
		s.sessionCache.Delete(ctx, userID) //nolint:errcheck — best-effort
	}

	return s.authTokenRepo.RevokeByUserID(ctx, userID)
}

// VerifyEmail validates a 6-digit code and marks the user as verified.
func (s *authService) VerifyEmail(ctx context.Context, req dto.VerifyEmailRequest) error {
	// Brute-force protection
	count, err := s.verificationStore.IncrementAttempts(ctx, req.Email)
	if err != nil {
		return err
	}
	if count > int64(maxVerifyAttempts) {
		return ErrTooManyVerifyAttempts
	}

	storedCode, err := s.verificationStore.GetCode(ctx, req.Email)
	if err != nil {
		return err
	}
	if storedCode == "" || storedCode != req.Code {
		return ErrInvalidCode
	}

	user, err := s.userRepo.FindByEmail(ctx, req.Email)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrInvalidCode // don't leak whether email exists
	}
	if err != nil {
		return err
	}
	if user.IsVerified {
		return ErrAlreadyVerified
	}

	if err := s.userRepo.UpdateVerificationStatus(ctx, user.ID, true); err != nil {
		return err
	}

	// Clean up Redis
	s.verificationStore.DeleteCode(ctx, req.Email)     //nolint:errcheck
	s.verificationStore.DeleteAttempts(ctx, req.Email)  //nolint:errcheck

	return nil
}

// ResendVerification generates a new verification code and sends it via email.
func (s *authService) ResendVerification(ctx context.Context, req dto.ResendVerificationRequest) error {
	hasCooldown, err := s.verificationStore.HasCooldown(ctx, req.Email)
	if err != nil {
		return err
	}
	if hasCooldown {
		return ErrResendCooldown
	}

	user, err := s.userRepo.FindByEmail(ctx, req.Email)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if user.IsVerified {
		return ErrAlreadyVerified
	}

	code := generateVerificationCode()
	if err := s.verificationStore.SetCode(ctx, req.Email, code, verificationTTL); err != nil {
		return err
	}
	if err := s.verificationStore.SetCooldown(ctx, req.Email, resendCooldown); err != nil {
		slog.Error("failed to set resend cooldown", "email", req.Email, "error", err)
	}

	if err := s.emailSender.SendVerificationCode(ctx, req.Email, code); err != nil {
		slog.Error("failed to send verification email", "email", req.Email, "error", err)
	}

	return nil
}

// generateVerificationCode returns a cryptographically random 6-digit numeric string.
func generateVerificationCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		// Fallback — should never happen with crypto/rand
		return "000000"
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// hashToken returns the SHA-256 hex digest of a token string.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}
