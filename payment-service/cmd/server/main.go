package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := config.Load()

	// ── PostgreSQL ─────────────────────────────────────────────────────────────
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		slog.Error("postgres connection failed", "error", err)
		os.Exit(1)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	slog.Info("payment-service postgres connected", "db", cfg.DBName)

	// NOTE: Payment Service does NOT use Redis directly.
	// Idempotency is handled at the DB layer (unique constraint on idempotency_key).
	// Kafka connection is initialised on Day 32 when the worker pool is implemented.

	// ── Router ─────────────────────────────────────────────────────────────────
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()

	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "timestamp": time.Now().UTC()})
	})
	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		sqlDB2, _ := db.DB()
		if sqlDB2.PingContext(ctx) != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "DOWN", "checks": gin.H{"postgres": "DOWN"},
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status": "UP", "checks": gin.H{"postgres": "UP"},
			"timestamp": time.Now().UTC(),
		})
	})

	// API v1 group — payment endpoints added on Day 31
	// v1 := router.Group("/api/v1")

	// ── HTTP Server + Graceful Shutdown ────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go func() {
		slog.Info("payment-service starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// On Day 32 we will also start the Kafka consumer goroutine here:
	// go kafkaConsumer.Start(ctx, cfg.KafkaWorkerCount)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down payment-service...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	sqlDB.Close()
	slog.Info("payment-service stopped cleanly")
}
