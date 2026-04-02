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
