# Development Guide

## Prerequisites

| Tool | Version | Install |
|---|---|---|
| Golang | 1.21+ | `brew install go` |
| Java | 21 LTS | `brew install openjdk@21` |
| Docker | 24+ | `brew install --cask docker` |
| Docker Compose | 2.x | Included with Docker Desktop |

## Quick Start

```bash
git clone https://github.com/hungCS22hcmiu/ecommerce-system.git
cd ecommerce-system

# Configure environment
cp .env.example .env
# Edit .env with actual values (DB password, SMTP credentials, etc.)

# Start core infrastructure
docker compose up -d postgres redis

# Start a specific service
docker compose up --build -d user-service
```

## Infrastructure Setup

Start only what the current task needs (prefer minimal):

```bash
# Core infrastructure (needed by almost everything)
docker compose up -d postgres redis

# Add Kafka only when working on payment/order flows
docker compose up -d zookeeper kafka

# Start a specific service
docker compose up -d user-service

# Build images (first time or after code changes)
docker compose build              # all services
docker compose build user-service # single service
```

Databases are initialized automatically on first Postgres start via `script/init-databases.sql`.

## Running Go Services Locally

Each Go service is a self-contained module. Run from its directory:

```bash
cd user-service   # or cart-service / payment-service

# Run directly
go run ./cmd/server/main.go

# Build binary
go build -o bin/server ./cmd/server/main.go

# Run tests
go test ./...

# Single package test
go test ./internal/handler/...

# Test with race detector (required for concurrency code)
go test -race ./...

# Integration tests (requires real Postgres + Redis)
go test -tags=integration -v -race ./internal/integration/
```

### Hot Reload (Development)

Go services include `Dockerfile.dev` + `.air.toml` for Air hot reload via Docker Compose with volume-mounted source.

### Go Config

Config is loaded from environment variables with fallbacks (see `config/config.go`). No config files — set env vars or use `.env` with Docker Compose.

## Running Java Services Locally

Each Java service uses Maven wrapper:

```bash
cd product-service   # or order-service

# Run
./mvnw spring-boot:run

# Build (skip tests)
./mvnw package -DskipTests

# Run tests
./mvnw test

# Single test class
./mvnw test -Dtest=ProductServiceApplicationTests
```

Java version: 21. Spring Boot: 3.5. Uses Flyway for DB migrations (currently disabled — enable when writing first migration).

## Docker Compose Services

| Container | Image | Port | Health Check |
|---|---|---|---|
| postgres | postgres:15-alpine | 5432 | `pg_isready` |
| redis | redis:7-alpine | 6379 | `redis-cli ping` |
| zookeeper | cp-zookeeper:7.5.0 | 2181 | — |
| kafka | cp-kafka:7.5.0 | 9092 (host) / 29092 (internal) | — |
| user-service | ./user-service | 8001 | `/health/ready` |
| product-service | ./product-service | 8081 | `/health/live` |
| cart-service | ./cart-service | 8002 | `/health/live` |
| order-service | ./order-service | 8082 | `/health/live` |
| payment-service | ./payment-service | 8003 | `/health/live` |

**Startup order:** postgres, redis → kafka → services

**Kafka note:** Inside Docker, services connect to `kafka:29092` (internal listener), not `kafka:9092` (host port).

## JWT Keys

RS256 keys are required for the user-service:

```bash
mkdir -p keys
openssl genrsa -out keys/private.pem 2048
openssl rsa -in keys/private.pem -pubout -out keys/public.pem
```

Paths configurable via `JWT_PRIVATE_KEY_PATH` / `JWT_PUBLIC_KEY_PATH` env vars.

## Database Access

Single PostgreSQL instance with 5 logical databases:

| Database | Service |
|---|---|
| `ecommerce_users` | user-service |
| `ecommerce_products` | product-service |
| `ecommerce_carts` | cart-service |
| `ecommerce_orders` | order-service |
| `ecommerce_payments` | payment-service |

Connect via psql:
```bash
docker exec ecommerce-postgres psql -U postgres -d ecommerce_users
```

### Stale Table Fix (Go Services)

If AutoMigrate fails with "constraint does not exist", drop the tables and restart:

```bash
docker exec ecommerce-postgres psql -U postgres -d ecommerce_users \
  -c "DROP TABLE IF EXISTS user_addresses, user_profiles, users CASCADE;"
docker compose restart user-service
```

## Environment Variables

All required variables are documented in `.env.example`. Key variables:

| Variable | Description | Default |
|---|---|---|
| `DB_HOST` | PostgreSQL host | `localhost` |
| `DB_PORT` | PostgreSQL port | `5432` |
| `DB_USER` | Database user | `postgres` |
| `DB_PASSWORD` | Database password | — |
| `REDIS_HOST` | Redis host | `localhost` |
| `REDIS_PORT` | Redis port | `6379` |
| `JWT_PRIVATE_KEY_PATH` | Path to RS256 private key | `./keys/private.pem` |
| `JWT_PUBLIC_KEY_PATH` | Path to RS256 public key | `./keys/public.pem` |
| `PRODUCT_SERVICE_URL` | Product service URL (for Cart) | `http://product-service:8081` |
| `KAFKA_BROKERS` | Kafka broker addresses | `kafka:29092` |
| `SMTP_HOST` | SMTP server for email verification | — |
| `SMTP_PORT` | SMTP port | `587` |
| `SMTP_USERNAME` | SMTP username | — |
| `SMTP_PASSWORD` | SMTP password | — |
| `SMTP_FROM` | Sender email address | — |

## API Testing

Each service has an `api.txt` file with curl-based testing commands. Full API specification: `api/openapi.yaml`.

```bash
# Example: Register a user
curl -X POST http://localhost:8001/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"SecurePass123!"}'
```

## Useful Commands

```bash
# View service logs
docker compose logs -f user-service

# Rebuild and restart a single service
docker compose up --build -d user-service

# Stop everything
docker compose down

# Stop and remove volumes (full reset)
docker compose down -v

# Seed sample users (1 admin, 1 customer, 1 seller — pre-verified)
docker exec ecommerce-postgres psql -U postgres -f /docker-entrypoint-initdb.d/sample_users.sql

# Check Kafka topics
docker exec ecommerce-kafka kafka-topics --list --bootstrap-server localhost:9092

# Redis CLI
docker exec ecommerce-redis redis-cli

# Monitor Postgres connections
docker exec ecommerce-postgres psql -U postgres -c "SELECT * FROM pg_stat_activity;"
```

## Makefile Targets

Current targets (user-service focused):

```bash
make infra-up       # Start postgres + redis
make deploy-user    # Build + start user-service
make test-user      # Run user-service tests with race detector
make db-seed        # Insert sample users
make db-nuke        # Drop all databases and recreate
```

Run `make help` or inspect `Makefile` for the full list.
