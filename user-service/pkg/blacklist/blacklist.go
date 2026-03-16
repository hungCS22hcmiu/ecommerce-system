package blacklist

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Blacklist stores and checks revoked JWT JTIs.
type Blacklist interface {
	Add(ctx context.Context, jti string, ttl time.Duration) error
	Contains(ctx context.Context, jti string) (bool, error)
}

type redisBlacklist struct {
	rdb *redis.Client
}

// New returns a Redis-backed Blacklist.
func New(rdb *redis.Client) Blacklist {
	return &redisBlacklist{rdb: rdb}
}

func (b *redisBlacklist) Add(ctx context.Context, jti string, ttl time.Duration) error {
	return b.rdb.Set(ctx, "blacklist:"+jti, "1", ttl).Err()
}

func (b *redisBlacklist) Contains(ctx context.Context, jti string) (bool, error) {
	err := b.rdb.Get(ctx, "blacklist:"+jti).Err()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
