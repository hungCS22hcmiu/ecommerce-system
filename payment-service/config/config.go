package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// Kafka — payment service is the heaviest Kafka user (producer + consumer)
	KafkaBrokers       string
	KafkaConsumerGroup string
	KafkaWorkerCount   int // number of goroutines in the worker pool (Day 32)

	// Mock gateway config — simulates a real payment provider
	GatewaySuccessRate  float64 // 0.0 to 1.0 (default: 0.9)
	GatewayMinLatencyMs int
	GatewayMaxLatencyMs int

	// JWT public key — for validating tokens on admin endpoints
	JWTPublicKeyPath string

	Env string
}

func Load() *Config {
	return &Config{
		Port:                getEnv("PORT", "8003"),
		DBHost:              getEnv("DB_HOST", "localhost"),
		DBPort:              getEnv("DB_PORT", "5432"),
		DBUser:              getEnv("DB_USER", "postgres"),
		DBPassword:          getEnv("DB_PASSWORD", "postgres"),
		DBName:              getEnv("DB_NAME", "ecommerce_payments"),
		KafkaBrokers:        getEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaConsumerGroup:  getEnv("KAFKA_CONSUMER_GROUP", "payment-service"),
		KafkaWorkerCount:    5, // 5 goroutines in worker pool
		GatewaySuccessRate:  0.9,
		GatewayMinLatencyMs: 50,
		GatewayMaxLatencyMs: 200,
		JWTPublicKeyPath:    getEnv("JWT_PUBLIC_KEY_PATH", "./keys/public.pem"),
		Env:                 getEnv("ENV", "development"),
	}
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName,
	)
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
