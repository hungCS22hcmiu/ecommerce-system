package verification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store manages email verification codes, resend cooldowns, and brute-force attempt counters in Redis.
type Store interface {
	SetCode(ctx context.Context, email string, code string, ttl time.Duration) error
	GetCode(ctx context.Context, email string) (string, error)
	DeleteCode(ctx context.Context, email string) error
	SetCooldown(ctx context.Context, email string, ttl time.Duration) error
	HasCooldown(ctx context.Context, email string) (bool, error)
	IncrementAttempts(ctx context.Context, email string) (int64, error)
	DeleteAttempts(ctx context.Context, email string) error
}

type redisStore struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) Store {
	return &redisStore{rdb: rdb}
}

func codeKey(email string) string     { return fmt.Sprintf("verification:%s", email) }
func cooldownKey(email string) string  { return fmt.Sprintf("verification_cooldown:%s", email) }
func attemptsKey(email string) string  { return fmt.Sprintf("verification_attempts:%s", email) }

const attemptsTTL = 15 * time.Minute

func (s *redisStore) SetCode(ctx context.Context, email string, code string, ttl time.Duration) error {
	return s.rdb.Set(ctx, codeKey(email), code, ttl).Err()
}

func (s *redisStore) GetCode(ctx context.Context, email string) (string, error) {
	val, err := s.rdb.Get(ctx, codeKey(email)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("verification.GetCode: %w", err)
	}
	return val, nil
}

func (s *redisStore) DeleteCode(ctx context.Context, email string) error {
	return s.rdb.Del(ctx, codeKey(email)).Err()
}

func (s *redisStore) SetCooldown(ctx context.Context, email string, ttl time.Duration) error {
	return s.rdb.Set(ctx, cooldownKey(email), "1", ttl).Err()
}

func (s *redisStore) HasCooldown(ctx context.Context, email string) (bool, error) {
	_, err := s.rdb.Get(ctx, cooldownKey(email)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("verification.HasCooldown: %w", err)
	}
	return true, nil
}

func (s *redisStore) IncrementAttempts(ctx context.Context, email string) (int64, error) {
	k := attemptsKey(email)
	count, err := s.rdb.Incr(ctx, k).Result()
	if err != nil {
		return 0, fmt.Errorf("verification.IncrementAttempts: %w", err)
	}
	if count == 1 {
		s.rdb.Expire(ctx, k, attemptsTTL) //nolint:errcheck — best-effort TTL
	}
	return count, nil
}

func (s *redisStore) DeleteAttempts(ctx context.Context, email string) error {
	return s.rdb.Del(ctx, attemptsKey(email)).Err()
}
