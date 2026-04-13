package cache

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/internal/repository"
)

// StartSyncWorker runs a background goroutine that persists Redis carts to
// Postgres every 30 seconds. Errors are logged and skipped — the goroutine
// never stops due to a single failure. Stops when ctx is cancelled.
func StartSyncWorker(
	ctx context.Context,
	rdb *redis.Client,
	redisRepo repository.RedisCartRepository,
	cartRepo repository.CartRepository,
) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	slog.Info("cart sync worker started")

	for {
		select {
		case <-ticker.C:
			syncAll(ctx, rdb, redisRepo, cartRepo)
		case <-ctx.Done():
			slog.Info("cart sync worker stopped")
			return
		}
	}
}

// SyncOnce runs a single sync pass — exported for use in integration tests.
func SyncOnce(
	ctx context.Context,
	rdb *redis.Client,
	redisRepo repository.RedisCartRepository,
	cartRepo repository.CartRepository,
) {
	syncAll(ctx, rdb, redisRepo, cartRepo)
}

func syncAll(
	ctx context.Context,
	rdb *redis.Client,
	redisRepo repository.RedisCartRepository,
	cartRepo repository.CartRepository,
) {
	var cursor uint64
	synced, failed := 0, 0

	for {
		keys, next, err := rdb.Scan(ctx, cursor, "cart:*", 100).Result()
		if err != nil {
			slog.Error("cart sync: SCAN failed", "error", err)
			return
		}

		for _, key := range keys {
			if err := syncOne(ctx, key, redisRepo, cartRepo); err != nil {
				slog.Error("cart sync: failed to sync cart", "key", key, "error", err)
				failed++
			} else {
				synced++
			}
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}

	if synced+failed > 0 {
		slog.Info("cart sync complete", "synced", synced, "failed", failed)
	}
}

func syncOne(
	ctx context.Context,
	key string,
	redisRepo repository.RedisCartRepository,
	cartRepo repository.CartRepository,
) error {
	// Parse userID from key format "cart:{uuid}"
	suffix := strings.TrimPrefix(key, "cart:")
	userID, err := uuid.Parse(suffix)
	if err != nil {
		slog.Warn("cart sync: skipping key with unparseable userID", "key", key)
		return nil
	}

	// Fetch items from Redis
	cartData, err := redisRepo.GetCart(ctx, userID)
	if err != nil {
		return err
	}

	// Skip empty carts — nothing to persist, avoids ghost Postgres rows
	if len(cartData) == 0 {
		return nil
	}

	// Ensure Postgres cart row exists
	cart, err := cartRepo.UpsertCart(ctx, userID)
	if err != nil {
		return err
	}

	// Convert Redis map → []model.CartItem
	now := time.Now()
	items := make([]model.CartItem, 0, len(cartData))
	for productID, val := range cartData {
		items = append(items, model.CartItem{
			CartID:      cart.ID,
			ProductID:   productID,
			ProductName: val.ProductName,
			Quantity:    val.Quantity,
			UnitPrice:   val.UnitPrice,
			AddedAt:     now,
		})
	}

	// Full replace — delete all existing items then bulk-insert
	return cartRepo.ReplaceItems(ctx, cart.ID, items)
}
