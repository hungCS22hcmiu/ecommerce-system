package verification_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/verification"
)

func newTestStore(t *testing.T) (verification.Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return verification.New(rdb), mr
}

func TestVerification_CodeRoundtripAndDelete(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	email := "user@example.com"

	// Missing key — GetCode must return "", nil (not an error).
	code, err := s.GetCode(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, "", code)

	require.NoError(t, s.SetCode(ctx, email, "123456", 5*time.Minute))
	code, err = s.GetCode(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, "123456", code)

	require.NoError(t, s.DeleteCode(ctx, email))
	code, _ = s.GetCode(ctx, email)
	assert.Equal(t, "", code, "code must be gone after Delete")
}

func TestVerification_Code_TTL_Expires(t *testing.T) {
	s, mr := newTestStore(t)
	ctx := context.Background()
	email := "expiry@example.com"

	require.NoError(t, s.SetCode(ctx, email, "999000", 2*time.Second))
	mr.FastForward(3 * time.Second)

	code, err := s.GetCode(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, "", code, "code must be gone after TTL")
}

func TestVerification_Cooldown(t *testing.T) {
	s, mr := newTestStore(t)
	ctx := context.Background()
	email := "cooldown@example.com"

	has, err := s.HasCooldown(ctx, email)
	require.NoError(t, err)
	assert.False(t, has, "no cooldown initially")

	require.NoError(t, s.SetCooldown(ctx, email, 60*time.Second))
	has, _ = s.HasCooldown(ctx, email)
	assert.True(t, has, "cooldown active right after Set")

	mr.FastForward(61 * time.Second)
	has, _ = s.HasCooldown(ctx, email)
	assert.False(t, has, "cooldown lifts after TTL")
}

func TestVerification_AttemptsCounter_IncrementsAndExpires(t *testing.T) {
	s, mr := newTestStore(t)
	ctx := context.Background()
	email := "brute@example.com"

	n, err := s.IncrementAttempts(ctx, email)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	n, _ = s.IncrementAttempts(ctx, email)
	assert.EqualValues(t, 2, n)

	n, _ = s.IncrementAttempts(ctx, email)
	assert.EqualValues(t, 3, n)

	// Counter must auto-expire after the 15-minute window set on the first Incr.
	mr.FastForward(16 * time.Minute)
	n, _ = s.IncrementAttempts(ctx, email)
	assert.EqualValues(t, 1, n, "after TTL expiry the counter restarts at 1")
}

func TestVerification_DeleteAttempts(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	email := "reset-counter@example.com"

	_, _ = s.IncrementAttempts(ctx, email)
	_, _ = s.IncrementAttempts(ctx, email)
	require.NoError(t, s.DeleteAttempts(ctx, email))

	n, _ := s.IncrementAttempts(ctx, email)
	assert.EqualValues(t, 1, n, "counter restarts at 1 after explicit Delete")
}
