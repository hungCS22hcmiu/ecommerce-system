package loginattempt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const windowTTL = 15 * time.Minute

// Counter tracks failed login attempts per email using Redis INCR.
// Key pattern: login_attempts:{email}
// TTL is set to a 15-minute sliding window on the first increment.
type Counter interface {
	// Increment atomically increments the counter for the given email.
	// Sets a 15-minute TTL on the first increment.
	// Returns the new count.
	Increment(ctx context.Context, email string) (int64, error)
	// Get returns the current failed attempt count (0 if key absent).
	Get(ctx context.Context, email string) (int64, error)
	// Delete removes the counter (call on successful login).
	Delete(ctx context.Context, email string) error
}

type redisCounter struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) Counter {
	return &redisCounter{rdb: rdb}
}

func key(email string) string {
	return fmt.Sprintf("login_attempts:%s", email)
}

func (c *redisCounter) Increment(ctx context.Context, email string) (int64, error) {
	k := key(email)
	count, err := c.rdb.Incr(ctx, k).Result()
	if err != nil {
		return 0, fmt.Errorf("loginattempt.Increment: %w", err)
	}
	// Set TTL only on first increment to avoid resetting the window
	if count == 1 {
		c.rdb.Expire(ctx, k, windowTTL) //nolint:errcheck — best-effort TTL
	}
	return count, nil
}

func (c *redisCounter) Get(ctx context.Context, email string) (int64, error) {
	val, err := c.rdb.Get(ctx, key(email)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil // no attempts recorded
	}
	if err != nil {
		return 0, fmt.Errorf("loginattempt.Get: %w", err)
	}
	return val, nil
}

func (c *redisCounter) Delete(ctx context.Context, email string) error {
	return c.rdb.Del(ctx, key(email)).Err()
}
