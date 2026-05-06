//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	pgmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/repository"
)

// TestConcurrentIdempotency is the proof required by ADR locking-strategy.md §5 and
// proposal §10.4 Scenario 3: N goroutines racing to insert the same idempotency key
// must result in exactly one payment row and one history row.
func TestConcurrentIdempotency(t *testing.T) {
	ctx := context.Background()

	// Spin up an isolated Postgres container for this test.
	pgCtr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("ecommerce_payments"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgCtr.Terminate(ctx) })

	dsn, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(25)

	// Run migrations so the payments + payment_history tables and enums exist.
	runMigrations(t, sqlDB, dsn)

	repo := repository.NewPaymentRepository(db)

	const N = 10
	idemKey := uuid.NewString()
	orderID := uuid.New()
	userID := uuid.New()

	results := make([]error, N)
	var wg sync.WaitGroup
	gate := make(chan struct{}) // start-gate: all goroutines wait here before racing

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-gate // blocked until close(gate) fires all goroutines simultaneously
			p := &model.Payment{
				ID:             uuid.New(),
				OrderID:        orderID,
				UserID:         userID,
				Amount:         decimal.NewFromFloat(99.99),
				Currency:       "USD",
				Status:         model.PaymentStatusPending,
				Method:         model.PaymentMethodMockCard,
				IdempotencyKey: idemKey,
			}
			h := &model.PaymentHistory{
				NewStatus: model.PaymentStatusPending,
				Reason:    "concurrent test",
			}
			results[idx] = repo.Create(ctx, p, h)
		}(i)
	}

	close(gate) // release all N goroutines at once
	wg.Wait()

	// Exactly one goroutine must succeed.
	successCount := 0
	for _, err := range results {
		if err == nil {
			successCount++
		} else {
			assert.ErrorIs(t, err, repository.ErrDuplicateIdempotencyKey,
				"failed goroutines must return ErrDuplicateIdempotencyKey, got: %v", err)
		}
	}
	assert.Equal(t, 1, successCount, "exactly one goroutine must succeed")

	// DB must have exactly one payment row for this idempotency key.
	var paymentCount int64
	db.Model(&model.Payment{}).Where("idempotency_key = ?", idemKey).Count(&paymentCount)
	assert.Equal(t, int64(1), paymentCount, "exactly 1 payment row in DB")

	// And exactly one history row linked to that payment.
	var historyCount int64
	db.Model(&model.PaymentHistory{}).
		Joins("JOIN payments ON payments.id = payment_history.payment_id").
		Where("payments.idempotency_key = ?", idemKey).
		Count(&historyCount)
	assert.Equal(t, int64(1), historyCount, "exactly 1 payment_history row in DB")
}

// runMigrations applies the payment-service migrations against the container DB.
// The migration files path is relative to where the test binary runs (payment-service root).
func runMigrations(t *testing.T, sqlDB *sql.DB, _ string) {
	t.Helper()
	driver, err := pgmigrate.WithInstance(sqlDB, &pgmigrate.Config{})
	require.NoError(t, err, "migration driver")

	m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "ecommerce_payments", driver)
	require.NoError(t, err, "migrate.New")

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		require.NoError(t, err, "migrate.Up")
	}
}

