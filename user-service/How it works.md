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
- The pessimistic lock on login (`SELECT FOR UPDATE`) is critical — without it, two concurrent login requests can both read `FailedLoginAttempts = 4`, both increment to 5, but only one actually locks the account in DB.

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
    ├─ [FAST PATH] Redis pre-check: login_attempts:{email} >= 5 → 423 Locked
    │
    ├─ BEGIN TRANSACTION
    │   ├─ SELECT * FROM users WHERE email=? FOR UPDATE  ← pessimistic lock
    │   ├─ user not found → loginErr=ErrInvalidCredentials, COMMIT (no-op)
    │   ├─ user.IsLocked → loginErr=ErrAccountLocked, COMMIT
    │   ├─ bcrypt.Compare fails:
    │   │   ├─ UPDATE failed_login_attempts++, is_locked=(attempts>=5)
    │   │   └─ COMMIT  ← counter persists even on auth failure
    │   └─ password OK:
    │       ├─ UPDATE failed_login_attempts=0, is_locked=false
    │       ├─ Check is_verified (else ErrEmailNotVerified, COMMIT)
    │       ├─ GenerateAccessToken (RS256 JWT, jti=UUID, TTL=15min)
    │       ├─ GenerateRefreshToken (128-char hex, opaque)
    │       ├─ INSERT auth_tokens (SHA-256 hash of refresh token)
    │       └─ COMMIT
    │
    ├─ [POST-TX] bad password → Redis INCR login_attempts:{email}
    └─ [POST-TX] success → Redis DEL counter + SET session:{userID}
```

**Why TX always commits on auth errors:** If the TX rolled back on wrong password, the `UpdateLoginAttempts` write would be lost, making brute-force protection impossible. Auth errors are stored in an outer `loginErr` variable returned after the TX.

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
              │ users        │                  │ session:{id} │        │ verify email│
              │ user_profiles│                  │ blacklist:{jti}│      │             │
              │ user_addresses│                 │ login_attempts│       └─────────────┘
              │ auth_tokens  │                  │ verification: │
              └─────────────┘                  └──────────────┘
```

### Key packages

| Package | Role |
|---|---|
| `pkg/jwt` | RS256 sign/verify, token loading from PEM files |
| `pkg/blacklist` | Redis `blacklist:{jti}` — O(1) revocation check |
| `pkg/session` | Redis `session:{userID}` — JSON-marshaled UserResponse, 30 min TTL |
| `pkg/loginattempt` | Redis `login_attempts:{email}` — sliding 15 min TTL counter |
| `pkg/verification` | Redis verification code + attempt tracking + 60s resend cooldown |
| `pkg/password` | bcrypt Hash/Compare wrapper |
| `pkg/email` | SMTP STARTTLS sender |
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

user_profiles (1:1 with users)
  user_id UUID FK
  first_name, last_name, phone, avatar_url

auth_tokens (refresh tokens)
  id UUID PK
  user_id UUID FK
  refresh_token_hash VARCHAR  ← SHA-256, never stores raw token
  expires_at TIMESTAMPTZ
  is_revoked BOOL
```

---

## 5. Code Examples

### The TX-always-commits login pattern
```go
// auth_service.go:170
txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    user, err := s.userRepo.FindByEmailForUpdate(ctx, tx, req.Email)
    if errors.Is(err, repository.ErrNotFound) {
        loginErr = ErrInvalidCredentials
        return nil // ← COMMIT the no-op TX, not rollback
    }

    if !password.Compare(user.PasswordHash, req.Password) {
        s.userRepo.UpdateLoginAttempts(ctx, tx, user.ID, newAttempts, locked)
        loginErr = ErrInvalidCredentials
        return nil // ← COMMIT so counter write persists
    }
    // ... generate tokens ...
    return nil // ← COMMIT with tokens
})
// loginErr checked AFTER tx — real DB errors are the only rollbacks
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

### Pessimistic lock on login (`SELECT FOR UPDATE`)

| Pro | Con |
|---|---|
| Correctness guaranteed — only one TX reads + increments at a time | Serializes concurrent login attempts for same email |
| Prevents TOCTOU race on account lockout | Holds DB row lock for duration of bcrypt (100ms+) |
| Simple to reason about | Doesn't scale if the same account is hit from many IPs simultaneously |

**Mitigation:** Redis pre-check at the top of `Login()` short-circuits before the DB even for the common case (already locked), reducing lock contention.

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
- You need **account lockout** correctness under concurrent load — pessimistic locking is the right call
- Your access token TTL is short enough that a Redis blacklist is practical (entries expire naturally)
- Services consuming identity are deployed on the same internal network and can trust forwarded headers from a trusted gateway

### Avoid when:
- **Very high login throughput for the same account** — the `FOR UPDATE` lock will serialize all attempts; consider a Redis-only counter approach (INCR + EXPIRE) if lockout precision matters less than throughput
- **Microservices span trust boundaries** — forwarding `X-Seller-Id` without signature works only inside a private network; add HMAC or mTLS if services span zones
- **You need refresh token rotation** — current implementation reuses the same refresh token; for higher security (e.g., refresh token families), rotate on every use and revoke the family on replay detection

---

## 8. Interview Insights

### Q: Why use `SELECT FOR UPDATE` on login instead of an optimistic approach?
**A:** Login is write-heavy for failed attempts. With optimistic locking you'd need a version field, and retries on concurrent failures would mean some login attempts never update the counter at all — breaking lockout semantics. Pessimistic lock gives a simple guarantee: at most one writer at a time per user row. The tradeoff is serialization on that row, acceptable because the common case (successful login from a single user) has no contention.

### Q: Why does the transaction commit even on wrong password?
**A:** The `UpdateLoginAttempts` write must persist to enforce brute-force lockout. If we returned an error from the TX callback, GORM would roll back and the counter increment would be lost. The pattern: store auth errors in an outer variable (`loginErr`), always `return nil` from the TX callback for auth failures, only `return err` for real DB errors. The outer caller checks `loginErr` after the TX.

### Q: How does logout work if JWTs are stateless?
**A:** Stateless means we can't "delete" a token. Instead, we maintain a **blacklist** in Redis: on logout, the token's `jti` (a UUID in the claims) is added with TTL = remaining lifetime. The auth middleware checks this on every request. Since access tokens are only 15 minutes, the Redis key auto-expires and the blacklist stays small.

### Q: Why hash the refresh token before storing it?
**A:** If the DB is compromised, raw refresh tokens would be usable. Storing SHA-256(token) means an attacker with read access to `auth_tokens` cannot replay the tokens — they'd need to reverse SHA-256. The raw token is only ever in memory and in transit (HTTPS).

### Q: How would you scale this if login becomes a bottleneck?
**A:** Several levers:
1. **Read replicas** — Refresh (profile lookup on cache miss) can go to a read replica.
2. **Connection pool tuning** — Already configured (25 max open, 5 idle, 5 min lifetime).
3. **Redis counter instead of DB lock for attempt tracking** — Sacrifice perfect precision for throughput; Redis INCR is O(1) and non-blocking.
4. **Horizontal scaling** — Stateless JWT verification means any instance can validate tokens without coordination. Session cache in Redis is already shared. Only login (writes) are sticky to the DB primary.

### Q: What's the difference between `session cache` and `blacklist`?
**A:** They serve opposite purposes:
- **Session cache** (`session:{userID}`) = positive cache. Stores who you are so downstream services don't query the DB. Gets invalidated on logout or profile update.
- **Blacklist** (`blacklist:{jti}`) = negative cache. Records revoked tokens so the stateless JWT check still catches logged-out tokens. Auto-expires with the token TTL.
