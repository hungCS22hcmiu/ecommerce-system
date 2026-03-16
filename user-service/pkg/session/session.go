package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/dto"
)

// Cache stores and retrieves a user's profile in Redis.
// Key pattern: session:{userID}
type Cache interface {
	Set(ctx context.Context, userID uuid.UUID, user dto.UserResponse, ttl time.Duration) error
	Get(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error)
	Delete(ctx context.Context, userID uuid.UUID) error
}

type redisCache struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) Cache {
	return &redisCache{rdb: rdb}
}

func key(userID uuid.UUID) string {
	return fmt.Sprintf("session:%s", userID)
}

func (c *redisCache) Set(ctx context.Context, userID uuid.UUID, user dto.UserResponse, ttl time.Duration) error {
	b, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("session.Set marshal: %w", err)
	}
	return c.rdb.Set(ctx, key(userID), b, ttl).Err()
}

func (c *redisCache) Get(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error) {
	val, err := c.rdb.Get(ctx, key(userID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil // cache miss — not an error
	}
	if err != nil {
		return nil, fmt.Errorf("session.Get: %w", err)
	}
	var user dto.UserResponse
	if err := json.Unmarshal(val, &user); err != nil {
		return nil, fmt.Errorf("session.Get unmarshal: %w", err)
	}
	return &user, nil
}

func (c *redisCache) Delete(ctx context.Context, userID uuid.UUID) error {
	return c.rdb.Del(ctx, key(userID)).Err()
}
