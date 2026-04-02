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
	"github.com/golang-migrate/migrate/v4"
	pgmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/hungCS22hcmiu/ecommrece-system/cart-service/config"
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

	// ── Database Migrations ───────────────────────────────────────────────────
	migDriver, err := pgmigrate.WithInstance(sqlDB, &pgmigrate.Config{})
	if err != nil {
		slog.Error("migration driver failed", "error", err)
		os.Exit(1)
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "ecommerce_carts", migDriver)
	if err != nil {
		slog.Error("migration init failed", "error", err)
		os.Exit(1)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations applied")

	// ── Redis ──────────────────────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr(),
		Password: cfg.RedisPassword,
	})
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		slog.Error("redis connection failed", "error", err)
		os.Exit(1)
	}

	slog.Info("cart-service dependencies connected",
		"postgres", cfg.DBHost+":"+cfg.DBPort,
		"redis", cfg.RedisAddr(),
		"productService", cfg.ProductServiceURL,
	)

	// ── Router ─────────────────────────────────────────────────────────────────
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()

	// Health probes
	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "timestamp": time.Now().UTC()})
	})
	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		checks := gin.H{}
		overall := "UP"
		httpStatus := http.StatusOK

		sqlDB2, _ := db.DB()
		if sqlDB2.PingContext(ctx) != nil {
			checks["postgres"] = "DOWN"
			overall = "DOWN"
			httpStatus = http.StatusServiceUnavailable
		} else {
			checks["postgres"] = "UP"
		}
		if rdb.Ping(ctx).Err() != nil {
			checks["redis"] = "DOWN"
			overall = "DOWN"
			httpStatus = http.StatusServiceUnavailable
		} else {
			checks["redis"] = "UP"
		}

		c.JSON(httpStatus, gin.H{"status": overall, "checks": checks, "timestamp": time.Now().UTC()})
	})

	// API v1 group — cart endpoints added on Day 19
	// v1 := router.Group("/api/v1")

	// ── HTTP Server + Graceful Shutdown ────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go func() {
		slog.Info("cart-service starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down cart-service...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	sqlDB.Close()
	rdb.Close()
	slog.Info("cart-service stopped cleanly")
}
