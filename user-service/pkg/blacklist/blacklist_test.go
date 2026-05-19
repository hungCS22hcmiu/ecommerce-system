package blacklist_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/blacklist"
)

// newTestBlacklist spins up an in-process miniredis and returns a blacklist
// backed by a real go-redis client connected to it. mr is returned so tests
// can call FastForward() to simulate TTL expiry.
func newTestBlacklist(t *testing.T) (blacklist.Blacklist, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return blacklist.New(rdb), mr
}

func TestBlacklist_AddAndContains(t *testing.T) {
	bl, _ := newTestBlacklist(t)
	ctx := context.Background()

	jti := "jti-abc-123"

	contains, err := bl.Contains(ctx, jti)
	require.NoError(t, err)
	assert.False(t, contains, "fresh jti must not be revoked")

	require.NoError(t, bl.Add(ctx, jti, 5*time.Minute))

	contains, err = bl.Contains(ctx, jti)
	require.NoError(t, err)
	assert.True(t, contains, "jti must be revoked after Add")
}

func TestBlacklist_ReRevoke_Idempotent(t *testing.T) {
	bl, _ := newTestBlacklist(t)
	ctx := context.Background()

	jti := "jti-double"
	require.NoError(t, bl.Add(ctx, jti, 5*time.Minute))
	// Re-add the same jti — must not error and must remain revoked.
	require.NoError(t, bl.Add(ctx, jti, 5*time.Minute))

	contains, err := bl.Contains(ctx, jti)
	require.NoError(t, err)
	assert.True(t, contains, "re-revoke must keep the jti revoked")
}

func TestBlacklist_TTL_Expires(t *testing.T) {
	bl, mr := newTestBlacklist(t)
	ctx := context.Background()

	jti := "jti-short-lived"
	require.NoError(t, bl.Add(ctx, jti, 2*time.Second))

	contains, _ := bl.Contains(ctx, jti)
	assert.True(t, contains, "jti must be present immediately after Add")

	// Simulate TTL passage in miniredis (no real-clock wait).
	mr.FastForward(3 * time.Second)

	contains, err := bl.Contains(ctx, jti)
	require.NoError(t, err)
	assert.False(t, contains, "jti must be auto-expired after TTL")
}

func TestBlacklist_DifferentJTIs_Isolated(t *testing.T) {
	bl, _ := newTestBlacklist(t)
	ctx := context.Background()

	require.NoError(t, bl.Add(ctx, "alpha", 5*time.Minute))

	containsAlpha, _ := bl.Contains(ctx, "alpha")
	containsBeta, _ := bl.Contains(ctx, "beta")
	assert.True(t, containsAlpha)
	assert.False(t, containsBeta, "an unrelated jti must not be affected")
}
