# User Service

The User Service is the authentication and identity management microservice in the e-commerce platform. It handles user registration, login with brute-force protection, JWT-based authentication (RS256), profile management, and address CRUD operations. Built with Go, Gin, and GORM.

## Project Structure

```
user-service/
├── cmd/server/main.go          # Entrypoint — wires dependencies and starts HTTP server
├── config/config.go            # Environment-based configuration with fallback defaults
├── internal/
│   ├── dto/                    # Request/response structs with validation tags
│   ├── handler/                # HTTP handlers (Gin) — auth, user, health
│   ├── middleware/             # Auth (JWT), recovery, structured JSON logger
│   ├── model/                  # GORM models — User, UserProfile, UserAddress, AuthToken
│   ├── repository/             # Database access layer (interface + GORM implementation)
│   ├── service/                # Business logic (depends only on repository interfaces)
│   └── integration/            # Integration tests (build tag: integration)
├── pkg/
│   ├── blacklist/              # JWT blacklist via Redis (blacklist:{jti})
│   ├── jwt/                    # RS256 JWT generation and validation
│   ├── loginattempt/           # Login attempt counter via Redis (login_attempts:{email})
│   ├── password/               # bcrypt hash and compare (cost 12)
│   ├── response/               # Standardized JSON response envelope helpers
│   └── session/                # Session cache via Redis (session:{userID}, 30 min TTL)
├── keys/                       # RSA key pair for JWT signing/verification
├── Dockerfile                  # Multi-stage production build (alpine)
└── Dockerfile.dev              # Development build with Air hot reload
```

## Data Flow

```
HTTP Request
  → Gin Router
    → Middleware (logger, recovery, auth)
      → Handler (validates input via DTOs)
        → Service (business logic, transactions)
          → Repository (interface — GORM implementation)
            → PostgreSQL / Redis
```

All layers propagate `context.Context` from the request down to database calls via `db.WithContext(ctx)`. Services depend only on repository **interfaces**, making them unit-testable with mocks.

## API Endpoints

### Health

| Method | Path            | Auth | Description                    |
|--------|-----------------|------|--------------------------------|
| GET    | `/health/live`  | No   | Liveness probe (always 200)    |
| GET    | `/health/ready` | No   | Readiness probe (pings PG + Redis) |

### Authentication (`/api/v1/auth`)

| Method | Path        | Auth | Description                          |
|--------|-------------|------|--------------------------------------|
| POST   | `/register` | No   | Create a new user account            |
| POST   | `/login`    | No   | Authenticate and receive JWT tokens  |
| POST   | `/refresh`  | No   | Exchange refresh token for new access token |
| POST   | `/logout`   | Yes  | Blacklist access token, revoke refresh tokens |

### User Management (`/api/v1/users`)

| Method | Path                    | Auth     | Description                       |
|--------|-------------------------|----------|-----------------------------------|
| GET    | `/:id`                  | Internal | Get user by ID (service-to-service only) |
| GET    | `/profile`              | Yes      | Get current user's profile        |
| PUT    | `/profile`              | Yes      | Update profile (invalidates session cache) |
| POST   | `/addresses`            | Yes      | Create a new address              |
| PUT    | `/addresses/:id`        | Yes      | Update an address (ownership check) |
| DELETE | `/addresses/:id`        | Yes      | Delete an address (ownership check) |
| PUT    | `/addresses/:id/default`| Yes      | Set address as default (atomic TX) |

All responses follow a consistent envelope:

```json
{ "success": true, "data": { ... } }
{ "success": false, "error": { ... } }
```

## Authentication Flow

```
┌─────────┐    POST /login     ┌──────────────┐
│  Client  │ ───────────────── │  Auth Handler │
└─────────┘                    └──────┬───────┘
                                      │
                          ┌───────────▼────────────┐
                          │  1. Redis pre-check     │  Check login_attempts:{email}
                          │     (brute-force gate)  │  If >= 5 → reject immediately
                          └───────────┬────────────┘
                                      │
                          ┌───────────▼────────────┐
                          │  2. SELECT ... FOR UPDATE│  Pessimistic lock on user row
                          │     (DB transaction)    │  Verify password (bcrypt)
                          │     Increment/reset     │  Update failed_login_attempts
                          │     attempt counter     │  TX always commits (auth errors
                          └───────────┬────────────┘  stored outside TX, not rolled back)
                                      │
                          ┌───────────▼────────────┐
                          │  3. On success:         │
                          │     - Generate access   │  RS256, 15 min TTL, includes jti
                          │       token (JWT)       │
                          │     - Generate refresh  │  128-char hex, stored as SHA-256
                          │       token             │    hash in auth_tokens table
                          │     - Cache session     │  Redis session:{userID}, 30 min
                          │     - Clear attempts    │  Delete login_attempts:{email}
                          └────────────────────────┘
```

- **Access token**: RS256 JWT, 15 min TTL, contains `jti` claim for blacklisting
- **Refresh token**: 128-char random hex, stored as SHA-256 hash in the `auth_tokens` table
- **Logout**: blacklists the `jti` in Redis (TTL = remaining token lifetime), deletes session cache, revokes all refresh tokens

## Database

**PostgreSQL** — database `ecommerce_users`

Tables (auto-migrated by GORM at startup):

| Table            | Description                                      |
|------------------|--------------------------------------------------|
| `users`          | Email, password hash, role, lock status, soft delete |
| `user_profiles`  | First/last name, phone — FK to users             |
| `user_addresses` | Street, city, state, zip, default flag — FK to users |
| `auth_tokens`    | SHA-256 hashed refresh tokens, expiry, revoked flag |

Connection pool: 25 max open, 5 idle, 5 min max lifetime.

## Redis Usage

| Key Pattern              | Purpose                    | TTL        |
|--------------------------|----------------------------|------------|
| `session:{userID}`       | Cached user profile (JSON) | 30 minutes |
| `blacklist:{jti}`        | Revoked JWT access tokens  | Remaining token lifetime |
| `login_attempts:{email}` | Failed login counter       | 15 min sliding window |

## Environment Variables

| Variable              | Default               | Description                  |
|-----------------------|-----------------------|------------------------------|
| `PORT`                | `8001`                | HTTP server port             |
| `DB_HOST`             | `localhost`           | PostgreSQL host              |
| `DB_PORT`             | `5432`                | PostgreSQL port              |
| `DB_USER`             | `postgres`            | PostgreSQL user              |
| `DB_PASSWORD`         | `postgres`            | PostgreSQL password          |
| `DB_NAME`             | `ecommerce_users`     | PostgreSQL database name     |
| `REDIS_HOST`          | `localhost`           | Redis host                   |
| `REDIS_PORT`          | `6379`                | Redis port                   |
| `REDIS_PASSWORD`      | _(empty)_             | Redis password               |
| `JWT_PRIVATE_KEY_PATH`| `./keys/private.pem`  | Path to RS256 private key    |
| `JWT_PUBLIC_KEY_PATH` | `./keys/public.pem`   | Path to RS256 public key     |
| `ENV`                 | `development`         | `development` or `production`|

## Running Locally

### Prerequisites

- Go 1.25+
- PostgreSQL 15+
- Redis 7+
- RSA key pair in `keys/` (already included for development)

### With Docker Compose (recommended)

```bash
# From the project root
cp .env.example .env          # configure if needed

docker compose up -d postgres redis
docker compose up -d user-service
```

### Without Docker

```bash
cd user-service

# Ensure PostgreSQL and Redis are running locally
# Set environment variables if defaults don't match your setup

go run ./cmd/server/main.go
```

### Running Tests

```bash
cd user-service

# Unit tests
go test -race ./...

# Integration tests (require running PostgreSQL + Redis)
docker compose up -d postgres redis
go test -tags=integration -v -race ./internal/integration/
```

## Tech Stack

- **Go** with [Gin](https://github.com/gin-gonic/gin) (HTTP framework) and [GORM](https://gorm.io) (ORM)
- **PostgreSQL** — persistent storage with pessimistic locking on login
- **Redis** — session cache, JWT blacklist, login attempt rate limiting
- **JWT RS256** — stateless authentication with `jti`-based revocation
- **testify** — unit testing with mocks; integration tests use real databases
- **testcontainers-go** — container-based integration test infrastructure
