package middleware_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/middleware"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/blacklist"
	jwtpkg "github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/jwt"
)

func init() {
	gin.SetMode(gin.TestMode)
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

var _ blacklist.Blacklist = (*mockBlacklist)(nil)

// ─── Helpers ─────────────────────────────────────────────────────────────────

func generateTestRSAKey(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key, &key.PublicKey
}

// newProtectedRouter builds a minimal Gin router with the Auth middleware and a
// simple 200 handler so tests can assert middleware behaviour end-to-end.
func newProtectedRouter(publicKey *rsa.PublicKey, bl blacklist.Blacklist) *gin.Engine {
	r := gin.New()
	r.GET("/protected", middleware.Auth(publicKey, bl), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"userID": c.GetString(middleware.CtxUserID),
			"role":   c.GetString(middleware.CtxRole),
			"jti":    c.GetString(middleware.CtxJTI),
		})
	})
	return r
}

func get(router *gin.Engine, path, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestAuthMiddleware_MissingHeader_Returns401(t *testing.T) {
	privKey, pubKey := generateTestRSAKey(t)
	_ = privKey
	bl := new(mockBlacklist)

	router := newProtectedRouter(pubKey, bl)
	w := get(router, "/protected", "")

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	bl.AssertNotCalled(t, "Contains")
}

func TestAuthMiddleware_MalformedHeader_Returns401(t *testing.T) {
	_, pubKey := generateTestRSAKey(t)
	bl := new(mockBlacklist)

	router := newProtectedRouter(pubKey, bl)
	w := get(router, "/protected", "Token abc123") // wrong prefix

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	bl.AssertNotCalled(t, "Contains")
}

func TestAuthMiddleware_InvalidToken_Returns401(t *testing.T) {
	_, pubKey := generateTestRSAKey(t)
	bl := new(mockBlacklist)

	router := newProtectedRouter(pubKey, bl)
	w := get(router, "/protected", "Bearer not.a.valid.jwt")

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	bl.AssertNotCalled(t, "Contains")
}

func TestAuthMiddleware_BlacklistedToken_Returns401(t *testing.T) {
	privKey, pubKey := generateTestRSAKey(t)
	bl := new(mockBlacklist)

	token, err := jwtpkg.GenerateAccessToken("user-1", "test@example.com", "customer", privKey)
	require.NoError(t, err)

	// Parse token to get jti
	claims, err := jwtpkg.ValidateToken(token, pubKey)
	require.NoError(t, err)

	bl.On("Contains", mock.Anything, claims.ID).Return(true, nil)

	router := newProtectedRouter(pubKey, bl)
	w := get(router, "/protected", "Bearer "+token)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	bl.AssertExpectations(t)
}

func TestAuthMiddleware_ValidToken_InjectsContext(t *testing.T) {
	privKey, pubKey := generateTestRSAKey(t)
	bl := new(mockBlacklist)

	token, err := jwtpkg.GenerateAccessToken("user-42", "alice@example.com", "admin", privKey)
	require.NoError(t, err)

	claims, err := jwtpkg.ValidateToken(token, pubKey)
	require.NoError(t, err)

	bl.On("Contains", mock.Anything, claims.ID).Return(false, nil)

	router := newProtectedRouter(pubKey, bl)
	w := get(router, "/protected", "Bearer "+token)

	assert.Equal(t, http.StatusOK, w.Code)
	bl.AssertExpectations(t)
}

func TestAuthMiddleware_BlacklistError_Returns500(t *testing.T) {
	privKey, pubKey := generateTestRSAKey(t)
	bl := new(mockBlacklist)

	token, err := jwtpkg.GenerateAccessToken("user-1", "test@example.com", "customer", privKey)
	require.NoError(t, err)

	claims, err := jwtpkg.ValidateToken(token, pubKey)
	require.NoError(t, err)

	bl.On("Contains", mock.Anything, claims.ID).Return(false, assert.AnError)

	router := newProtectedRouter(pubKey, bl)
	w := get(router, "/protected", "Bearer "+token)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	bl.AssertExpectations(t)
}
