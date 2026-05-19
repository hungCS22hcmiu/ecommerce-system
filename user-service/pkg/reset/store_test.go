package reset_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/reset"
)

func newTestStore(t *testing.T) (reset.Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return reset.New(rdb), mr
}

func TestReset_TokenRoundtrip(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	email := "user@example.com"

	tok, err := s.GetCode(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, "", tok)

	require.NoError(t, s.SetCode(ctx, email, "tok-abc", 15*time.Minute))
	tok, _ = s.GetCode(ctx, email)
	assert.Equal(t, "tok-abc", tok)
}

func TestReset_Token_TTL_Expires(t *testing.T) {
	s, mr := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.SetCode(ctx, "e@x.com", "tok-short", 30*time.Second))
	mr.FastForward(31 * time.Second)

	tok, err := s.GetCode(ctx, "e@x.com")
	require.NoError(t, err)
	assert.Equal(t, "", tok, "token expires after TTL")
}

func TestReset_SingleUseEnforced_ByDelete(t *testing.T) {
	// "Single-use" is enforced by callers calling DeleteCode after a successful
	// reset; this test asserts the Delete actually rejects future GetCode hits.
	s, _ := newTestStore(t)
	ctx := context.Background()
	email := "single@x.com"

	require.NoError(t, s.SetCode(ctx, email, "tok-1", 15*time.Minute))
	tok, _ := s.GetCode(ctx, email)
	assert.Equal(t, "tok-1", tok)

	require.NoError(t, s.DeleteCode(ctx, email))
	tok, _ = s.GetCode(ctx, email)
	assert.Equal(t, "", tok, "after DeleteCode the token must be unusable")
}

func TestReset_Cooldown(t *testing.T) {
	s, mr := newTestStore(t)
	ctx := context.Background()
	email := "rate-limited@x.com"

	has, _ := s.HasCooldown(ctx, email)
	assert.False(t, has)

	require.NoError(t, s.SetCooldown(ctx, email, 60*time.Second))
	has, _ = s.HasCooldown(ctx, email)
	assert.True(t, has)

	mr.FastForward(61 * time.Second)
	has, _ = s.HasCooldown(ctx, email)
	assert.False(t, has, "cooldown lifts after TTL")
}

func TestReset_AttemptsCounter_TTL30Min(t *testing.T) {
	s, mr := newTestStore(t)
	ctx := context.Background()
	email := "brute@x.com"

	n, _ := s.IncrementAttempts(ctx, email)
	assert.EqualValues(t, 1, n)
	n, _ = s.IncrementAttempts(ctx, email)
	assert.EqualValues(t, 2, n)

	// reset.attemptsTTL is 30m (vs verification's 15m). Push past it.
	mr.FastForward(31 * time.Minute)
	n, _ = s.IncrementAttempts(ctx, email)
	assert.EqualValues(t, 1, n, "counter restarts at 1 after 30m TTL")
}

func TestReset_DeleteAttempts(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	email := "explicit-clear@x.com"

	_, _ = s.IncrementAttempts(ctx, email)
	_, _ = s.IncrementAttempts(ctx, email)
	require.NoError(t, s.DeleteAttempts(ctx, email))

	n, _ := s.IncrementAttempts(ctx, email)
	assert.EqualValues(t, 1, n)
}
