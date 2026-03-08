package jwt_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	jwtpkg "github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/jwt"
)

func generateTestKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return priv, &priv.PublicKey
}

func TestGenerateAndValidateAccessToken(t *testing.T) {
	priv, pub := generateTestKeyPair(t)

	token, err := jwtpkg.GenerateAccessToken("user-id-1", "a@b.com", "customer", priv)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := jwtpkg.ValidateToken(token, pub)
	require.NoError(t, err)
	assert.Equal(t, "user-id-1", claims.UserID)
	assert.Equal(t, "a@b.com", claims.Email)
	assert.Equal(t, "customer", claims.Role)
	assert.NotEmpty(t, claims.ID) // jti must be set
	assert.Equal(t, "user-service", claims.Issuer)
}

func TestValidateToken_ExpiredToken_ReturnsError(t *testing.T) {
	priv, pub := generateTestKeyPair(t)

	// Manually craft an already-expired token
	claims := jwtpkg.Claims{
		UserID: "u1",
		Email:  "a@b.com",
		Role:   "customer",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenStr, err := tok.SignedString(priv)
	require.NoError(t, err)

	_, err = jwtpkg.ValidateToken(tokenStr, pub)
	assert.Error(t, err)
}

func TestValidateToken_WrongKey_ReturnsError(t *testing.T) {
	priv, _ := generateTestKeyPair(t)
	_, wrongPub := generateTestKeyPair(t) // different key pair

	token, err := jwtpkg.GenerateAccessToken("u1", "a@b.com", "customer", priv)
	require.NoError(t, err)

	_, err = jwtpkg.ValidateToken(token, wrongPub)
	assert.Error(t, err)
}

func TestGenerateRefreshToken_UniqueAndCorrectLength(t *testing.T) {
	t1, err := jwtpkg.GenerateRefreshToken()
	require.NoError(t, err)
	t2, err := jwtpkg.GenerateRefreshToken()
	require.NoError(t, err)

	assert.Len(t, t1, 128) // 64 bytes → 128 hex chars
	assert.NotEqual(t, t1, t2)
}
