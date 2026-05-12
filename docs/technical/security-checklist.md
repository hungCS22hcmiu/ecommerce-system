# Security Checklist — Week 16 Audit

Covers all five microservices and the Nginx gateway. Assessed against OWASP API Security Top 10 (2023).

---

## Per-Service Status

| Item | user-service | product-service | cart-service | order-service | payment-service |
|---|---|---|---|---|---|
| Input validation | ✅ | ✅ | ✅ | ✅ | ✅ |
| Mass assignment (DTOs, not entities) | ✅ | ✅ | ✅ | ✅ | ✅ |
| Auth middleware on protected routes | ✅ | ⚠️ (1) | ✅ | ⚠️ (2) | ✅ |
| Ownership / access checks | ✅ | ✅ | ✅ | ✅ (3) | ✅ |
| Parameterized queries (no SQL concat) | ✅ GORM | ✅ JPA | ✅ GORM | ✅ JPA | ✅ GORM |
| Secrets not hardcoded | ✅ | ✅ | ✅ | ✅ | ✅ |

**Notes:**

(1) **product-service** — write endpoints (`POST/PUT/DELETE /products`) rely on `X-Seller-Id` header forwarded by Nginx. No JWT validation inside the service. Acceptable for internship scope; production would add Spring Security + JWT filter.

(2) **order-service** — reads `X-User-Id` header, which is forwarded by Nginx but not signed. A full production gateway would strip client-supplied `X-User-Id` and inject it from the validated JWT. Deferred to a future auth-gateway phase.

(3) **order-service** — `GET /{id}`, `GET /history`, and `PUT /{id}/cancel` all verify `order.userId == requestingUserId`. The `PUT /{id}/ship` and `PUT /{id}/deliver` endpoints were identified as missing a seller/role check; mitigated in Week 16 by blocking them at the Nginx layer (see below).

---

## OWASP API Security Top 10 — System Assessment

| # | Risk | Status | Notes |
|---|---|---|---|
| API1 | Broken Object Level Authorization | ✅ Mitigated | Ownership checks on all user-scoped reads; ship/deliver blocked at Nginx |
| API2 | Broken Authentication | ✅ Mitigated | RS256 JWT, 15-min TTL, blacklist on logout, login attempt lockout |
| API3 | Broken Object Property Level Auth | ✅ Mitigated | DTOs used everywhere; no entity binding; no internal fields exposed |
| API4 | Unrestricted Resource Consumption | ✅ Mitigated | Nginx rate limiting (10 req/s general, 5 req/min on auth) |
| API5 | Broken Function Level Authorization | ⚠️ Partial | ship/deliver require seller role — blocked externally via Nginx; no in-service RBAC |
| API6 | Unrestricted Access to Sensitive Flows | ✅ Mitigated | Inventory reserve/release blocked at Nginx; payment creation internal-only |
| API7 | Server-Side Request Forgery | ✅ Low risk | No user-supplied URLs fetched by any service |
| API8 | Security Misconfiguration | ✅ Mitigated | Security headers, CORS tightened, no debug endpoints exposed publicly |
| API9 | Improper Assets Management | ✅ Mitigated | Health endpoints not rate-limited but return no sensitive data |
| API10 | Unsafe Consumption of APIs | ✅ Mitigated | cart-service validates product data from product-service before persisting |

---

## Week 16 Changes Applied

### nginx/nginx.conf

| Change | Detail |
|---|---|
| Added `Referrer-Policy: no-referrer` | Prevents referrer leakage in cross-origin requests |
| Added `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'` | Appropriate for a pure JSON API (no HTML served) |
| CORS `*` → `http://localhost:3000` | Scoped to React frontend dev server; update to production domain at deploy time |
| Auth rate limit zone `auth_limit` (5 req/min) | Applied to `/api/v1/auth` alongside the general 10 req/s limit to slow brute-force attempts |
| Block `PUT /api/v1/orders/:id/(ship\|deliver)` | Returns 403 to external clients; endpoints callable internally (Docker network) only |
| Block `POST /api/v1/inventory/:id/(reserve\|release)` | Returns 403 to external clients; called service-to-service (order-service → product-service) only |

---

## Known Limitations / Deferred Items

| Item | Risk | Mitigation Plan |
|---|---|---|
| Java services (order, product) don't validate JWT | Medium | Blocked at Nginx layer. Full fix: add Spring Security + JWT filter when RBAC system is built. |
| `X-User-Id` header not signed | Medium | Nginx strips unsigned headers in a real API gateway. Mitigated by Docker network isolation in dev. |
| No RBAC system (seller / admin roles) | Medium | ship/deliver blocked at Nginx for now. Full fix: JWT claims with roles + Spring Security method-level security. |
| HSTS not configured | Low | Only applicable once HTTPS/TLS is added (AWS deployment phase, Week 28+). |
| CORS `localhost:3000` hardcoded | Low | Update `Access-Control-Allow-Origin` to production domain at deployment time. |
| `POST /api/v1/payments` unauthenticated | Low by design | Intended for Kafka saga. Protected by Docker internal network isolation. Add internal shared secret header in production. |
