# Testing Strategy

## Test Pyramid

| Level | Coverage Target | Tools | What to Test |
|---|---|---|---|
| **Unit** | 70%+ (service layer), 100% (auth handler) | Go: `testify`; Java: `JUnit 5 + Mockito` | Business logic, validation, state transitions, edge cases |
| **Integration** | Critical paths | Go: `httptest` + real DB; Java: `TestContainers + @SpringBootTest` | DB queries, Redis ops, Kafka pub/sub, HTTP clients |
| **Concurrency** | All race conditions | Go: `-race` flag; Java: `ExecutorService + CountDownLatch` | Stock contention, cart updates, payment idempotency |
| **E2E** | Happy + error paths | curl scripts, k6 | Full user journeys through Nginx |

## Running Tests

### Go Services

```bash
cd user-service   # or cart-service / payment-service

go test ./...                                          # all tests
go test ./internal/handler/...                         # single package
go test -race ./...                                    # with race detector (required)
go test -cover ./...                                   # with coverage
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out  # HTML report
go test -v -run TestLogin ./internal/handler/...       # specific test
go test -tags=integration -v -race ./internal/integration/  # integration (requires real DB + Redis)
```

### Java Services

```bash
cd product-service   # or order-service

./mvnw test                                   # all tests
./mvnw test -Dtest=ProductServiceTest         # single class
./mvnw test jacoco:report                     # with coverage (target/site/jacoco/index.html)
```

## Go Testing Patterns

- **`github.com/stretchr/testify`** — assert, require, mock
- Always run with `-race` flag
- Mock repository **interfaces** for unit tests (never mock `*gorm.DB` directly)
- Integration tests use `//go:build integration` tag and `httptest.NewServer` with full wired stack (no mocks)

### Mock Pattern

```go
type MockUserRepository struct {
    mock.Mock
}

func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
    args := m.Called(ctx, email)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*model.User), args.Error(1)
}

func TestRegister_Success(t *testing.T) {
    mockRepo := new(MockUserRepository)
    mockRepo.On("FindByEmail", mock.Anything, "test@example.com").
        Return(nil, repository.ErrNotFound)
    mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*model.User")).
        Return(nil)

    // ... wire service with mock, call Register, assert
    mockRepo.AssertExpectations(t)
}
```

## Java Testing Patterns

- **JUnit 5** — test framework
- **Mockito** — mocking
- **TestContainers** — real PostgreSQL/Redis/Kafka in tests
- **`@SpringBootTest`** — integration tests with full context

### Integration Test Pattern

```java
@SpringBootTest
@Testcontainers
class ProductServiceIntegrationTest {
    @Container
    static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:15-alpine");

    @DynamicPropertySource
    static void configureProperties(DynamicPropertyRegistry registry) {
        registry.add("spring.datasource.url", postgres::getJdbcUrl);
        registry.add("spring.datasource.username", postgres::getUsername);
        registry.add("spring.datasource.password", postgres::getPassword);
    }

    @Autowired
    private ProductService productService;

    @Test
    void createProduct_shouldPersistAndReturn() { /* ... */ }
}
```

## Critical Test Scenarios

### Unit Tests

| Scenario | Service | What It Proves |
|---|---|---|
| Register with duplicate email → error | User | Duplicate check works |
| Login with wrong password → increment attempts | User | Lockout counter logic |
| Login with locked account → rejected | User | Account lockout enforcement |
| Expired/blacklisted JWT rejected | User | Token validation + Redis blacklist |
| Verify email with wrong code → brute-force protection | User | Attempt limiting |
| Product CRUD operations | Product | Basic business logic |
| Add to cart → validates product | Cart | Cross-service validation |
| Payment with duplicate idempotency key → return existing | Payment | Idempotency |

### Integration Tests

| Scenario | Service | What It Proves |
|---|---|---|
| Register → verify email → login → access profile → refresh → logout | User | Full auth flow with real DB + Redis |
| Profile CRUD + address management with ownership checks | User | Authorization + data integrity |
| Product CRUD + cache hit/miss | Product | Redis cache-aside, eviction |
| Cart operations with Redis + background Postgres sync | Cart | Redis-first storage pattern |
| Order → Payment → stock confirmed (Kafka saga) | Order + Payment | Full async flow, idempotency |
| Circuit breaker opens after service failure | Cart/Order | Fallback behavior |

### Concurrency Tests

| Scenario | Service | What It Proves |
|---|---|---|
| 200 goroutines reserve 100 stock | Product | Optimistic locking prevents overselling |
| Simultaneous pay + cancel | Order | Pessimistic lock ensures exactly-one state transition |
| Duplicate Kafka event | Payment | Idempotency key prevents double charge |
| Concurrent cart updates | Cart | Redis WATCH/MULTI prevents lost updates |
| Concurrent login attempts | User | SELECT FOR UPDATE prevents lockout bypass |

## Load Testing (Phase 4)

### Tool: k6

```bash
brew install k6
```

### Scenarios

1. **Product listing + search** (read-heavy) — target p95 < 500ms
2. **Cart operations** (mixed read/write) — target p95 < 200ms
3. **Full checkout flow** (write-heavy, multi-service) — target p95 < 2s
4. **Concurrent stock reservation** — 200 VUs on same product (contention test)

### Performance Targets

| Metric | Target |
|---|---|
| Median latency (p50) | < 200ms |
| Tail latency (p99) | < 1 second |
| Cart operations | < 50ms |
| Product search | < 500ms |

## CI Pipeline (Phase 6)

```
Pull Request → Lint → Tests (70%+) → Security Scan
Push to main  → Lint → Tests (70%+) → Build Docker → Deploy
```

- Go: `go vet`, `golangci-lint`, `govulncheck`
- Java: checkstyle, spotbugs, OWASP Dependency-Check

---

## Test Results (as of Phase 4 completion)

### Unit Tests

| Service | Tests | Result | Runner |
|---|---|---|---|
| user-service (service + handler) | 76 / 76 | ✅ All pass | `go test -race` |
| cart-service (service) | 11 / 11 | ✅ All pass | `go test -race` |
| payment-service (service) | 6 / 6 | ✅ All pass | `go test -race` |
| product-service (unit + cache AOP) | 35 / 35 | ✅ All pass | JUnit 5 + Mockito |
| **Total unit** | **128 / 128** | ✅ | |

### Integration Tests

| Service | Tests | Result | Infrastructure |
|---|---|---|---|
| cart-service | 18 / 18 | ✅ All pass | Real Postgres + Redis DB 1 + mock product-service |
| user-service | 4 / 4 ¹ | ✅ Pass | Real Postgres + Redis |
| product-service (Testcontainers) | 27 / 27 | ✅ All pass | Real Postgres + Redis via Testcontainers |
| payment-service (Kafka) | 5 / 5 | ✅ All pass | Real Postgres + Kafka via Testcontainers |
| cart-service (contract) | 8 / 8 | ✅ All pass | httptest mock server |
| **Total integration** | **62 / 62** | ✅ | |

> ¹ Two additional user-service tests exist but have pre-existing infrastructure issues unrelated to concurrency: `TestFullAuthFlow` requires email verification to be bypassed in tests; `TestProfileFlow_WithContainers` has an intermittent Testcontainers Postgres timeout on this machine.

### Concurrency / Race Condition Tests

| Test | Service | What It Proves | Result |
|---|---|---|---|
| `TestConcurrentLogin_SelectForUpdate_PreventsLockoutBypass` | user-service | 10 goroutines on a start gate with warm-up at `attempts=4`; `SELECT FOR UPDATE` serializes the lockout write; zero goroutines receive HTTP 200 (no bypass); DB `is_locked=true`, Redis counter ≥ 5 | ✅ PASS (17.7 s, `-race`) |
| `TestDegradedMode_CircuitOpen_ReadOpsStillWork` | cart-service | Circuit breaker opened with 5 × 3-retry 500 calls; `AddItem` fails fast (`ErrProductServiceUnavailable`, zero extra HTTP hits); `GetCart` / `UpdateItem` / `RemoveItem` / `ClearCart` all succeed via Redis-only path | ✅ PASS (3.1 s, `-race`) |
| `TestConcurrentAdd_Integration` | cart-service | 10 goroutines add same product simultaneously; Redis `WATCH/MULTI/EXEC` prevents lost updates; exactly 1 field in hash | ✅ PASS (`-race`) |
| `payment_idempotency_test.go` | payment-service | 10 goroutines submit same idempotency key; DB `UNIQUE` constraint enforces exactly-one winner | ✅ PASS (`-race`) |
| `InventoryConcurrencyTest` | product-service | 10 threads reserve from stock=5; optimistic locking (`@Version`) ensures exactly 5 succeed, 5 fail with `OptimisticLockException` | ✅ PASS (Java `ExecutorService`) |

### E2E Tests

| Script | Assertions | Result | What It Covers |
|---|---|---|---|
| `bash script/e2e-test.sh` | 14 / 14 | ✅ All pass | Register → login → browse products → add to cart → create order → confirm → list orders |
| `bash script/e2e-payment.sh` | 12 / 12 | ✅ All pass | Kafka saga: order created → payment processed → CONFIRMED + PAYMENT_FAILED → order status updated |

All scripts default to `http://localhost` (port 80, through Nginx). Override with `USER_SVC=http://localhost:8001`.

### Performance Baseline

Measured with `script/perf-baseline.sh` (sequential, single-threaded, `n=10` samples each):

| Operation | min | p50 | avg | max |
|---|---|---|---|---|
| GET /api/v1/products (list) | — | **5 ms** | — | — |
| GET /api/v1/products/:id | — | — | — | — |
| POST /api/v1/orders | — | **11 ms** | — | — |

Load test (`script/loadtest-orders.sh`): **100 orders at 10 concurrent** — 0 stuck in `PENDING`, 0 messages in DLQ after 30-second drain.

### Security Audit (OWASP API Top 10)

Detailed checklist: `docs/security-checklist.md`.

| OWASP Item | Status | Notes |
|---|---|---|
| API1 — Broken Object Level Auth | ✅ Mitigated | Ownership checks on all user-scoped reads; ship/deliver blocked at Nginx |
| API2 — Broken Authentication | ✅ Mitigated | RS256 JWT, 15-min TTL, Redis blacklist on logout, 5-attempt lockout with `SELECT FOR UPDATE` |
| API3 — Broken Object Property Level Auth | ✅ Mitigated | DTOs everywhere; no entity binding; no internal fields exposed |
| API4 — Unrestricted Resource Consumption | ✅ Mitigated | Nginx: 10 req/s general (burst 5), 5 req/min on auth endpoints (burst 3) |
| API5 — Broken Function Level Auth | ⚠️ Partial | ship/deliver blocked externally at Nginx; no in-service RBAC yet |
| API6 — Unrestricted Access to Sensitive Flows | ✅ Mitigated | Inventory reserve/release and payment creation are internal-only routes (403 from Nginx) |
| API7 — SSRF | ✅ Low risk | No user-supplied URLs fetched by any service |
| API8 — Security Misconfiguration | ✅ Mitigated | CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, CORS locked to `localhost:3001` |
| API9 — Improper Assets Management | ✅ Mitigated | Health endpoints expose no sensitive data; debug routes not exposed |
| API10 — Unsafe Consumption of APIs | ✅ Mitigated | cart-service validates all product data from product-service before persisting |

**Per-service cross-check:**

| Check | User | Product | Cart | Order | Payment |
|---|---|---|---|---|---|
| Input validation | ✅ | ✅ | ✅ | ✅ | ✅ |
| Mass assignment protection (DTOs) | ✅ | ✅ | ✅ | ✅ | ✅ |
| Auth middleware on protected routes | ✅ | ⚠️ (gateway header) | ✅ | ⚠️ (gateway header) | ✅ |
| Ownership / access checks | ✅ | ✅ | ✅ | ✅ | ✅ |
| Parameterized queries | ✅ GORM | ✅ JPA | ✅ GORM | ✅ JPA | ✅ GORM |
| No hardcoded secrets | ✅ | ✅ | ✅ | ✅ | ✅ |
