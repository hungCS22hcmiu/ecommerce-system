package password_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/password"
)

func TestHash_ReturnsBcryptHash(t *testing.T) {
	hash, err := password.Hash("secret123")

	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	// bcrypt hashes always start with $2a$ (cost 12 prefix)
	assert.Contains(t, hash, "$2a$12$")
}

func TestHash_DifferentCallsProduceDifferentHashes(t *testing.T) {
	hash1, err1 := password.Hash("secret123")
	hash2, err2 := password.Hash("secret123")

	require.NoError(t, err1)
	require.NoError(t, err2)
	// bcrypt uses a random salt each time
	assert.NotEqual(t, hash1, hash2)
}

func TestCompare_CorrectPassword_ReturnsTrue(t *testing.T) {
	hash, err := password.Hash("secret123")
	require.NoError(t, err)

	assert.True(t, password.Compare(hash, "secret123"))
}

func TestCompare_WrongPassword_ReturnsFalse(t *testing.T) {
	hash, err := password.Hash("secret123")
	require.NoError(t, err)

	assert.False(t, password.Compare(hash, "wrongpassword"))
}

func TestCompare_EmptyPassword_ReturnsFalse(t *testing.T) {
	hash, err := password.Hash("secret123")
	require.NoError(t, err)

	assert.False(t, password.Compare(hash, ""))
}

func TestCompare_InvalidHash_ReturnsFalse(t *testing.T) {
	assert.False(t, password.Compare("not-a-valid-hash", "secret123"))
}
