package loginattempt_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/loginattempt"
)

// ─── Interface compliance check ───────────────────────────────────────────────

func TestCounterInterface_Defined(t *testing.T) {
	var _ loginattempt.Counter = (loginattempt.Counter)(nil)
}

// ─── Behaviour contract tests (fake in-memory impl) ──────────────────────────

type fakeCounter struct {
	counts map[string]int64
}

func newFakeCounter() loginattempt.Counter {
	return &fakeCounter{counts: make(map[string]int64)}
}

func (f *fakeCounter) Increment(_ context.Context, email string) (int64, error) {
	f.counts[email]++
	return f.counts[email], nil
}

func (f *fakeCounter) Get(_ context.Context, email string) (int64, error) {
	return f.counts[email], nil
}

func (f *fakeCounter) Delete(_ context.Context, email string) error {
	delete(f.counts, email)
	return nil
}

func TestCounter_IncrementStartsAtOne(t *testing.T) {
	c := newFakeCounter()
	count, err := c.Increment(context.Background(), "user@example.com")
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestCounter_IncrementIsMonotonic(t *testing.T) {
	c := newFakeCounter()
	ctx := context.Background()
	email := "user@example.com"

	for i := int64(1); i <= 5; i++ {
		count, err := c.Increment(ctx, email)
		require.NoError(t, err)
		assert.Equal(t, i, count)
	}
}

func TestCounter_GetReturnZeroWhenAbsent(t *testing.T) {
	c := newFakeCounter()
	count, err := c.Get(context.Background(), "nobody@example.com")
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestCounter_DeleteResetsCount(t *testing.T) {
	c := newFakeCounter()
	ctx := context.Background()
	email := "user@example.com"

	c.Increment(ctx, email)
	c.Increment(ctx, email)

	require.NoError(t, c.Delete(ctx, email))

	count, err := c.Get(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}
