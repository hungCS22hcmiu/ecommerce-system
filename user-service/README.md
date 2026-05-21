# user-service

Authentication and identity microservice for the e-commerce platform. Handles registration, login with brute-force protection, JWT-based authentication (RS256), profile management, address CRUD, and a public seller profile endpoint. Built with Go, Gin, and GORM.

- **Port:** 8001
- **Database:** PostgreSQL (`ecommerce_users`)
- **Redis:** sessions, JWT blacklist, login attempt counter

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

## API Endpoints

### Health

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health/live` | No | Liveness probe (always 200) |
| GET | `/health/ready` | No | Readiness probe (pings PG + Redis) |

### Authentication — `/api/v1/auth`

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/register` | No | Create a new user account |
| POST | `/login` | No | Authenticate and receive JWT tokens |
| POST | `/refresh` | No | Exchange refresh token for new access token |
| POST | `/logout` | Yes | Blacklist access token, revoke refresh tokens |
| GET | `/verify-email` | No | Verify email address via token from registration email |
| POST | `/resend-verification` | No | Re-send the verification email |

### User Management — `/api/v1/users`

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/:id` | Internal | Get user by ID (service-to-service only) |
| GET | `/:id/seller-profile` | No | Public seller profile — name, joined date (used by seller shop page) |
| GET | `/profile` | Yes | Get current user's profile |
| PUT | `/profile` | Yes | Update profile (invalidates session cache) |
| POST | `/addresses` | Yes | Create a new address |
| PUT | `/addresses/:id` | Yes | Update an address (ownership check) |
| DELETE | `/addresses/:id` | Yes | Delete an address (ownership check) |
| PUT | `/addresses/:id/default` | Yes | Set address as default (atomic TX) |

## Authentication Flow

```
POST /login
  → 1. Redis pre-check (login_attempts:{email} ≥ 5 → reject immediately)
  → 2. SELECT user by email, submit password to bcrypt worker pool
       · Worker pool: runtime.NumCPU() goroutines; pool full → 503 + Retry-After: 1
       · Increment or reset login_attempts:{email} in Redis (TX always commits)
  → 3. On success:
       · RS256 JWT access token (15 min TTL, includes jti)
       · 128-char hex refresh token (stored as SHA-256 hash in auth_tokens)
       · Cache session in Redis (session:{userID}, 30 min)
       · Clear login_attempts:{email}
```

Login-attempt tracking is Redis-only — no `SELECT ... FOR UPDATE` row lock on login.

**Access token:** RS256 JWT, 15 min TTL, contains `jti` for blacklisting.
**Refresh token:** 128-char random hex, stored as SHA-256 hash in `auth_tokens`.
**Logout:** blacklists the `jti` in Redis (TTL = remaining token lifetime), deletes session cache, revokes all refresh tokens.

## Redis Usage

| Key Pattern | Purpose | TTL |
|---|---|---|
| `session:{userID}` | Cached user profile (JSON) | 30 min |
| `blacklist:{jti}` | Revoked JWT access tokens | Remaining token lifetime |
| `login_attempts:{email}` | Failed login counter | 15 min sliding window |

## Database

**PostgreSQL** — `ecommerce_users`

| Table | Description |
|---|---|
| `users` | Email, password hash, role, lock status, soft delete |
| `user_profiles` | First/last name, phone — FK to users |
| `user_addresses` | Street, city, state, zip, default flag — FK to users |
| `auth_tokens` | SHA-256 hashed refresh tokens, expiry, revoked flag |

Connection pool: 25 max open, 5 idle, 5 min max lifetime. Schema auto-migrated by GORM at startup.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8001` | HTTP server port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | PostgreSQL user |
| `DB_PASSWORD` | `postgres` | PostgreSQL password |
| `DB_NAME` | `ecommerce_users` | PostgreSQL database name |
| `REDIS_HOST` | `localhost` | Redis host |
| `REDIS_PORT` | `6379` | Redis port |
| `REDIS_PASSWORD` | _(empty)_ | Redis password |
| `JWT_PRIVATE_KEY_PATH` | `./keys/private.pem` | Path to RS256 private key |
| `JWT_PUBLIC_KEY_PATH` | `./keys/public.pem` | Path to RS256 public key |
| `SMTP_HOST` | — | Required for email verification |
| `SMTP_PORT` | — | |
| `SMTP_USERNAME` | — | |
| `SMTP_PASSWORD` | — | |
| `BCRYPT_COST` | `12` | bcrypt work factor (docker-compose uses `10` for faster dev startup) |
| `ENV` | `development` | `development` or `production` |

## Running

**With Docker Compose (recommended)**
```bash
cp .env.example .env
docker compose up -d postgres redis
docker compose up -d user-service
```

**Locally**
```bash
cd user-service
go run ./cmd/server/main.go
```

## Testing

```bash
# Unit tests
go test -race ./...

# Integration tests (require running PostgreSQL + Redis)
docker compose up -d postgres redis
go test -tags=integration -v -race ./internal/integration/
```

Notable integration tests:
- `TestConcurrentLogin_SelectForUpdate_PreventsLockoutBypass` — 10 goroutines at `attempts=4`; proves Redis-based attempt counter serializes correctly, no goroutine bypasses the 5-attempt gate
- `TestAttemptCounter`, `TestJWTMiddleware_*` — Redis-backed attempt counter and token validation

## Tech Stack

- **Go** with Gin (HTTP) and GORM (ORM)
- **PostgreSQL** — persistent storage with pessimistic locking on login
- **Redis** — session cache, JWT blacklist, login attempt rate limiting
- **JWT RS256** — stateless auth with `jti`-based revocation
- **testify** — unit testing; integration tests use real databases
