# Coding Conventions

## API Response Envelope

All endpoints return a consistent JSON envelope (defined in `api/openapi.yaml`).

**Success (single resource):**
```json
{
  "success": true,
  "data": { ... }
}
```

**Success (paginated list):**
```json
{
  "success": true,
  "data": [...],
  "meta": {
    "page": 0,
    "size": 20,
    "totalElements": 150,
    "totalPages": 8
  }
}
```

**Error:**
```json
{
  "success": false,
  "error": {
    "code": "INSUFFICIENT_STOCK",
    "message": "Only 3 units available for product 42",
    "timestamp": "2026-02-23T10:30:00Z",
    "path": "/api/v1/inventory/42/reserve",
    "details": { ... }
  }
}
```

### HTTP Status Codes

| Code | Usage |
|---|---|
| 200 | Successful GET, PUT, PATCH |
| 201 | Successful POST (resource created) — includes `Location` header |
| 204 | Successful DELETE (no body) |
| 400 | Validation errors, malformed request body |
| 401 | Missing, invalid, or expired JWT |
| 403 | Valid JWT but insufficient role/permissions |
| 404 | Resource not found |
| 409 | Conflict (duplicate email, insufficient stock, invalid state transition) |
| 422 | Unprocessable entity (business rule violation) |
| 429 | Rate limit exceeded (includes `Retry-After` header) |
| 500 | Internal server error |

### Go Response Helpers (`pkg/response/`)

```go
response.Success(c, data)                 // 200 + envelope
response.Created(c, data)                 // 201 + envelope
response.Error(c, statusCode, code, msg)  // error envelope
response.BadRequest(c, code, msg)         // 400
response.Conflict(c, code, msg)           // 409
response.InternalError(c, msg)            // 500
```

### Validation Error Response

Go services use `github.com/go-playground/validator/v10` on DTOs. Validation errors are mapped to a field→tag map:

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Validation failed",
    "details": {
      "email": "required",
      "password": "min"
    }
  }
}
```

---

## Go Service Conventions

### Project Structure

```
<service>/
├── cmd/server/main.go          # Dependency wiring + server start
├── config/config.go            # Env-based configuration (no config files)
├── internal/
│   ├── handler/                # HTTP handlers (Gin) — thin, delegates to service
│   ├── service/                # Business logic — depends only on repository interfaces
│   ├── repository/             # DB access (interface + GORM impl)
│   ├── model/                  # GORM models
│   ├── dto/                    # Request/response structs with `validate:"..."` tags
│   └── middleware/             # Recovery + structured JSON logger
├── pkg/
│   ├── response/               # Shared response envelope helpers
│   ├── password/               # bcrypt helpers
│   └── jwt/                    # JWT helpers (RS256)
├── Dockerfile                  # Multi-stage production build
├── Dockerfile.dev              # Air hot reload for development
├── go.mod
└── api.txt                     # curl-based API testing reference
```

### Dependency Wiring

Always in `main.go`, following this chain:
```
db → repository → service → handler → router
```

Services depend only on repository **interfaces**, never on `*gorm.DB` directly.

### Context Propagation

Every handler extracts `c.Request.Context()` and passes it through:
```
handler → service → repository → db.WithContext(ctx)
```

### Error Sentinels

Use sentinel errors for business logic (not generic errors):
```go
var (
    ErrDuplicateEmail     = errors.New("email already registered")
    ErrUserNotFound       = errors.New("user not found")
    ErrInvalidCredentials = errors.New("invalid credentials")
    ErrAccountLocked      = errors.New("account is locked")
)
```

### Configuration

Environment variables with fallbacks in `config/config.go`. No config files — set env vars or use `.env` with Docker Compose.

### UUID Primary Keys

All primary keys use UUID v4. GORM models use `gorm:"type:uuid;default:gen_random_uuid()"`.

### Soft Delete

Enabled on `User` model via GORM's `DeletedAt` field. Other models use hard delete.

### AutoMigrate

Called in `main.go` at startup. Drops and recreates tables cleanly in dev if schema drifts.

---

## Java Service Conventions

### Project Structure

```
<service>/
├── src/main/java/com/ecommerce/<service>/
│   ├── controller/             # REST controllers
│   ├── service/                # Business logic
│   ├── repository/             # Spring Data JPA repositories
│   ├── model/                  # JPA entities
│   ├── dto/                    # Request/response DTOs
│   ├── exception/              # Custom exceptions + global handler
│   ├── config/                 # Redis, Kafka, security config
│   └── <Service>Application.java
├── src/main/resources/
│   ├── application.yml
│   └── db/migration/           # Flyway SQL migrations
├── src/test/
├── pom.xml
└── Dockerfile
```

### Database Migrations

Flyway for versioned SQL migrations. Files in `src/main/resources/db/migration/`.

### Validation

`@Valid` + Bean Validation annotations (`@NotNull`, `@Size`, `@Email`, etc.) on DTOs.

### Transactions

`@Transactional` at service layer. Default isolation: `READ_COMMITTED`.

### Lombok

Used for boilerplate reduction: `@Data`, `@Builder`, `@NoArgsConstructor`, `@AllArgsConstructor`.

---

## Naming Conventions

### API Endpoints

- Plural nouns: `/api/v1/products`, `/api/v1/orders`
- Nested resources: `/api/v1/carts/me/items`
- Actions as sub-resources: `/api/v1/orders/{id}/cancel`, `/api/v1/orders/{id}/ship`
- Version prefix: `/api/v1/`

### Database

- Table names: lowercase, plural (`users`, `order_items`)
- Column names: lowercase, snake_case (`created_at`, `user_id`)
- Index names: `idx_<table>_<column>` (e.g., `idx_products_category_id`)
- Foreign keys: `<referenced_table_singular>_id` (e.g., `user_id`, `order_id`)

### Go Code

- Package names: lowercase, single word (`handler`, `service`, `repository`)
- Interfaces: verb-noun pattern (`UserRepository`, `AuthService`)
- Exported functions: PascalCase
- Unexported: camelCase
- Test files: `*_test.go` in the same package

### Java Code

- Package naming: `com.ecommerce.<service>.<layer>`
- Classes: PascalCase (`ProductService`, `OrderController`)
- Spring beans: constructor injection (not field injection)

---

## Git Conventions

### Branch Naming

- Feature branches: `<Service-name>` or `feature/<description>`
- Bug fixes: `fix/<description>`

### Commit Messages

- Prefix with day number during implementation: `Day N: <description>`
- Be descriptive: `Day 8: Add Login and Refresh token tests for auth_handler and auth_service`

---

## Logging

All services emit JSON-structured logs with consistent fields:

```json
{
  "timestamp": "2026-02-23T10:30:00.123Z",
  "level": "INFO",
  "service": "order-service",
  "correlationId": "req-abc-123",
  "userId": "user-uuid",
  "method": "POST",
  "path": "/api/v1/orders",
  "status": 201,
  "latencyMs": 145,
  "message": "Order created successfully"
}
```

- **Correlation ID:** Generated at Nginx, propagated via `X-Correlation-ID` header
- **Log levels:** ERROR (actionable failures), WARN (degraded), INFO (key events), DEBUG (dev only)

---

## Security Conventions

- Never return `password_hash` in API responses (excluded from DTOs)
- Parameterized queries only — no raw SQL concatenation
- JWT validation per-service (defense in depth)
- Internal service calls: Docker network isolation + API key header
- Refresh tokens: SHA-256 hashed before DB storage; raw token returned to client
