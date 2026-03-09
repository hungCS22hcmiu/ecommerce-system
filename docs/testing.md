# Testing Strategy

## Test Pyramid

| Level | Coverage Target | Tools | What to Test |
|---|---|---|---|
| **Unit** | 70%+ (service layer), 100% (auth handler) | Go: `testify`; Java: `JUnit 5 + Mockito` | Business logic, validation, state transitions, edge cases |
| **Integration** | Critical paths | Go: `testcontainers-go`; Java: `TestContainers + @SpringBootTest` | DB queries, Redis ops, Kafka pub/sub, HTTP clients |
| **Concurrency** | All race conditions | Go: `-race` flag; Java: `ExecutorService + CountDownLatch` | Stock contention, cart updates, payment idempotency |
| **E2E** | Happy + error paths | Postman/Newman, k6 | Full user journeys through Nginx |

## Running Tests

### Go Services

```bash
cd user-service   # or cart-service / payment-service

# Run all tests
go test ./...

# Single package
go test ./internal/handler/...
go test ./internal/service/...

# With race detector (required for concurrency code)
go test -race ./...

# With coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Verbose output
go test -v ./...

# Run specific test
go test -v -run TestLogin ./internal/handler/...

# Benchmarks
go test -bench=. -benchmem ./...
```

### Java Services

```bash
cd product-service   # or order-service

# Run all tests
./mvnw test

# Single test class
./mvnw test -Dtest=ProductServiceApplicationTests

# With coverage report
./mvnw test jacoco:report
# Report at target/site/jacoco/index.html
```

## Go Testing Stack

- **`github.com/stretchr/testify`** — assert, require, mock
- Always run with `-race` flag
- Mock interfaces for unit tests (never mock `*gorm.DB` directly)

### Mock Pattern

```go
// Define mock in test file
type MockUserRepository struct {
    mock.Mock
}

func (m *MockUserRepository) Create(ctx context.Context, user *model.User) error {
    args := m.Called(ctx, user)
    return args.Error(0)
}

func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
    args := m.Called(ctx, email)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*model.User), args.Error(1)
}

// Use in test
func TestRegister_Success(t *testing.T) {
    mockRepo := new(MockUserRepository)
    service := NewAuthServiceWithRepo(mockRepo)

    mockRepo.On("FindByEmail", mock.Anything, "test@example.com").
        Return(nil, repository.ErrNotFound)
    mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*model.User")).
        Return(nil)

    user, err := service.Register(context.Background(), dto.RegisterRequest{
        Email:    "test@example.com",
        Password: "SecurePass123!",
    })

    require.NoError(t, err)
    assert.Equal(t, "test@example.com", user.Email)
    mockRepo.AssertExpectations(t)
}
```

## Java Testing Stack

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
    void createProduct_shouldPersistAndReturn() {
        // ...
    }
}
```

## Critical Test Scenarios

### Unit Tests

| Scenario | Service | What It Proves |
|---|---|---|
| Register with duplicate email → error | User | Duplicate check works |
| Login with wrong password → increment attempts | User | Lockout counter logic |
| Login with locked account → rejected | User | Account lockout enforcement |
| Expired JWT rejected | User | Token validation |
| Blacklisted JWT rejected | User | Redis blacklist check |
| Product CRUD operations | Product | Basic business logic |
| Add to cart → validates product | Cart | Cross-service validation |
| Payment with duplicate idempotency key → return existing | Payment | Idempotency |

### Integration Tests

| Scenario | Type | What It Proves |
|---|---|---|
| Register → Login → Access Profile | Integration | JWT auth flow, Redis session, bcrypt |
| Product CRUD + cache hit / cache miss | Integration | Redis cache-aside, eviction |
| Add to cart → price changes → checkout detects stale price | Integration | Price snapshot vs re-validation |
| Order → Payment success → stock confirmed | E2E | Full Kafka saga, idempotency |
| Order → Payment failure → stock released | E2E | Compensation logic |
| Circuit breaker opens after Product Service failure | Integration | Fallback behavior |
| Rate limiting (burst 150 requests) | Integration | Nginx rejects excess with 429 |

### Concurrency Tests

| Scenario | Service | What It Proves |
|---|---|---|
| **200 goroutines reserve 100 stock** | Product | Optimistic locking prevents overselling |
| **Simultaneous pay + cancel** | Order | Pessimistic lock ensures exactly-one state transition |
| **Duplicate Kafka event** | Payment | Idempotency key prevents double charge |
| **Concurrent cart updates** | Cart | Redis WATCH/MULTI prevents lost updates |
| **Concurrent login attempts** | User | SELECT FOR UPDATE prevents lockout bypass |

#### Go Concurrency Test Example

```go
func TestConcurrentStockReservation(t *testing.T) {
    product := createProductWithStock(100)

    var wg sync.WaitGroup
    var successCount int64
    var failCount int64

    for i := 0; i < 200; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            err := inventoryService.ReserveStock(product.ID, 1)
            if err == nil {
                atomic.AddInt64(&successCount, 1)
            } else {
                atomic.AddInt64(&failCount, 1)
            }
        }()
    }

    wg.Wait()
    assert.Equal(t, int64(100), successCount) // Exactly 100 succeed
    assert.Equal(t, int64(100), failCount)    // Exactly 100 fail
}
```

#### Java Concurrency Test Example

```java
@Test
void testConcurrentStockReservation() throws Exception {
    Product product = createProductWithStock(100);

    ExecutorService executor = Executors.newFixedThreadPool(50);
    CountDownLatch startLatch = new CountDownLatch(1);
    AtomicInteger successCount = new AtomicInteger(0);

    List<Future<?>> futures = new ArrayList<>();
    for (int i = 0; i < 200; i++) {
        futures.add(executor.submit(() -> {
            startLatch.await(); // All threads start simultaneously
            try {
                inventoryService.reserveStock(product.getId(), 1, UUID.randomUUID());
                successCount.incrementAndGet();
            } catch (Exception e) { /* expected */ }
            return null;
        }));
    }

    startLatch.countDown(); // Release all threads
    futures.forEach(f -> { try { f.get(); } catch (Exception e) {} });

    assertEquals(100, successCount.get());
    assertEquals(0, productRepository.findById(product.getId()).getAvailableStock());
}
```

## Load Testing

### Tool: k6

```bash
brew install k6
```

### Test Profiles

| Profile | Ramp Up | Sustain | Ramp Down |
|---|---|---|---|
| Smoke | 1 VU | 30s | — |
| Load | 100 VUs over 2 min | 5 min | 2 min |
| Stress | 200 VUs over 2 min | 10 min | 5 min |

### Scenarios

1. **Product listing + search** (read-heavy) — expect p95 < 500ms
2. **Add to cart + update** (mixed read/write) — expect p95 < 200ms
3. **Full checkout flow** (write-heavy, multi-service) — expect p95 < 2s
4. **Concurrent stock reservation** — 200 VUs reserving same product (contention test)

### Performance Targets

| Metric | Target |
|---|---|
| Median latency (p50) | < 200ms |
| Tail latency (p99) | < 1 second |
| Concurrent users | 10,000 |
| Kafka throughput | 1,000 events/second |
| Cart operations | < 50ms |
| Product search | < 500ms |

## Test Data

- Seed scripts in `script/` generate: 100 products, 50 users, 10 categories
- Idempotent — safe to run multiple times (`INSERT ... ON CONFLICT DO NOTHING`)
- Separate seed for dev (small) and load-test (10K products, 1K users)

## CI Pipeline

```
Pull Request → Lint → Tests (70%+) → Security Scan
Push to main  → Lint → Tests (70%+) → Build Docker → Deploy
```

- Go: `go vet`, `golangci-lint`, `govulncheck`
- Java: checkstyle, spotbugs, OWASP Dependency-Check
- No critical CVEs allowed in production builds
