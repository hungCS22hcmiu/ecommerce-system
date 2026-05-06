package main

import (
	"context"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // registers pprof handlers on the default mux
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	pgmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/segmentio/kafka-go"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/config"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/gateway"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/handler"
	kafkapkg "github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/kafka"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/middleware"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/service"
	jwtpkg "github.com/hungCS22hcmiu/ecommrece-system/payment-service/pkg/jwt"
)

func main() {
	// Structured JSON logs with a static service field on every line.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger.With("service", "payment-service"))

	cfg := config.Load()

	// ── PostgreSQL ─────────────────────────────────────────────────────────────
	db, err := gorm.Open(gormpg.Open(cfg.DSN()), &gorm.Config{
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
	slog.Info("postgres connected", "db", cfg.DBName)

	// ── Database Migrations ───────────────────────────────────────────────────
	migDriver, err := pgmigrate.WithInstance(sqlDB, &pgmigrate.Config{})
	if err != nil {
		slog.Error("migration driver failed", "error", err)
		os.Exit(1)
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "ecommerce_payments", migDriver)
	if err != nil {
		slog.Error("migration init failed", "error", err)
		os.Exit(1)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations applied")

	// NOTE: Payment Service does NOT use Redis.
	// Idempotency is handled at the DB layer via unique constraint on idempotency_key.

	// ── JWT Public Key ─────────────────────────────────────────────────────────
	publicKey, err := jwtpkg.LoadPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		slog.Error("failed to load JWT public key", "path", cfg.JWTPublicKeyPath, "error", err)
		os.Exit(1)
	}

	// ── Dependencies ──────────────────────────────────────────────────────────
	gw := gateway.NewMockGateway(cfg)
	repo := repository.NewPaymentRepository(db)
	svc := service.NewPaymentService(repo, gw)
	paymentHandler := handler.NewPaymentHandler(svc)

	// ── Router ─────────────────────────────────────────────────────────────────
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(middleware.Correlation())
	router.Use(middleware.Logger())

	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "timestamp": time.Now().UTC()})
	})

	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		checks := gin.H{}
		overall := "UP"
		code := http.StatusOK

		sqlDB2, _ := db.DB()
		if sqlDB2.PingContext(ctx) != nil {
			checks["postgres"] = "DOWN"
			overall = "DOWN"
			code = http.StatusServiceUnavailable
		} else {
			checks["postgres"] = "UP"
		}

		conn, kafkaErr := kafka.DialContext(ctx, "tcp", cfg.KafkaBrokers)
		if kafkaErr != nil {
			checks["kafka"] = "DOWN"
			overall = "DOWN"
			code = http.StatusServiceUnavailable
		} else {
			conn.Close()
			checks["kafka"] = "UP"
		}

		c.JSON(code, gin.H{"status": overall, "checks": checks, "timestamp": time.Now().UTC()})
	})

	// ── API Routes ────────────────────────────────────────────────────────────
	v1 := router.Group("/api/v1")

	// POST /payments is internal — no JWT required (called by Kafka saga or smoke tests)
	v1.POST("/payments", paymentHandler.CreatePayment)

	// Read endpoints require a valid JWT
	authed := v1.Group("/payments")
	authed.Use(middleware.Auth(publicKey))
	authed.GET("", paymentHandler.ListByUser)
	authed.GET("/order/:orderId", paymentHandler.GetByOrderID)
	authed.GET("/:id", paymentHandler.GetByID)

	// ── pprof on :6060 (must NOT be publicly exposed) ─────────────────────────
	go func() {
		slog.Info("pprof listening", "port", 6060)
		if err := http.ListenAndServe(":6060", nil); err != nil {
			slog.Error("pprof server error", "error", err)
		}
	}()

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

	producer := kafkapkg.NewProducer(cfg.KafkaBrokers)
	consumer := kafkapkg.NewConsumer(cfg, svc, producer)
	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	go consumer.Run(consumerCtx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down payment-service...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Stop consumer fetch loop and drain workers within deadline.
	consumerCancel()
	consumerDone := make(chan struct{})
	go func() { consumer.Wait(); close(consumerDone) }()
	select {
	case <-consumerDone:
		slog.Info("consumer drained cleanly")
	case <-shutdownCtx.Done():
		slog.Warn("shutdown deadline exceeded — forcing consumer close")
	}

	// 2. Close Kafka writers (flush any buffered acks).
	producer.Close()

	// 3. Stop accepting HTTP, drain in-flight requests.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	// 4. Close DB pool.
	sqlDB.Close()
	slog.Info("payment-service stopped cleanly")
}
