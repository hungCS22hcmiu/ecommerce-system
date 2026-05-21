package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/service"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/blacklist"
	jwtpkg "github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/jwt"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/loginattempt"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/password"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/session"
)

// ─── Mock user repository ─────────────────────────────────────────────────────

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

func (m *mockUserRepo) FindByEmailForUpdate(ctx context.Context, tx *gorm.DB, email string) (*model.User, error) {
	args := m.Called(ctx, tx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *mockUserRepo) FindByEmailWithProfile(ctx context.Context, email string) (*model.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *mockUserRepo) UpdateLoginAttempts(ctx context.Context, tx *gorm.DB, userID uuid.UUID, attempts int, isLocked bool) error {
	args := m.Called(ctx, tx, userID, attempts, isLocked)
	return args.Error(0)
}

func (m *mockUserRepo) FindByIDWithProfile(ctx context.Context, id uuid.UUID) (*model.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *mockUserRepo) UpdateProfile(ctx context.Context, userID uuid.UUID, firstName, lastName, phone string) error {
	args := m.Called(ctx, userID, firstName, lastName, phone)
	return args.Error(0)
}

func (m *mockUserRepo) UpdateVerificationStatus(ctx context.Context, userID uuid.UUID, verified bool) error {
	args := m.Called(ctx, userID, verified)
	return args.Error(0)
}

func (m *mockUserRepo) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	args := m.Called(ctx, userID, passwordHash)
	return args.Error(0)
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

// ─── Register tests ───────────────────────────────────────────────────────────

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

// ─── Mock auth-token repository ───────────────────────────────────────────────

type mockAuthTokenRepo struct {
	mock.Mock
}

func (m *mockAuthTokenRepo) Create(ctx context.Context, tx *gorm.DB, token *model.AuthToken) error {
	args := m.Called(ctx, tx, token)
	return args.Error(0)
}

func (m *mockAuthTokenRepo) FindByHash(ctx context.Context, hash string) (*model.AuthToken, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.AuthToken), args.Error(1)
}

func (m *mockAuthTokenRepo) RevokeByUserID(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// generateTestRSAKey generates a 2048-bit RSA key pair for use in tests.
func generateTestRSAKey(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key, &key.PublicKey
}

// newMockDB creates a *gorm.DB backed by go-sqlmock so tests can assert
// transaction lifecycle (BEGIN / COMMIT / ROLLBACK) without a real database.
func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock
}

func validLoginRequest() dto.LoginRequest {
	return dto.LoginRequest{Email: "john@example.com", Password: "secret123"}
}

// bcryptHash is a thin wrapper so tests can produce real bcrypt hashes without
// importing the internal service package (which would cause a cycle).
func bcryptHash(plain string) (string, error) {
	return password.Hash(plain)
}

// ─── Login tests ──────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	db, dbMock := newMockDB(t)
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, db, nil, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	userID := uuid.New()
	user := &model.User{
		Email:      "john@example.com",
		Role:       "customer",
		IsVerified: true,
		Profile:    &model.UserProfile{FirstName: "John", LastName: "Doe"},
	}
	user.ID = userID

	hash, err := bcryptHash("secret123")
	require.NoError(t, err)
	user.PasswordHash = hash

	// Only the token-insert TX; bcrypt runs outside any transaction now
	dbMock.ExpectBegin()
	dbMock.ExpectCommit()

	userRepo.On("FindByEmailWithProfile", mock.Anything, "john@example.com").
		Return(user, nil)
	tokenRepo.On("Create", mock.Anything, mock.Anything, mock.AnythingOfType("*model.AuthToken")).
		Return(nil)

	resp, err := svc.Login(context.Background(), validLoginRequest())

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
	assert.Equal(t, "john@example.com", resp.User.Email)
	assert.Equal(t, "customer", resp.User.Role)
	assert.Equal(t, "John", resp.User.FirstName)
	assert.Equal(t, "Doe", resp.User.LastName)
	require.NoError(t, dbMock.ExpectationsWereMet())
	userRepo.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}

func TestLogin_UserNotFound_ReturnsErrInvalidCredentials(t *testing.T) {
	db, _ := newMockDB(t)
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, db, nil, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	// No DB transaction — failure happens before any TX
	userRepo.On("FindByEmailWithProfile", mock.Anything, "john@example.com").
		Return(nil, repository.ErrNotFound)

	_, err := svc.Login(context.Background(), validLoginRequest())

	assert.ErrorIs(t, err, service.ErrInvalidCredentials)
	userRepo.AssertExpectations(t)
}

func TestLogin_WrongPassword_ReturnsErrInvalidCredentials(t *testing.T) {
	db, _ := newMockDB(t)
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, db, nil, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	userID := uuid.New()
	// hash for "correct-password", request sends "secret123"
	hash, err := bcryptHash("correct-password")
	require.NoError(t, err)
	user := &model.User{
		Email:        "john@example.com",
		PasswordHash: hash,
	}
	user.ID = userID

	// No DB transaction — failure happens before any TX
	userRepo.On("FindByEmailWithProfile", mock.Anything, "john@example.com").
		Return(user, nil)

	_, err = svc.Login(context.Background(), validLoginRequest())

	assert.ErrorIs(t, err, service.ErrInvalidCredentials)
	userRepo.AssertExpectations(t)
}

func TestLogin_CreateAuthTokenError_ReturnsError(t *testing.T) {
	db, dbMock := newMockDB(t)
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, db, nil, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	userID := uuid.New()
	hash, err := bcryptHash("secret123")
	require.NoError(t, err)
	user := &model.User{Email: "john@example.com", PasswordHash: hash, IsVerified: true, Role: "customer"}
	user.ID = userID

	dbMock.ExpectBegin()
	dbMock.ExpectRollback()

	dbErr := errors.New("token insert failed")
	userRepo.On("FindByEmailWithProfile", mock.Anything, "john@example.com").
		Return(user, nil)
	tokenRepo.On("Create", mock.Anything, mock.Anything, mock.AnythingOfType("*model.AuthToken")).
		Return(dbErr)

	_, err = svc.Login(context.Background(), validLoginRequest())

	assert.Error(t, err)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

// ─── Refresh tests ────────────────────────────────────────────────────────────

func TestRefresh_Success(t *testing.T) {
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, nil, nil, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	userID := uuid.New()
	rawToken := "some-raw-refresh-token"
	authToken := &model.AuthToken{
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	user := &model.User{Email: "john@example.com", Role: "customer"}
	user.ID = userID

	tokenRepo.On("FindByHash", mock.Anything, mock.AnythingOfType("string")).
		Return(authToken, nil)
	userRepo.On("FindByID", mock.Anything, userID).
		Return(user, nil)

	resp, err := svc.Refresh(context.Background(), rawToken)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.AccessToken)
	assert.Equal(t, "john@example.com", resp.User.Email)
	tokenRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestRefresh_TokenNotFound_ReturnsErrInvalidToken(t *testing.T) {
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, nil, nil, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	tokenRepo.On("FindByHash", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, repository.ErrTokenNotFound)

	_, err := svc.Refresh(context.Background(), "bad-token")

	assert.ErrorIs(t, err, service.ErrInvalidToken)
	userRepo.AssertNotCalled(t, "FindByID")
}

func TestRefresh_DBError_ReturnsError(t *testing.T) {
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, nil, nil, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	dbErr := errors.New("connection lost")
	tokenRepo.On("FindByHash", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, dbErr)

	_, err := svc.Refresh(context.Background(), "some-token")

	assert.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
}

func TestRefresh_UserNotFound_ReturnsErrInvalidToken(t *testing.T) {
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, nil, nil, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	userID := uuid.New()
	authToken := &model.AuthToken{
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	tokenRepo.On("FindByHash", mock.Anything, mock.AnythingOfType("string")).
		Return(authToken, nil)
	userRepo.On("FindByID", mock.Anything, userID).
		Return(nil, repository.ErrNotFound)

	_, err := svc.Refresh(context.Background(), "some-token")

	assert.ErrorIs(t, err, service.ErrInvalidToken)
}

// ─── Mock blacklist ───────────────────────────────────────────────────────────

type mockBlacklist struct {
	mock.Mock
}

func (m *mockBlacklist) Add(ctx context.Context, jti string, ttl time.Duration) error {
	args := m.Called(ctx, jti, ttl)
	return args.Error(0)
}

func (m *mockBlacklist) Contains(ctx context.Context, jti string) (bool, error) {
	args := m.Called(ctx, jti)
	return args.Bool(0), args.Error(1)
}

// Ensure mockBlacklist satisfies the interface at compile time.
var _ blacklist.Blacklist = (*mockBlacklist)(nil)

// ─── Logout tests ─────────────────────────────────────────────────────────────

// mintToken is a helper that generates a signed access token for a given userID.
func mintToken(t *testing.T, privKey *rsa.PrivateKey, userID string) string {
	t.Helper()
	tok, err := jwtpkg.GenerateAccessToken(userID, "test@example.com", "customer", privKey)
	require.NoError(t, err)
	return tok
}

func TestLogout_Success_BlacklistsJTIAndRevokesRefreshTokens(t *testing.T) {
	bl := new(mockBlacklist)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)
	userID := uuid.New()
	svc := service.NewAuthService(nil, tokenRepo, nil, bl, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	token := mintToken(t, privKey, userID.String())

	bl.On("Add", mock.Anything, mock.AnythingOfType("string"), mock.MatchedBy(func(ttl time.Duration) bool {
		return ttl > 0 && ttl <= 15*time.Minute
	})).Return(nil)
	tokenRepo.On("RevokeByUserID", mock.Anything, userID).Return(nil)

	err := svc.Logout(context.Background(), token)

	require.NoError(t, err)
	bl.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}

func TestLogout_InvalidToken_ReturnsErrInvalidToken(t *testing.T) {
	_, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(nil, nil, nil, nil, nil, nil, nil, nil, nil, pubKey, nil, "", nil)

	err := svc.Logout(context.Background(), "not.a.valid.jwt")

	assert.ErrorIs(t, err, service.ErrInvalidToken)
}

func TestLogout_BlacklistError_ReturnsError(t *testing.T) {
	bl := new(mockBlacklist)
	privKey, pubKey := generateTestRSAKey(t)
	userID := uuid.New()
	svc := service.NewAuthService(nil, nil, nil, bl, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	token := mintToken(t, privKey, userID.String())
	bl.On("Add", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(errors.New("redis down"))

	err := svc.Logout(context.Background(), token)

	assert.Error(t, err)
	bl.AssertExpectations(t)
}

func TestLogout_RevokeByUserIDError_ReturnsError(t *testing.T) {
	bl := new(mockBlacklist)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)
	userID := uuid.New()
	svc := service.NewAuthService(nil, tokenRepo, nil, bl, nil, nil, nil, nil, privKey, pubKey, nil, "", nil)

	token := mintToken(t, privKey, userID.String())
	bl.On("Add", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
	tokenRepo.On("RevokeByUserID", mock.Anything, userID).Return(errors.New("db error"))

	err := svc.Logout(context.Background(), token)

	assert.Error(t, err)
	bl.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}

// ─── Mock session.Cache ───────────────────────────────────────────────────────

type mockSessionCache struct {
	mock.Mock
}

func (m *mockSessionCache) Set(ctx context.Context, userID uuid.UUID, user dto.UserResponse, ttl time.Duration) error {
	args := m.Called(ctx, userID, user, ttl)
	return args.Error(0)
}

func (m *mockSessionCache) Get(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.UserResponse), args.Error(1)
}

func (m *mockSessionCache) Delete(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

var _ session.Cache = (*mockSessionCache)(nil)

// ─── Mock loginattempt.Counter ────────────────────────────────────────────────

type mockAttemptCounter struct {
	mock.Mock
}

func (m *mockAttemptCounter) Increment(ctx context.Context, email string) (int64, error) {
	args := m.Called(ctx, email)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockAttemptCounter) Get(ctx context.Context, email string) (int64, error) {
	args := m.Called(ctx, email)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockAttemptCounter) Delete(ctx context.Context, email string) error {
	args := m.Called(ctx, email)
	return args.Error(0)
}

var _ loginattempt.Counter = (*mockAttemptCounter)(nil)

// ─── Login: Redis pre-check and post-verify counter tests ────────────────────

func TestLogin_RedisPreCheck_BlocksAtMax(t *testing.T) {
	counter := new(mockAttemptCounter)
	privKey, pubKey := generateTestRSAKey(t)
	// No DB needed — pre-check should abort before any DB call
	svc := service.NewAuthService(nil, nil, nil, nil, nil, counter, nil, nil, privKey, pubKey, nil, "", nil)

	counter.On("Get", mock.Anything, "john@example.com").Return(int64(5), nil)

	_, err := svc.Login(context.Background(), validLoginRequest())

	assert.ErrorIs(t, err, service.ErrAccountLocked)
	counter.AssertExpectations(t)
}

func TestLogin_IncrementsRedisCounterOnBadPassword(t *testing.T) {
	db, _ := newMockDB(t)
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	counter := new(mockAttemptCounter)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, db, nil, nil, counter, nil, nil, privKey, pubKey, nil, "", nil)

	userID := uuid.New()
	hash, err := bcryptHash("correct-password")
	require.NoError(t, err)
	user := &model.User{
		Email:        "john@example.com",
		PasswordHash: hash,
	}
	user.ID = userID

	// No DB transaction — bad password returns before any TX
	counter.On("Get", mock.Anything, "john@example.com").Return(int64(0), nil)
	userRepo.On("FindByEmailWithProfile", mock.Anything, "john@example.com").
		Return(user, nil)
	counter.On("Increment", mock.Anything, "john@example.com").Return(int64(1), nil)

	_, err = svc.Login(context.Background(), validLoginRequest())

	assert.ErrorIs(t, err, service.ErrInvalidCredentials)
	counter.AssertExpectations(t)
}

func TestLogin_DeletesRedisCounterAndSetsSessionOnSuccess(t *testing.T) {
	db, dbMock := newMockDB(t)
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	counter := new(mockAttemptCounter)
	sc := new(mockSessionCache)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, db, nil, sc, counter, nil, nil, privKey, pubKey, nil, "", nil)

	userID := uuid.New()
	hash, err := bcryptHash("secret123")
	require.NoError(t, err)
	user := &model.User{
		Email:      "john@example.com",
		Role:       "customer",
		IsVerified: true,
		Profile:    &model.UserProfile{FirstName: "John", LastName: "Doe"},
	}
	user.ID = userID
	user.PasswordHash = hash

	dbMock.ExpectBegin()
	dbMock.ExpectCommit()

	counter.On("Get", mock.Anything, "john@example.com").Return(int64(0), nil)
	userRepo.On("FindByEmailWithProfile", mock.Anything, "john@example.com").
		Return(user, nil)
	tokenRepo.On("Create", mock.Anything, mock.Anything, mock.AnythingOfType("*model.AuthToken")).
		Return(nil)
	counter.On("Delete", mock.Anything, "john@example.com").Return(nil)
	sc.On("Set", mock.Anything, userID, mock.AnythingOfType("dto.UserResponse"), mock.AnythingOfType("time.Duration")).
		Return(nil)

	resp, err := svc.Login(context.Background(), validLoginRequest())

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, dbMock.ExpectationsWereMet())
	counter.AssertExpectations(t)
	sc.AssertExpectations(t)
}

func TestLogin_BcryptOverload_ReturnsErrBcryptOverload(t *testing.T) {
	db, _ := newMockDB(t)
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	privKey, pubKey := generateTestRSAKey(t)

	// Pool with zero-capacity queue and no workers started → Verify always returns ErrBcryptOverload
	pool := password.NewPool(0)

	svc := service.NewAuthService(userRepo, tokenRepo, db, nil, nil, nil, nil, nil, privKey, pubKey, nil, "", pool)

	userID := uuid.New()
	hash, err := bcryptHash("secret123")
	require.NoError(t, err)
	user := &model.User{Email: "john@example.com", PasswordHash: hash, IsVerified: true}
	user.ID = userID

	userRepo.On("FindByEmailWithProfile", mock.Anything, "john@example.com").
		Return(user, nil)

	_, err = svc.Login(context.Background(), validLoginRequest())

	assert.ErrorIs(t, err, service.ErrBcryptOverload)
}

// ─── Refresh: session cache hit ───────────────────────────────────────────────

func TestRefresh_UsesSessionCacheOnHit(t *testing.T) {
	userRepo := new(mockUserRepo)
	tokenRepo := new(mockAuthTokenRepo)
	sc := new(mockSessionCache)
	privKey, pubKey := generateTestRSAKey(t)
	svc := service.NewAuthService(userRepo, tokenRepo, nil, nil, sc, nil, nil, nil, privKey, pubKey, nil, "", nil)

	userID := uuid.New()
	authToken := &model.AuthToken{
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	cached := &dto.UserResponse{
		ID:    userID.String(),
		Email: "john@example.com",
		Role:  "customer",
	}

	tokenRepo.On("FindByHash", mock.Anything, mock.AnythingOfType("string")).
		Return(authToken, nil)
	sc.On("Get", mock.Anything, userID).Return(cached, nil)

	resp, err := svc.Refresh(context.Background(), "some-raw-token")

	require.NoError(t, err)
	assert.NotEmpty(t, resp.AccessToken)
	assert.Equal(t, "john@example.com", resp.User.Email)
	// FindByID must NOT be called — cache was hit
	userRepo.AssertNotCalled(t, "FindByID")
	sc.AssertExpectations(t)
}

// ─── Logout: session cache deletion ──────────────────────────────────────────

func TestLogout_DeletesSessionCache(t *testing.T) {
	bl := new(mockBlacklist)
	tokenRepo := new(mockAuthTokenRepo)
	sc := new(mockSessionCache)
	privKey, pubKey := generateTestRSAKey(t)
	userID := uuid.New()
	svc := service.NewAuthService(nil, tokenRepo, nil, bl, sc, nil, nil, nil, privKey, pubKey, nil, "", nil)

	token := mintToken(t, privKey, userID.String())

	bl.On("Add", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
	sc.On("Delete", mock.Anything, userID).Return(nil)
	tokenRepo.On("RevokeByUserID", mock.Anything, userID).Return(nil)

	err := svc.Logout(context.Background(), token)

	require.NoError(t, err)
	bl.AssertExpectations(t)
	sc.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}
