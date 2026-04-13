package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var ErrConcurrentUpdate = errors.New("concurrent cart update, please retry")

// CartItemValue is the JSON value stored in Redis hash field
type CartItemValue struct {
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type RedisCartRepository interface {
	GetCart(ctx context.Context, userID uuid.UUID) (map[int64]CartItemValue, error)
	AddOrUpdateItem(ctx context.Context, userID uuid.UUID, productID int64, val CartItemValue) error
	RemoveItem(ctx context.Context, userID uuid.UUID, productID int64) error
	ClearCart(ctx context.Context, userID uuid.UUID) error
}

type redisCartRepository struct {
	rdb *redis.Client
}

func NewRedisCartRepository(rdb *redis.Client) RedisCartRepository {
	return &redisCartRepository{rdb: rdb}
}

func (r *redisCartRepository) GetCart(ctx context.Context, userID uuid.UUID) (map[int64]CartItemValue, error) {
	key := fmt.Sprintf("cart:%s", userID)
	res, err := r.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	cart := make(map[int64]CartItemValue)
	for k, v := range res {
		productID, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			continue
		}

		var val CartItemValue
		if err := json.Unmarshal([]byte(v), &val); err != nil {
			continue
		}
		cart[productID] = val
	}

	return cart, nil
}

func (r *redisCartRepository) AddOrUpdateItem(ctx context.Context, userID uuid.UUID, productID int64, val CartItemValue) error {
	key := fmt.Sprintf("cart:%s", userID)
	field := strconv.FormatInt(productID, 10)
	jsonVal, err := json.Marshal(val)
	if err != nil {
		return err
	}

	for retries := 0; retries < 3; retries++ {
		err := r.rdb.Watch(ctx, func(tx *redis.Tx) error {
			_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, key, field, jsonVal)
				pipe.Expire(ctx, key, 7*24*time.Hour)
				return nil
			})
			return err
		}, key)

		if err == nil {
			return nil
		}

		if !errors.Is(err, redis.TxFailedErr) {
			return err
		}
	}

	return ErrConcurrentUpdate
}

func (r *redisCartRepository) RemoveItem(ctx context.Context, userID uuid.UUID, productID int64) error {
	key := fmt.Sprintf("cart:%s", userID)
	field := strconv.FormatInt(productID, 10)
	return r.rdb.HDel(ctx, key, field).Err()
}

func (r *redisCartRepository) ClearCart(ctx context.Context, userID uuid.UUID) error {
	key := fmt.Sprintf("cart:%s", userID)
	return r.rdb.Del(ctx, key).Err()
}
