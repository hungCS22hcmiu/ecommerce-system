# user-service: How It Works

---

## 1. What Is It?

The `user-service` is a Go microservice (Gin + GORM) that owns **identity and access management** for the entire ecommerce platform.

**Analogy:** Think of it as a hotel's front desk and key card system. It checks you in (register), verifies your identity (login), issues a key card with an expiry (JWT), invalidates the card on checkout (logout), and manages your room preferences (profile/addresses). All other hotel floors (services) trust the card — they don't call the front desk again.

**Responsibilities:**
- User registration with email verification
- Login with brute-force protection and account locking
- RS256 JWT issuance (access + refresh tokens)
- Token revocation via Redis blacklist
- Session caching for fast downstream profile lookups
- Profile and address management

---

## 2. Why It Matters

### In this project
- Every protected endpoint across all 5 services is gated by JWT claims forwarded from this service.
- The session cache (`session:{userID}`) means product-service and cart-service can read user profile without hitting this service's DB on every request.
- The Redis-authoritative login attempt counter (`login_attempts:{email}`) is what prevents brute-force bypass — the DB columns `failed_login_attempts` / `is_locked` are schema artifacts from an earlier design and are no longer written at login time.

### In real-world systems
- Auth services are the highest-value attack surface. Getting brute-force protection, token storage, and revocation wrong causes account takeovers.
- Stateless JWTs at scale mean the service doesn't need to be in the hot path for every request — only at login and token refresh.
- The split between short-lived access tokens (15 min) and long-lived refresh tokens (7 days) is a deliberate security/UX tradeoff used by Google, GitHub, and every OAuth 2.0 system.

---

## 3. How It Works — Step-by-Step Flows

### Registration
```
POST /api/v1/auth/register
    │
    ├─ Check duplicate email (FindByEmail)
    ├─ bcrypt hash password
    ├─ INSERT user + profile (GORM transaction)
    ├─ Generate 6-digit code (crypto/rand)
    ├─ Store code in Redis: verification:{email} TTL=15min
    └─ Send code via SMTP (async best-effort)
```

### Login — the critical path
```
POST /api/v1/auth/login
    │
    ├─ 1. Redis pre-check: login_attempts:{email} >= 5 → 403 Locked (no DB hit)
    │
    ├─ 2. Plain DB read (no transaction, no FOR UPDATE)
    │       SELECT * FROM users WHERE email=? (with profile join)
    │       user not found → ErrInvalidCredentials
    │
    ├─ 3. Bcrypt verify via worker pool (outside any DB lock)
    │       Pool full → 503 + Retry-After: 1
    │       Mismatch → Redis INCR login_attempts:{email} (best-effort)
    │               → ErrInvalidCredentials
    │       Check is_verified → ErrEmailNotVerified if false
    │
    ├─ 4. Small TX (success path only)
    │       GenerateAccessToken (RS256 JWT, jti=UUID, TTL=15min)
    │       GenerateRefreshToken (128-char hex, opaque)
    │       INSERT auth_tokens (SHA-256 hash of refresh token)
    │       COMMIT
    │
    └─ 5. Post-success Redis updates
            DEL login_attempts:{email}
            SET session:{userID} (JSON profile, 30 min TTL)
```

**Why login attempt tracking is Redis-only:** The attempt counter (`INCR login_attempts:{email}`) is a best-effort operation outside any DB transaction. A Redis failure on increment is logged but never surfaces to the caller — it's intentionally fire-and-forget. This eliminates any DB row lock held during bcrypt (which takes 100ms+), avoiding serialization of concurrent login attempts for the same account. The Redis pre-check at step 1 is the enforcement gate; the counter increment at step 3 is the write.

### Token Refresh
```
POST /api/v1/auth/refresh  (body: refresh_token)
    │
    ├─ Hash refresh token (SHA-256) → look up auth_tokens table
    ├─ Cache hit?  → read session:{userID} from Redis
    ├─ Cache miss? → SELECT user from DB, warm Redis cache
    └─ Issue new RS256 access token (refresh token unchanged)
```

### Logout
```
POST /api/v1/auth/logout  (Authorization: Bearer <access>)
    │
    ├─ ValidateToken → extract jti + expiry
    ├─ Redis SET blacklist:{jti} = "" TTL = remaining token lifetime
    ├─ Redis DEL session:{userID}
    └─ UPDATE auth_tokens SET revoked=true WHERE user_id=?
```

### Forgot Password
```
POST /api/v1/auth/forgot-password  (body: email)
    │
    ├─ Redis HasCooldown(password_reset_cooldown:{email}) → 429 if set
    ├─ FindByEmail → not found → return nil (no enumeration leak)
    ├─ GenerateRefreshToken() as reset token (128-char hex, opaque)
    ├─ Redis SET password_reset:{email} = token   TTL=30min
    ├─ Redis SET password_reset_cooldown:{email}  TTL=60s
    └─ emailSender.SendPasswordReset (async best-effort)
```

### Reset Password
```
POST /api/v1/auth/reset-password  (body: email, token, new_password)
    │
    ├─ Redis INCR password_reset_attempts:{email} > 5 → 429
    ├─ Redis GET password_reset:{email} → mismatch or empty → 400
    ├─ FindByEmail → hash new password (bcrypt) → UpdatePassword
    ├─ Redis DEL password_reset:{email} + attempts key
    └─ RevokeByUserID → forces re-login on all devices
```

### Request Authentication (middleware)
```
Every protected request
    │
    ├─ Parse "Authorization: Bearer <token>"
    ├─ RSA-verify signature + check exp
    ├─ Redis GET blacklist:{jti} → present → 401
    └─ Inject userID, role, jti into Gin context → next handler
```

---

## 4. System Design — Components & Architecture

```
                         ┌──────────────────────────────────────────┐
                         │              user-service                 │
                         │                                           │
  HTTP ──────────────────┤  Gin Router                               │
                         │    │                                      │
                         │    ├── middleware.Auth (JWT + blacklist)  │
                         │    │                                      │
                         │    ├── AuthHandler ──► AuthService        │
                         │    │                       │              │
                         │    └── UserHandler ──► UserService        │
                         │                           │               │
                         └───────────────────────────┼───────────────┘
                                                     │
                    ┌────────────────────────────────┼──────────────────────┐
                    │                                │                      │
              ┌─────▼──────┐                  ┌──────▼──────┐        ┌──────▼──────┐
              │ PostgreSQL  │                  │    Redis     │        │    SMTP     │
              │             │                  │              │        │             │
              │ users        │                  │ session:{id}            │        │ verify email│
              │ user_profiles│                  │ blacklist:{jti}         │        │ reset link  │
              │ user_addresses│                 │ login_attempts:{email}  │        └─────────────┘
              │ auth_tokens  │                  │ verification:{email}    │
              └─────────────┘                  │ password_reset:{email}  │
                                               └─────────────────────────┘
```

### Key packages

| Package | Role |
|---|---|
| `pkg/jwt` | RS256 sign/verify, token loading from PEM files |
| `pkg/blacklist` | Redis `blacklist:{jti}` — O(1) revocation check |
| `pkg/session` | Redis `session:{userID}` — JSON-marshaled UserResponse, 30 min TTL |
| `pkg/loginattempt` | Redis `login_attempts:{email}` — sliding 15 min TTL counter |
| `pkg/verification` | Redis verification code + attempt tracking + 60 s resend cooldown |
| `pkg/reset` | Redis password-reset tokens + attempt tracking + 60 s cooldown |
| `pkg/password` | bcrypt Hash/Compare + bounded worker pool (`runtime.NumCPU()` goroutines) |
| `pkg/email` | SMTP STARTTLS sender (verification codes + reset links) |
| `internal/repository` | GORM implementations behind interfaces → testable |
| `internal/service` | Business logic, depends only on interfaces |
| `internal/handler` | HTTP layer, parses/validates DTOs, delegates to service |

### Data models

```
users
  id UUID PK (gen_random_uuid)
  email VARCHAR UNIQUE NOT NULL
  password_hash VARCHAR NOT NULL
  role VARCHAR DEFAULT 'customer'
  is_locked BOOL DEFAULT false
  failed_login_attempts INT DEFAULT 0
  is_verified BOOL DEFAULT false
  verified_at TIMESTAMPTZ
  deleted_at TIMESTAMPTZ  ← soft-delete

user_profiles (1:1 with users)
  user_id UUID FK
  first_name, last_name, phone, avatar_url

user_addresses (1:many with users)
  user_id UUID FK
  label VARCHAR       ← e.g. "Home", "Work"
  address_line1/2, city, state, country, postal_code
  is_default BOOL

auth_tokens (refresh tokens)
  id UUID PK
  user_id UUID FK
  refresh_token_hash VARCHAR UNIQUE  ← SHA-256, never stores raw token
  expires_at TIMESTAMPTZ
  revoked BOOL
```

---

## 5. Code Examples

### Redis-only login attempt counter + bcrypt pool
```go
// auth_service.go — Login()

// 1. Fast gate: no DB hit for locked accounts
if count, _ := s.attemptCounter.Get(ctx, req.Email); count >= maxLoginAttempts {
    return nil, ErrAccountLocked
}

// 2. Plain read — no FOR UPDATE, no transaction
user, err := s.userRepo.FindByEmailWithProfile(ctx, req.Email)

// 3. Bcrypt outside any lock — pool caps goroutine saturation
verifyErr = s.bcryptPool.Verify(ctx, user.PasswordHash, req.Password)
if verifyErr != nil {
    s.attemptCounter.Increment(ctx, req.Email) // best-effort Redis INCR
    return nil, ErrInvalidCredentials
}

// 4. Small TX: only token persistence, not attempt tracking
s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    // INSERT auth_tokens (SHA-256 hash of rawRefresh)
    return s.authTokenRepo.Create(ctx, tx, authToken)
})

// 5. Post-success: clear counter + warm session cache
s.attemptCounter.Delete(ctx, req.Email)
s.sessionCache.Set(ctx, userID, resp.User, sessionTTL)
```

### JWT middleware blacklist check
```go
// auth_middleware.go:34
claims, err := jwtpkg.ValidateToken(tokenStr, publicKey)
if err != nil { /* 401 */ }

blacklisted, err := bl.Contains(c.Request.Context(), claims.ID) // O(1) Redis GET
if blacklisted { /* 401 revoked */ }

c.Set(CtxUserID, claims.UserID) // downstream handlers read this
```

### Opaque refresh token storage
```go
// Never store raw refresh token — only its hash
authToken := &model.AuthToken{
    UserID:           user.ID,
    RefreshTokenHash: hashToken(rawRefresh), // SHA-256 hex
    ExpiresAt:        time.Now().Add(refreshTokenTTL),
}
// Return rawRefresh to client; DB only holds the hash
```

---

## 6. Trade-offs

### Redis-only login attempt counter (no `SELECT FOR UPDATE`)

| Pro | Con |
|---|---|
| No DB row lock held during 100ms+ bcrypt — no serialization | Redis failure silently drops an increment (best-effort) |
| Horizontal scale: any instance reads/writes the same Redis counter | INCR is not atomic with the DB read — theoretical double-count at the boundary |
| O(1) and non-blocking for the common case | `failed_login_attempts` / `is_locked` DB columns are no longer authoritative |

**Why this is acceptable:** The lockout threshold (5 attempts) has enough slack that an occasional missed increment doesn't create a meaningful bypass window. Redis availability (99.9%+) makes the failure mode rare, and the bcrypt cost already throttles attackers without a hard lock.

### Bcrypt worker pool

| Pro | Con |
|---|---|
| Caps CPU saturation — bcrypt can't spawn unbounded goroutines | Queue size (256) is a config knob; wrong value = too-early 503s or OOM |
| Clean load shedding: queue full → immediate 503 + `Retry-After: 1` | Pool starts cold — first `runtime.NumCPU()` requests warm it |
| Workers drain on graceful shutdown (pool.Stop() called after srv.Shutdown) | Adds one goroutine-hop latency per login |

### Short-lived access tokens (15 min)

| Pro | Con |
|---|---|
| Limits damage window if token is stolen | Client must implement refresh logic |
| Stateless verification (no DB/Redis hit per request) | UX friction on token expiry without silent refresh |
| Redis blacklist only needs to hold entries ≤ 15 min | |

### Redis session cache

| Pro | Con |
|---|---|
| Avoids DB round-trip on every refresh | Stale data risk if profile changes between cache entries |
| 30 min TTL bounds staleness | Profile update must explicitly invalidate: `sessionCache.Delete` |
| Cache miss falls back to DB silently | Two sources of truth during cache lifetime |

---

## 7. When to Use / Avoid

### Use this pattern when:
- You need **account lockout** and can tolerate best-effort counter semantics — Redis INCR is sufficient for most threat models
- Your access token TTL is short enough that a Redis blacklist is practical (entries expire naturally)
- Services consuming identity are deployed on the same internal network and can trust forwarded headers from a trusted gateway
- Login volumes are high and you cannot afford to hold a DB row lock for the duration of bcrypt

### Avoid when:
- **Lockout precision is a hard security requirement** — if even one missed increment is unacceptable (e.g., financial systems), back the counter with a DB write inside a transaction
- **Microservices span trust boundaries** — forwarding `X-Seller-Id` without signature works only inside a private network; add HMAC or mTLS if services span zones
- **You need refresh token rotation** — current implementation reuses the same refresh token; for higher security (e.g., refresh token families), rotate on every use and revoke the family on replay detection

---

## 8. Interview Insights

### Q: Why is login attempt tracking Redis-only instead of a DB transaction?
**A:** The original design used `SELECT FOR UPDATE` to serialize concurrent logins and guarantee the attempt counter incremented correctly. It was replaced because bcrypt takes 100ms+, and holding a DB row lock for that duration serializes every login attempt for the same email — unacceptable under load. The Redis `INCR` approach is O(1) and non-blocking. The tradeoff: Redis `INCR` is best-effort — a Redis failure silently drops an increment. For most systems this is acceptable because: (1) the lockout threshold (5 attempts) has enough slack, and (2) bcrypt itself already throttles attackers without a hard lock.

### Q: Why is there a bcrypt worker pool instead of calling bcrypt directly?
**A:** `bcrypt.CompareHashAndPassword` is CPU-bound and takes ~100ms at cost 10. Without a pool, a burst of login requests would spawn an unbounded number of goroutines each saturating a CPU core, potentially starving the rest of the server. The pool (`runtime.NumCPU()` workers, queue 256) caps concurrent bcrypt ops to the number of CPU cores. When the queue is full, new requests get an immediate 503 + `Retry-After: 1` rather than piling up and exhausting memory.

### Q: How does logout work if JWTs are stateless?
**A:** Stateless means we can't "delete" a token. Instead, we maintain a **blacklist** in Redis: on logout, the token's `jti` (a UUID in the claims) is added with TTL = remaining lifetime. The auth middleware checks this on every request. Since access tokens are only 15 minutes, the Redis key auto-expires and the blacklist stays small.

### Q: Why hash the refresh token before storing it?
**A:** If the DB is compromised, raw refresh tokens would be usable. Storing SHA-256(token) means an attacker with read access to `auth_tokens` cannot replay the tokens — they'd need to reverse SHA-256. The raw token is only ever in memory and in transit (HTTPS).

### Q: How would you scale this if login becomes a bottleneck?
**A:** Several levers:
1. **Read replicas** — Refresh (profile lookup on cache miss) can go to a read replica.
2. **Connection pool tuning** — Already configured (50 max open, 10 idle, 5 min lifetime). Can increase if DB can handle more connections.
3. **Bcrypt pool sizing** — Increase `NewPool(queueSize)` and consider more workers if CPU allows; or lower `BCRYPT_COST` in dev/test.
4. **Horizontal scaling** — Stateless JWT verification means any instance can validate tokens without coordination. Session cache and attempt counter in Redis are already shared. Only login (DB writes for refresh token) are sticky to the DB primary.

### Q: What's the difference between `session cache` and `blacklist`?
**A:** They serve opposite purposes:
- **Session cache** (`session:{userID}`) = positive cache. Stores who you are so downstream services don't query the DB. Gets invalidated on logout or profile update.
- **Blacklist** (`blacklist:{jti}`) = negative cache. Records revoked tokens so the stateless JWT check still catches logged-out tokens. Auto-expires with the token TTL.
