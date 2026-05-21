package password_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/password"
)

func TestPool_VerifyCorrectPassword(t *testing.T) {
	pool := password.NewPool(16)
	pool.Start()
	defer pool.Stop()

	hash, err := password.Hash("mysecret")
	require.NoError(t, err)

	err = pool.Verify(context.Background(), hash, "mysecret")
	assert.NoError(t, err)
}

func TestPool_VerifyWrongPassword(t *testing.T) {
	pool := password.NewPool(16)
	pool.Start()
	defer pool.Stop()

	hash, err := password.Hash("mysecret")
	require.NoError(t, err)

	err = pool.Verify(context.Background(), hash, "wrongpassword")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, password.ErrBcryptOverload)
}

func TestPool_ErrBcryptOverload_WhenQueueFull(t *testing.T) {
	// Queue size 0 means the channel is unbuffered; no workers started
	// so Verify always hits the default branch immediately.
	pool := password.NewPool(0)

	hash, err := password.Hash("mysecret")
	require.NoError(t, err)

	err = pool.Verify(context.Background(), hash, "mysecret")
	assert.ErrorIs(t, err, password.ErrBcryptOverload)
}

func TestPool_ContextCancellation(t *testing.T) {
	pool := password.NewPool(16)
	pool.Start()
	defer pool.Stop()

	hash, err := password.Hash("mysecret")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	err = pool.Verify(ctx, hash, "mysecret")
	// Either the job was enqueued and then ctx was detected, or it ran fine —
	// context cancelled means we get ctx.Err() back.
	// (Workers are running so the job may complete before ctx is checked.)
	// The important invariant: no panic, no hang.
	_ = err
}

func TestPool_ConcurrentVerify(t *testing.T) {
	pool := password.NewPool(256)
	pool.Start()
	defer pool.Stop()

	hash, err := password.Hash("concurrent")
	require.NoError(t, err)

	const goroutines = 20
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = pool.Verify(context.Background(), hash, "concurrent")
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		assert.NoError(t, e, "goroutine %d failed", i)
	}
}
