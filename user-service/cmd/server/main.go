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
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/hungCS22hcmiu/ecommrece-system/user-service/config"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/handler"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/middleware"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/user-service/internal/service"
	jwtpkg "github.com/hungCS22hcmiu/ecommrece-system/user-service/pkg/jwt"
)

func main() {
	// ── Structured logging (JSON in prod, text in dev) ────────────────────────
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Config ────────────────────────────────────────────────────────────────
	cfg := config.Load()

	// ── RSA Keys (fail fast if missing) ───────────────────────────────────────
	privateKey, err := jwtpkg.LoadPrivateKey(cfg.JWTPrivateKeyPath)
	if err != nil {
		slog.Error("failed to load JWT private key", "path", cfg.JWTPrivateKeyPath, "error", err)
		os.Exit(1)
	}
	publicKey, err := jwtpkg.LoadPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		slog.Error("failed to load JWT public key", "path", cfg.JWTPublicKeyPath, "error", err)
		os.Exit(1)
	}
	slog.Info("JWT keys loaded", "private", cfg.JWTPrivateKeyPath, "public", cfg.JWTPublicKeyPath)

	// ── PostgreSQL ────────────────────────────────────────────────────────────
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		slog.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	// Connection pool settings (proposal §5.3)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	slog.Info("postgres connected", "dsn", cfg.DBHost+":"+cfg.DBPort+"/"+cfg.DBName)

	// ── Redis ─────────────────────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr(),
		Password: cfg.RedisPassword,
		DB:       0,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	slog.Info("redis connected", "addr", cfg.RedisAddr())

	// ── AutoMigrate ───────────────────────────────────────────────────────────
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserProfile{},
		&model.UserAddress{},
		&model.AuthToken{},
	); err != nil {
		slog.Error("automigrate failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database schema migrated")

	// ── Router ────────────────────────────────────────────────────────────────
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New() // gin.New() instead of gin.Default() — we add our own middleware
	router.Use(middleware.Recovery())
	router.Use(middleware.Logger())

	// ── Register handlers ─────────────────────────────────────────────────────
	healthHandler := handler.NewHealthHandler(db, rdb)

	// Health probes — unauthenticated, not rate-limited
	router.GET("/health/live", healthHandler.Live)
	router.GET("/health/ready", healthHandler.Ready)

	// API v1 group
	userRepo := repository.NewUserRepository(db)
	authTokenRepo := repository.NewAuthTokenRepository(db)
	authSvc := service.NewAuthService(userRepo, authTokenRepo, db, privateKey, publicKey)
	authHandler := handler.NewAuthHandler(authSvc)

	v1 := router.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
	}

	// ── HTTP Server ───────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine so it doesn't block the shutdown logic below
	go func() {
		slog.Info("user-service starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// ── Graceful shutdown (proposal §12.3) ────────────────────────────────────
	// Block until we receive SIGINT or SIGTERM (e.g., docker stop, Ctrl+C)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down user-service...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop accepting new requests; wait for in-flight requests to finish
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}

	// Close DB and Redis
	sqlDB.Close()
	rdb.Close()

	slog.Info("user-service stopped cleanly")
}
