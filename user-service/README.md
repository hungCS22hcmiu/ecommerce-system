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
│   ├── password/               # bcrypt hash/compare + bounded worker pool
│   ├── reset/                  # Password-reset tokens, cooldowns, attempt counters in Redis
│   ├── response/               # Standardized JSON response envelope helpers
│   ├── session/                # Session cache via Redis (session:{userID}, 30 min TTL)
│   └── verification/           # Email verification codes, cooldowns, attempt counters in Redis
├── migrations/                 # golang-migrate SQL migration files
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
| POST | `/verify-email` | No | Verify email with the 6-digit code sent at registration |
| POST | `/resend-verification` | No | Re-send the verification code (60 s cooldown) |
| POST | `/forgot-password` | No | Send a password-reset link via email (30 min token, 60 s cooldown) |
| POST | `/reset-password` | No | Set a new password using a reset token |

### User Management — `/api/v1/users`

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/:id` | Internal | Get user by ID (service-to-service, Docker-internal only) |
| GET | `/sellers/:id` | No | Public seller profile — name, avatar, join date (used by seller shop page) |
| GET | `/profile` | Yes | Get current user's profile |
| PUT | `/profile` | Yes | Update profile (invalidates session cache) |
| POST | `/addresses` | Yes | Create a new address |
| PUT | `/addresses/:id` | Yes | Update an address (ownership check) |
| DELETE | `/addresses/:id` | Yes | Delete an address (ownership check) |
| PUT | `/addresses/:id/default` | Yes | Set address as default (atomic TX) |

## Authentication Flow

```
POST /login
  → 1. Redis pre-check — login_attempts:{email} ≥ 5 → 403 (no DB hit)
  → 2. Plain DB read (no transaction, no FOR UPDATE) — fetch user + profile
  → 3. Bcrypt verify via bounded worker pool (runtime.NumCPU() goroutines)
       · Pool full → 503 + Retry-After: 1
       · Bad password → Redis INCR login_attempts:{email}
  → 4. Small TX (success only) — INSERT auth_tokens (refresh token hash)
  → 5. Post-success — Redis DEL login_attempts:{email} + SET session:{userID}
```

Login-attempt tracking is Redis-only — no `SELECT ... FOR UPDATE` on login.

**Access token:** RS256 JWT, 15 min TTL, contains `jti` for blacklisting.
**Refresh token:** 128-char random hex, stored as SHA-256 hash in `auth_tokens`.
**Logout:** blacklists the `jti` in Redis (TTL = remaining token lifetime), deletes session cache, revokes all refresh tokens for that user.

## Redis Usage

| Key Pattern | Purpose | TTL |
|---|---|---|
| `session:{userID}` | Cached user profile (JSON) | 30 min |
| `blacklist:{jti}` | Revoked JWT access tokens | Remaining token lifetime |
| `login_attempts:{email}` | Failed login counter | 15 min sliding window |
| `verification:{email}` | Email verification code | 15 min |
| `verification_cooldown:{email}` | Resend cooldown gate | 60 s |
| `verification_attempts:{email}` | Verify brute-force counter | 15 min |
| `password_reset:{email}` | Password-reset token | 30 min |
| `password_reset_cooldown:{email}` | Reset-request cooldown gate | 60 s |
| `password_reset_attempts:{email}` | Reset brute-force counter | 30 min |

## Database

**PostgreSQL** — `ecommerce_users`

| Table | Description |
|---|---|
| `users` | Email, password hash, role, lock status, soft delete |
| `user_profiles` | First/last name, phone — FK to users |
| `user_addresses` | Street, city, state, zip, default flag — FK to users |
| `auth_tokens` | SHA-256 hashed refresh tokens, expiry, revoked flag |

Connection pool: 50 max open, 10 idle, 5 min max lifetime. Schema managed by `golang-migrate` SQL migrations (`migrations/`); runs automatically at startup.

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
- `TestConcurrentLogin_SelectForUpdate_PreventsLockoutBypass` — 10 goroutines at `attempts=4`; proves the Redis INCR counter prevents any goroutine from bypassing the 5-attempt gate
- `TestAttemptCounter`, `TestJWTMiddleware_*` — Redis-backed attempt counter and token validation

## Tech Stack

- **Go** with Gin (HTTP) and GORM (ORM)
- **PostgreSQL** — persistent storage; login uses no row lock (Redis-authoritative attempt counter)
- **Redis** — session cache, JWT blacklist, login attempt rate limiting
- **JWT RS256** — stateless auth with `jti`-based revocation
- **testify** — unit testing; integration tests use real databases
