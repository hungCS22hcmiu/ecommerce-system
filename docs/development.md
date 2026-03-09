# Development Guide

## Prerequisites

| Tool | Version | Install |
|---|---|---|
| Golang | 1.21+ | `brew install go` |
| Java | 17 or 21 LTS | `brew install openjdk@21` |
| Maven | 3.9+ | `brew install maven` |
| Docker | 24+ | `brew install --cask docker` |
| Docker Compose | 2.x | Included with Docker Desktop |
| Node.js | 20+ | `brew install node` (for frontend) |
| k6 | latest | `brew install k6` (for load testing) |

## Quick Start

```bash
git clone https://github.com/hungCS22hcmiu/ecommrece-system.git
cd ecommrece-system

# Configure environment
cp .env.example .env
# Edit .env with actual values

# Start everything
docker compose up --build -d

# Verify
curl http://localhost/health
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
```

### Hot Reload (Development)

Go services include `Dockerfile.dev` + `.air.toml` for Air hot reload:

```bash
# Via Docker Compose (volume-mounted source)
docker compose up user-service-dev
```

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

Java version: 21. Spring Boot: 3.5. Uses Flyway for DB migrations.

## Docker Compose Services

| Container | Image | Port | Health Check |
|---|---|---|---|
| nginx | nginx:1.25-alpine | 80 | `curl localhost:80/health` |
| postgres | postgres:15-alpine | 5432 | `pg_isready` |
| redis | redis:7-alpine | 6379 | `redis-cli ping` |
| zookeeper | cp-zookeeper:7.5.0 | 2181 | — |
| kafka | cp-kafka:7.5.0 | 9092 | — |
| user-service | ./user-service | 8001 | `/health/ready` |
| product-service | ./product-service | 8081 | `/health/ready` |
| cart-service | ./cart-service | 8002 | `/health/ready` |
| order-service | ./order-service | 8082 | `/health/ready` |
| payment-service | ./payment-service | 8003 | `/health/ready` |

**Startup order:** postgres, redis → kafka → services → nginx

**Kafka note:** Inside Docker, services connect to `kafka:29092` (internal listener), not `kafka:9092` (host port).

## JWT Keys

RS256 keys are required for the user-service. Generate them locally (they're gitignored):

```bash
cd user-service/keys/
openssl genrsa -out private.pem 2048
openssl rsa -in private.pem -pubout -out public.pem
```

Paths are configurable via `JWT_PRIVATE_KEY_PATH` / `JWT_PUBLIC_KEY_PATH` env vars.

## Database Access

Single PostgreSQL instance with 5 logical databases:

| Database | Service | Port |
|---|---|---|
| `ecommerce_users` | user-service | 5432 |
| `ecommerce_products` | product-service | 5432 |
| `ecommerce_carts` | cart-service | 5432 |
| `ecommerce_orders` | order-service | 5432 |
| `ecommerce_payments` | payment-service | 5432 |

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

## API Testing

Each service has an `api.txt` file with curl-based testing commands:

```bash
# Example: Register a user
curl -X POST http://localhost:8001/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"SecurePass123!"}'
```

Full API specification: `api/openapi.yaml` (served via Swagger UI at `/swagger` through Nginx).

## Profiling

### Go (pprof)

Built-in — zero external dependencies:
```bash
# CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile

# Goroutine dump
curl http://localhost:6060/debug/pprof/goroutine?debug=1

# Memory
go tool pprof http://localhost:6060/debug/pprof/heap
```

### Java (JFR)

```bash
# Start recording
java -XX:StartFlightRecording=duration=60s,filename=recording.jfr -jar app.jar

# Analyze with JDK Mission Control (jmc)
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

# Check Kafka topics
docker exec ecommerce-kafka kafka-topics --list --bootstrap-server localhost:9092

# Monitor Postgres connections
docker exec ecommerce-postgres psql -U postgres -c "SELECT * FROM pg_stat_activity;"

# Redis CLI
docker exec ecommerce-redis redis-cli
```
