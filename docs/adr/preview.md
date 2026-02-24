# High-Throughput E-Commerce System — Implementation Roadmap

**Duration:** 10 weeks · 6 days/week · 3 hours/day  
**Total Hours:** 180 hours  
**Goal:** Ship a fully functional, containerized e-commerce platform with 5 microservices, Kafka event-driven saga, and a minimal frontend — ready to demo in an internship interview.

**v3.0 Enhancements:** Added Go concurrency deep-dives, Java concurrency patterns, race condition testing, performance profiling (pprof/JFR), and cloud deployment (AWS & GCP) days.

---

## Budget Breakdown

| Phase | Weeks | Hours | What |
|---|---|---|---|
| Phase 1: Design | Week 1 | 18h | Architecture, schemas, API contracts, project scaffolding |
| Phase 2: Core Services | Weeks 2–6 | 90h | 5 microservices (User, Product, Cart, Order, Payment) |
| Phase 3: Integration | Week 7 | 18h | Docker Compose, Nginx, Kafka wiring, E2E flow, **cloud deployment** |
| Phase 4: Frontend | Week 8 | 18h | React + Vite — Login, Products, Cart, Checkout |
| Phase 5: Quality | Week 9 | 18h | Testing, security hardening, **performance profiling**, concurrency testing |
| Phase 6: Ship | Week 10 | 18h | Documentation, CI/CD, demo prep, README polish |

---

## Prerequisites (Before Week 1)

Make sure these are installed on your machine:

- [ ] **Golang** 1.21+ — `brew install go`
- [ ] **Java** 17 or 21 — `brew install openjdk@21`
- [ ] **Maven** 3.9+ — `brew install maven`
- [ ] **Docker Desktop** — `brew install --cask docker`
- [ ] **Node.js** 20+ — `brew install node` (for frontend in Week 8)
- [ ] **Postman** or **Bruno** — for API testing
- [ ] **VS Code** or **IntelliJ IDEA** — IDE setup
- [ ] **Git** configured with SSH key on GitHub
- [ ] **k6** — `brew install k6` (for load testing in Week 9)

---

## Phase 1: Design & Architecture

### Week 1 — System Design, Schemas & Project Scaffolding

**Goal:** Finalize all design decisions on paper before writing code. Scaffold empty projects so every service compiles and runs "Hello World."

---

#### Day 1 (3h) — Architecture Deep Dive & Learning

**Learn:**
- Read about microservices bounded contexts (Martin Fowler's article)
- Understand Database-per-Service pattern — why and how
- Study the proposal.md architecture diagram until you can redraw it from memory
- Read about REST API design conventions (JSON envelope, HTTP status codes, pagination)
- **Understand why Go for I/O-bound services and Java for transaction-heavy services** (proposal §3.2, §3.3)

**Do:**
- [ ] Draw the full system architecture on paper/whiteboard (all 5 services, Nginx, Kafka, Redis, PostgreSQL)
- [ ] List every inter-service call (who calls who, sync vs async)
- [ ] List every Kafka topic and who produces/consumes
- [ ] Write down the Order saga flow step by step
- [ ] **List every race condition from proposal §10.1 and understand the solution for each**

**Deliverable:** Hand-drawn or diagrammed architecture you fully understand. Race condition inventory documented.

---

#### Day 2 (3h) — Database Design & API Contracts

**Learn:**
- PostgreSQL UUID, JSONB, and indexing types (B-TREE, GIN, UNIQUE)
- Optimistic locking with `@Version` column — how JPA uses it
- **Transaction isolation levels: READ_COMMITTED vs REPEATABLE_READ vs SERIALIZABLE**
- OpenAPI 3.0 basics (paths, schemas, components)

**Do:**
- [ ] Write the full SQL schema for all 5 databases (CREATE TABLE statements)
- [ ] Define all indexes from the proposal (including partial indexes for active products)
- [ ] Create `script/init-databases.sql` — creates the 5 logical databases
- [ ] Start `api/openapi.yaml` — define the User Service endpoints first (register, login, profile)
- [ ] **Write down which locking strategy each service uses and why** (from proposal §10, Appendix C)

**Deliverable:** `script/init-databases.sql` and partial `api/openapi.yaml`. Locking strategy notes.

---

#### Day 3 (3h) — Go Project Scaffolding (User, Cart, Payment)

**Learn:**
- Go project layout convention (`cmd/`, `internal/`, `pkg/`)
- Go modules (`go mod init`, `go mod tidy`)
- Gin framework basics: router, handlers, middleware, context
- GORM basics: model definition, AutoMigrate, CRUD
- **Go concurrency primitives overview: goroutines, channels, sync.Mutex, sync.WaitGroup, errgroup, context**

**Do:**
- [ ] Scaffold `user-service/` with full directory structure:
  ```
  cmd/server/main.go
  internal/handler/ service/ repository/ model/ dto/ middleware/
  pkg/jwt/ password/ response/
  config/config.go
  ```
- [ ] Scaffold `cart-service/` and `payment-service/` with same structure
- [ ] Each service: `go mod init`, add Gin dependency, create a basic `main.go` with health check endpoint
- [ ] **Add `context.Context` to every handler signature from the start** — good practice for timeout propagation
- [ ] Verify: `go run cmd/server/main.go` starts and `GET /health/live` returns 200 for all three

**Deliverable:** 3 Go services that compile and respond to health checks.

---

#### Day 4 (3h) — Java Project Scaffolding (Product, Order)

**Learn:**
- Spring Boot 3.x project structure (controller, service, repository layers)
- Spring Initializr — selecting dependencies (Web, JPA, Validation, Redis, Kafka, Flyway, Lombok)
- Spring Data JPA repository interface pattern
- `application.yml` configuration structure
- **Spring @Transactional basics: propagation, isolation, rollback rules**

**Do:**
- [ ] Generate `product-service/` via Spring Initializr (or manually):
  - Dependencies: Spring Web, Spring Data JPA, PostgreSQL Driver, Spring Cache (Redis), Validation, Lombok, Flyway, Resilience4j
  - Package structure: `controller/`, `service/`, `repository/`, `model/`, `dto/`, `exception/`, `config/`
- [ ] Generate `order-service/` with same structure + Spring Kafka
- [ ] Add health check endpoint (`GET /health/live` returning 200) to both
- [ ] **Configure @EnableAsync and create a custom ThreadPoolTaskExecutor bean** (will need for notifications)
- [ ] Verify: `mvn spring-boot:run` starts both services (even if DB not connected yet)

**Deliverable:** 2 Java services that compile and respond to health checks.

---

#### Day 5 (3h) — Docker Compose & Infrastructure

**Learn:**
- Docker Compose v2 syntax: services, volumes, networks, depends_on, healthcheck
- PostgreSQL Docker image: environment variables, init scripts
- Redis Docker image: persistence options
- Kafka + Zookeeper Docker setup (Confluent images)

**Do:**
- [ ] Write `docker-compose.yml` with infrastructure only (no app services yet):
  - `postgres:15-alpine` on port 5432, with `init-databases.sql` mounted via volume
  - `redis:7-alpine` on port 6379
  - `zookeeper` + `kafka` (Confluent images) on ports 2181/9092
- [ ] Write `script/init-databases.sql`:
  ```sql
  CREATE DATABASE ecommerce_users;
  CREATE DATABASE ecommerce_products;
  CREATE DATABASE ecommerce_carts;
  CREATE DATABASE ecommerce_orders;
  CREATE DATABASE ecommerce_payments;
  ```
- [ ] `docker-compose up -d` and verify all infrastructure starts
- [ ] Test: connect to PostgreSQL with `psql`, verify 5 databases exist
- [ ] Test: `redis-cli ping` returns PONG

**Deliverable:** Working `docker-compose.yml` with all infrastructure. All 5 databases created.

---

#### Day 6 (3h) — Config, Environment & Seed Data

**Learn:**
- 12-Factor App: configuration via environment variables
- Go: reading env vars with `os.Getenv` and config structs
- Java: Spring Boot externalized configuration (`application.yml`, `@Value`, profiles)

**Do:**
- [ ] Create a shared `.env.example` file with all config variables:
  ```
  DB_HOST=localhost
  DB_PORT=5432
  REDIS_HOST=localhost
  REDIS_PORT=6379
  KAFKA_BROKERS=localhost:9092
  JWT_SECRET=...
  ```
- [ ] Implement config loading in all Go services (`config/config.go` — reads env vars into a struct)
- [ ] Configure `application.yml` for both Java services (DB, Redis, Kafka connection strings)
- [ ] Write `script/seed-data.sql`: 10 categories, 50 products (with stock), 5 test users (hashed passwords)
- [ ] Update `docker-compose.yml` to mount seed script
- [ ] Verify: `docker-compose up`, seed data loads, connect to each DB and query tables

**Deliverable:** All services can read configuration. Seed data populates dev database.

---

### Week 1 Checkpoint ✅

By end of Week 1 you should have:
- [ ] Full understanding of the architecture (can explain it in an interview)
- [ ] **Can explain every race condition and its solution**
- [ ] 5 empty but compiling/running services with health checks
- [ ] Docker Compose with PostgreSQL (5 DBs), Redis, Kafka all running
- [ ] Seed data loaded
- [ ] API contracts started in OpenAPI
- [ ] Git: clean commit history, one commit per day minimum

---

## Phase 2: Core Services

### Week 2 — User Service (Golang)

**Goal:** Complete authentication system with JWT, bcrypt, Redis session caching, refresh tokens, **and Go concurrency fundamentals**.

---

#### Day 7 (3h) — User Model, Registration & Password Hashing

**Learn:**
- bcrypt: how it works, cost factor, why it's better than SHA256 for passwords
- GORM model definition: tags, AutoMigrate, UUID primary keys
- Input validation in Go: struct tags, custom validators
- Standard API response envelope pattern

**Do:**
- [ ] Define `model/user.go`: User struct with GORM tags (UUID PK, unique email, password_hash, role, is_locked, failed_login_attempts, timestamps)
- [ ] Define `model/user_profile.go` and `model/user_address.go`
- [ ] Implement `pkg/password/password.go`: `Hash(password) → hash`, `Compare(hash, password) → bool` using bcrypt cost 12
- [ ] Implement `pkg/response/response.go`: standard JSON response helpers (`Success()`, `Error()`, `Paginated()`)
- [ ] Implement `dto/register_request.go`: email, password, first_name, last_name with validation tags
- [ ] Implement `repository/user_repository.go`: `Create(user)`, `FindByEmail(email)`, `FindByID(id)`
- [ ] Implement `service/auth_service.go`: `Register(req)` — validate input, check duplicate email, hash password, save user + profile
- [ ] Implement `handler/auth_handler.go`: `POST /api/v1/auth/register`
- [ ] Wire everything in `main.go`: DB connection → AutoMigrate → router → handlers
- [ ] **Pass `context.Context` through handler → service → repository** (good habit from day 1)
- [ ] Test with Postman: register a user, verify in database

**Deliverable:** Working user registration with hashed passwords stored in PostgreSQL.

---

#### Day 8 (3h) — Login, JWT Generation & Refresh Tokens

**Learn:**
- JWT structure: header.payload.signature
- RS256 vs HS256 — when to use which (we use RS256 for production-grade)
- Refresh token rotation pattern
- Go `golang-jwt/jwt/v5` library usage

**Do:**
- [ ] Generate RSA key pair for JWT signing: `openssl genrsa -out private.pem 2048` and extract public key
- [ ] Implement `pkg/jwt/jwt.go`:
  - `GenerateAccessToken(userId, email, role) → token string` (15 min TTL, RS256, includes `jti`)
  - `GenerateRefreshToken() → token string` (random 64-byte hex)
  - `ValidateToken(tokenString) → claims, error`
- [ ] Define `model/auth_token.go`: stores hashed refresh tokens in DB
- [ ] Implement `service/auth_service.go`: `Login(email, password)`:
  1. Find user by email → 401 if not found
  2. Check `is_locked` → 403 if locked
  3. Compare password → increment `failed_login_attempts` on failure, lock after 5
  4. On success → reset attempts, generate access + refresh token, save refresh token hash
  5. Return both tokens
- [ ] **Use `SELECT ... FOR UPDATE` (GORM `.Clauses(clause.Locking{Strength: "UPDATE"})`) on login to prevent login attempt race condition** (see proposal §4.2)
- [ ] Implement `handler/auth_handler.go`: `POST /api/v1/auth/login`
- [ ] Implement `POST /api/v1/auth/refresh`: validate refresh token, issue new access token
- [ ] Test: register → login → get tokens → use access token → refresh → get new access token

**Deliverable:** Full JWT auth flow working. Access + refresh tokens generated and validated. Login attempt counter is race-condition safe.

---

#### Day 9 (3h) — JWT Middleware, Redis Blacklist & Logout

**Learn:**
- Gin middleware pattern: `c.Next()`, `c.Abort()`, `c.Set()`/`c.Get()`
- Redis SET with TTL for token blacklisting
- Why blacklisting JWTs by `jti` instead of storing all valid tokens
- **Redis atomic operations: INCR, SET NX, EXPIRE** — why they're safe for concurrent goroutines

**Do:**
- [ ] Connect to Redis in `main.go` using `go-redis/redis/v9`
- [ ] Implement `middleware/auth_middleware.go`:
  1. Extract `Authorization: Bearer <token>` header
  2. Validate JWT signature and expiry
  3. Check Redis blacklist: `GET blacklist:{jti}` → 401 if found
  4. Inject `userId` and `role` into Gin context
- [ ] Implement `service/auth_service.go`: `Logout(token)`:
  1. Parse JWT to extract `jti` and remaining TTL
  2. `SET blacklist:{jti} "revoked" EX <remaining_seconds>` in Redis
  3. Revoke refresh token in DB
- [ ] Implement `handler/auth_handler.go`: `POST /api/v1/auth/logout`
- [ ] Implement Redis session caching: after login, cache user profile at `session:{userId}` (30 min TTL)
- [ ] **Implement atomic login attempt counter using Redis INCR** (proposal §8.4):
  ```go
  count, _ := redis.Incr(ctx, "login_attempts:"+email).Result()
  if count == 1 { redis.Expire(ctx, key, 15*time.Minute) }
  ```
- [ ] Test full flow: register → login → access protected route → logout → same token rejected

**Deliverable:** JWT middleware protecting routes. Redis-backed blacklist and session cache working. Atomic login counter.

---

#### Day 10 (3h) — User Profile & Address CRUD

**Learn:**
- Go struct embedding for request/response separation
- GORM preloading associations (`Preload("Profile")`, `Preload("Addresses")`)
- PATCH vs PUT semantics

**Do:**
- [ ] Implement `handler/user_handler.go`:
  - `GET /api/v1/users/profile` — return user with profile and addresses (preloaded)
  - `PUT /api/v1/users/profile` — update profile fields (first_name, last_name, phone)
- [ ] Implement `dto/update_profile_request.go` with validation
- [ ] Implement address endpoints:
  - `POST /api/v1/users/addresses` — add new address
  - `PUT /api/v1/users/addresses/{id}` — update address
  - `DELETE /api/v1/users/addresses/{id}` — remove address
  - `PUT /api/v1/users/addresses/{id}/default` — set as default shipping address
- [ ] On profile update: invalidate Redis session cache (write-through: update cache + DB)
- [ ] Implement `GET /api/v1/users/{id}` (internal, no auth) — for service-to-service lookups
- [ ] Test all endpoints with Postman

**Deliverable:** Complete User Service with auth + profile + addresses.

---

#### Day 11 (3h) — User Service Error Handling, Logging & Testing

**Learn:**
- Centralized error handling in Gin (recovery middleware, custom error types)
- Structured JSON logging in Go (`log/slog` or `zerolog`)
- Go table-driven tests and `testify` assertions
- Mocking interfaces in Go (`testify/mock`)
- **`go test -race` flag — how the Go race detector works**

**Do:**
- [ ] Implement `middleware/recovery.go`: catch panics, return 500 with structured error
- [ ] Implement `middleware/logging.go`: log every request with method, path, status, latency, correlation ID
- [ ] Define custom error types: `ErrUserNotFound`, `ErrDuplicateEmail`, `ErrInvalidCredentials`, `ErrAccountLocked`
- [ ] Map custom errors to HTTP status codes in handlers
- [ ] Write unit tests for `service/auth_service.go`:
  - Test: successful registration
  - Test: duplicate email returns 409
  - Test: login with wrong password increments failure count
  - Test: account locks after 5 failures
  - Test: successful login resets failure count
- [ ] Write unit tests for `pkg/jwt/jwt.go`:
  - Test: generate and validate token
  - Test: expired token rejected
  - Test: tampered token rejected
- [ ] Write unit tests for `pkg/password/password.go`
- [ ] **Run tests with race detector: `go test -race ./... -cover` → aim for 70%+ coverage**

**Deliverable:** Robust error handling, structured logging, 70%+ test coverage. Race detector clean.

---

#### Day 12 (3h) — User Service Dockerfile & Integration Test

**Learn:**
- Multi-stage Docker builds for Go (builder → alpine)
- `testcontainers-go` for spinning up PostgreSQL + Redis in tests
- Integration testing: test the full HTTP → service → DB flow
- **Go graceful shutdown pattern: `os.Signal` + `context.Done()`** (see proposal §12.3)

**Do:**
- [ ] Write `user-service/Dockerfile` (multi-stage: `golang:1.21-alpine` → `alpine:3.19`)
- [ ] Add user-service to `docker-compose.yml`:
  - `build: ./user-service`
  - Port mapping: 8001:8001
  - Environment variables from `.env`
  - Depends on: postgres, redis
  - Health check: `GET /health/ready`
- [ ] **Implement graceful shutdown in `main.go`** (signal handling + context cancellation)
- [ ] `docker-compose up --build user-service` — verify it starts and connects to DB/Redis
- [ ] Write 1 integration test: spin up test containers, run the full register → login → profile → logout flow
- [ ] Git: commit and push — clean PR-ready state

**Deliverable:** User Service fully containerized and tested end-to-end. Graceful shutdown implemented.

---

### Week 2 Checkpoint ✅

- [ ] User registration with bcrypt password hashing
- [ ] JWT login with access token (15 min) + refresh token (7 days)
- [ ] Redis token blacklist (logout)
- [ ] Redis session caching
- [ ] Account lockout after 5 failed logins (race-condition safe with SELECT FOR UPDATE)
- [ ] Profile & address CRUD
- [ ] Structured logging & error handling
- [ ] 70%+ unit test coverage, `go test -race` clean
- [ ] Graceful shutdown
- [ ] Dockerized and running in Docker Compose

---

### Week 3 — Product Service (Java/Spring Boot) — Includes Inventory

**Goal:** Product catalog with CRUD, search, categories, and stock management with **optimistic locking, @Transactional isolation levels, and retry on conflict**.

---

#### Day 13 (3h) — JPA Entities, Flyway Migrations & Basic CRUD

**Learn:**
- JPA entity mapping: `@Entity`, `@Id`, `@GeneratedValue`, `@Column`, `@ManyToOne`
- Flyway versioned migrations (`V1__create_products_table.sql`)
- Spring Data JPA repository interfaces (derived query methods)
- Lombok: `@Data`, `@Builder`, `@NoArgsConstructor`, `@AllArgsConstructor`
- **`@Version` annotation — how JPA uses it for optimistic locking under the hood**

**Do:**
- [ ] Define JPA entities: `Product`, `Category`, `ProductImage`, `StockMovement`
  - Product: include `stockQuantity`, `stockReserved`, **`@Version` for optimistic lock** — understand that JPA auto-adds `WHERE version = ?` to UPDATE
  - Category: self-referencing `parentId` for hierarchy
- [ ] Write Flyway migration `V1__create_schema.sql` with all tables, indexes, and constraints
- [ ] Define `ProductRepository extends JpaRepository<Product, Long>` with derived queries:
  - `findByCategoryId(Long categoryId, Pageable pageable)`
  - `findByStatus(ProductStatus status, Pageable pageable)`
- [ ] Implement `ProductService.createProduct(dto)`, `getProduct(id)`, `updateProduct(id, dto)`
- [ ] Implement `ProductController` with `POST`, `GET /{id}`, `PUT /{id}`, `DELETE /{id}` (soft delete)
- [ ] Configure `application.yml`: datasource, flyway, JPA settings, **hibernate batch settings**
- [ ] Test: start service, create a product, retrieve it

**Deliverable:** Product CRUD working with Flyway-managed schema. @Version column in place.

---

#### Day 14 (3h) — Pagination, Filtering, Full-Text Search & Categories

**Learn:**
- Spring Data `Pageable`, `Page<T>`, `Sort`
- JPA Specification pattern for dynamic filtering
- PostgreSQL full-text search with `to_tsvector` and `to_tsquery`
- Category tree building (recursive query or application-level)

**Do:**
- [ ] Implement `GET /api/v1/products` with pagination:
  - Query params: `page`, `size`, `sort`, `category`, `minPrice`, `maxPrice`, `status`
  - Return paginated response with `meta` (page, size, totalElements, totalPages)
- [ ] Implement JPA Specification for dynamic filtering:
  ```java
  public class ProductSpecification {
      public static Specification<Product> withFilters(ProductFilterDto filter) { ... }
  }
  ```
- [ ] Implement `GET /api/v1/products/search?q={query}`:
  - Native query: `SELECT * FROM products WHERE to_tsvector('english', name || ' ' || description) @@ plainto_tsquery(:query)`
  - Ordered by relevance rank
- [ ] Implement category endpoints:
  - `GET /api/v1/categories` — return tree structure (nested JSON)
  - `GET /api/v1/categories/{id}/products` — paginated products in category
- [ ] Test: search for products, filter by price range, paginate results

**Deliverable:** Full product listing with search, filter, pagination, and categories.

---

#### Day 15 (3h) — Inventory: Stock Reserve/Release with Optimistic Locking

**Learn:**
- **`@Version` annotation deep-dive**: how JPA detects concurrent modification
- **`OptimisticLockException` lifecycle**: when it's thrown, how to catch and retry
- Resilience4j `@Retry` annotation — declarative retry on specific exceptions
- **Transaction isolation levels: when to use READ_COMMITTED vs SERIALIZABLE**
- **Pessimistic locking alternative**: `@Lock(LockModeType.PESSIMISTIC_WRITE)` and `SELECT ... FOR UPDATE`

**Do:**
- [ ] Implement `InventoryService`:
  - `getAvailableStock(productId)` → returns `stockQuantity - stockReserved`
  - `reserveStock(productId, quantity, referenceId)`:
    1. Load product with `@Version`
    2. Check available stock ≥ requested quantity
    3. Increment `stockReserved`
    4. Save (triggers optimistic lock check — JPA adds `WHERE version = ?`)
    5. Log `StockMovement` (type=RESERVE)
  - `releaseStock(productId, quantity, referenceId)`:
    1. Decrement `stockReserved`
    2. Log `StockMovement` (type=RELEASE)
  - `confirmStock(productId, quantity)`:
    1. Decrement both `stockQuantity` and `stockReserved`
    2. Log `StockMovement` (type=OUT)
- [ ] **Add `@Retry(name = "stockRetry", maxAttempts = 3, retryExceptions = OptimisticLockException.class)`** on `reserveStock`
- [ ] **Implement alternative pessimistic lock method** for flash-sale scenarios:
  ```java
  @Query("SELECT p FROM Product p WHERE p.id = :id")
  @Lock(LockModeType.PESSIMISTIC_WRITE)
  Product findByIdWithPessimisticLock(@Param("id") Long id);
  ```
- [ ] Implement inventory controller endpoints
- [ ] **Write concurrent reservation test** (from proposal §10.5):
  - 200 threads reserving 1 unit each, product has 100 stock
  - Assert: exactly 100 succeed, exactly 100 get InsufficientStockException

**Deliverable:** Concurrency-safe stock management with both optimistic and pessimistic locking strategies. Concurrent test passes.

---

#### Day 16 (3h) — Redis Caching & Cache Invalidation

**Learn:**
- Spring Cache abstraction: `@Cacheable`, `@CacheEvict`, `@CachePut`
- Redis serialization (JSON vs Java serialization)
- Cache-aside pattern implementation
- Cache warming strategies
- **Cache stampede problem and singleflight pattern** (proposal §10.2)

**Do:**
- [ ] Add Redis dependency and configure `RedisCacheManager` in `config/RedisConfig.java`
- [ ] Add `@Cacheable("products")` on `getProduct(id)` with 10-min TTL
- [ ] Add `@CacheEvict("products")` on `updateProduct(id)` and `deleteProduct(id)`
- [ ] Add `@CacheEvict` on all stock operations (reserve/release/confirm change stock data)
- [ ] Cache category list: `@Cacheable("categories")` with 30-min TTL
- [ ] Implement cache warming: `@PostConstruct` method that pre-loads top-50 products **using @Async** (doesn't block startup)
- [ ] Test: GET product → check Redis for cache hit → update product → verify cache evicted → GET again → cache repopulated

**Deliverable:** Redis caching reducing DB load by ~80% on product reads.

---

#### Day 17 (3h) — Product Service Error Handling, Validation & Testing

**Learn:**
- Spring `@ControllerAdvice` and `@ExceptionHandler` for global error handling
- Bean Validation: `@Valid`, `@NotNull`, `@Size`, `@DecimalMin`, custom validators
- JUnit 5 + Mockito: `@Mock`, `@InjectMocks`, `when().thenReturn()`, `verify()`
- **Testing @Transactional behavior: how to verify rollback on exception**

**Do:**
- [ ] Implement `GlobalExceptionHandler`:
  - `ProductNotFoundException` → 404
  - `InsufficientStockException` → 409 (with available stock in response body)
  - **`OptimisticLockException` → 409 (concurrent modification — suggest retry)**
  - `MethodArgumentNotValidException` → 400 (field-level errors)
  - Generic `Exception` → 500
- [ ] Add validation annotations to all DTOs:
  - `CreateProductRequest`: name (required, 3-200 chars), price (> 0), stock (≥ 0)
  - `StockReserveRequest`: quantity (> 0)
- [ ] Write unit tests for `ProductService`:
  - CRUD operations (happy path + edge cases)
  - Product not found → exception
- [ ] Write unit tests for `InventoryService`:
  - Reserve stock — sufficient stock → success
  - Reserve stock — insufficient stock → `InsufficientStockException`
  - **Reserve stock — concurrent conflict → `OptimisticLockException` caught by retry**
  - Release stock → stock restored
  - Confirm stock → both quantity and reserved decremented
- [ ] Run: `mvn test` → aim for 70%+ coverage on service layer

**Deliverable:** Robust error handling, validated inputs, 70%+ test coverage.

---

#### Day 18 (3h) — Product Service Dockerfile & Integration Tests

**Learn:**
- Multi-stage Docker build for Spring Boot (Maven build → JRE runtime)
- TestContainers for PostgreSQL + Redis in integration tests
- `@SpringBootTest` with `@AutoConfigureTestDatabase`
- **JVM container awareness flags: `-XX:+UseContainerSupport`, `-XX:MaxRAMPercentage`**

**Do:**
- [ ] Write `product-service/Dockerfile` (multi-stage: `maven:3.9-eclipse-temurin-21` → `eclipse-temurin:21-jre-alpine`)
  - **Add JVM flags: `-XX:+UseContainerSupport -XX:MaxRAMPercentage=75.0 -XX:+UseZGC`**
- [ ] Add product-service to `docker-compose.yml` (port 8081, depends on postgres/redis)
- [ ] `docker-compose up --build product-service` — verify it starts, runs Flyway migrations, connects to Redis
- [ ] Write integration test with TestContainers:
  - Create product → GET returns it → Update → cache invalidated → Search finds it
  - **Reserve stock with concurrent threads → verify optimistic locking works in real DB**
- [ ] Git: commit and push

**Deliverable:** Product Service containerized, integration-tested, concurrent stock test passing.

---

### Week 3 Checkpoint ✅

- [ ] Product CRUD with soft delete
- [ ] Pagination, filtering, full-text search
- [ ] Category hierarchy
- [ ] **Stock reserve/release/confirm with optimistic locking & retry — concurrent test passes**
- [ ] **Pessimistic locking alternative for flash-sale scenarios**
- [ ] Stock movement audit trail
- [ ] Redis caching with eviction
- [ ] Global error handling & input validation
- [ ] 70%+ test coverage
- [ ] Dockerized and running

---

### Week 4 — Cart Service (Golang)

**Goal:** Redis-first shopping cart with background PostgreSQL persistence, price validation via Product Service, circuit breaker, **and Go concurrency patterns (goroutines, channels, errgroup)**.

---

#### Day 19 (3h) — Redis Cart Storage & Basic Operations

**Learn:**
- Redis data structures: Hash vs String for complex objects
- Redis TTL management: `EXPIRE`, `TTL`, extending TTL on writes
- JSON serialization in Go (`encoding/json`)
- Designing Redis-first architectures

**Do:**
- [ ] Implement `cache/cart_cache.go`:
  - `GetCart(ctx, userId) → CartDTO`
  - `SetCart(ctx, userId, cart)` — serialize to JSON, SET with 30-min TTL
  - `DeleteCart(ctx, userId)`
  - `ExtendTTL(ctx, userId)` — reset to 30 min on every write
- [ ] Implement `service/cart_service.go`:
  - `GetOrCreateCart(ctx, userId)` — get from Redis, or create new empty cart
  - `AddItem(ctx, userId, productId, quantity)` — add item or increment quantity
  - `UpdateItemQuantity(ctx, userId, productId, quantity)` — set new quantity
  - `RemoveItem(ctx, userId, productId)` — remove from items array
  - `ClearCart(ctx, userId)` — delete entire cart
  - `GetCartTotal(cart)` — compute total from items
- [ ] Implement `handler/cart_handler.go`:
  - All CRUD endpoints (POST, GET, PUT, DELETE)
- [ ] Apply JWT auth middleware (reuse from User Service `pkg/jwt`)
- [ ] Test: add items, update quantity, remove item, clear cart — all via Postman

**Deliverable:** Fully functional Redis-backed cart with all CRUD operations.

---

#### Day 20 (3h) — Product Service HTTP Client & Circuit Breaker

**Learn:**
- Go HTTP client: `net/http`, custom transport, timeouts
- Circuit breaker pattern: Closed → Open → Half-Open state transitions
- `gobreaker` library: configuration, callbacks, fallback behavior
- Service discovery in Docker (container DNS)
- **`context.WithTimeout` — preventing goroutine leaks on slow external calls**

**Do:**
- [ ] Implement `client/product_client.go`:
  - `GetProduct(ctx, productId) → ProductDTO, error` — HTTP GET to Product Service
  - `CheckStock(ctx, productId) → available int, error`
  - **Configure timeout: `context.WithTimeout(ctx, 3*time.Second)` on every call**
- [ ] Wrap with circuit breaker (gobreaker configuration from proposal §4.4)
- [ ] Implement fallback: when circuit is open, return cached product data from Redis (stale but available)
- [ ] Update `AddItem` flow:
  1. Call Product Service to validate product exists and is active
  2. Get current price from Product Service
  3. Store price snapshot in cart item
  4. If circuit is open → allow add with cached price + log warning
- [ ] Test: add item → product service returns data → cart stores it
- [ ] Test: stop product service → circuit opens → fallback to cached data

**Deliverable:** Cart validates products via Product Service with circuit breaker protection.

---

#### Day 21 (3h) — Background Persistence & Checkout Flow

**Learn:**
- **Go goroutines for background work — channel-based communication**
- **Debouncing with `time.Ticker` and `select` statement**
- GORM transactions for multi-table writes
- Checkout validation: price drift detection
- **`errgroup` for parallel product validation** (validate all cart items concurrently)

**Do:**
- [ ] Define GORM models: `Cart` (id, user_id, status, expires_at) and `CartItem` (cart_id, product_id, product_name, quantity, unit_price)
- [ ] Implement `repository/cart_repository.go`: GORM-based CRUD
- [ ] **Implement background persistence goroutine** (from proposal §4.4):
  - Channel receives dirty cart IDs on every modification
  - Ticker flushes pending carts to PostgreSQL every 5 minutes
  - `ctx.Done()` triggers final flush on shutdown
- [ ] Implement cart expiry handler:
  - When Redis key expires (TTL), the next background sync marks cart as `ABANDONED` in PostgreSQL
- [ ] Implement `POST /api/v1/carts/me/checkout`:
  1. Load cart from Redis
  2. **For each item: validate in parallel using `errgroup`** (call Product Service concurrently)
  3. Compare stored price vs current price — flag if changed
  4. Verify stock available for all items
  5. Return checkout summary: items, total, any price warnings
  6. Client decides whether to proceed → calls `POST /orders`
- [ ] Test: add items → wait → verify PostgreSQL has the cart → checkout → see summary

**Deliverable:** Cart persists to PostgreSQL in background via goroutine. Checkout validates prices and stock in parallel.

---

#### Day 22 (3h) — Cart Service Testing, Dockerfile & Docker Compose

**Learn:**
- Testing Redis operations with miniredis (in-memory Redis for tests)
- Mocking HTTP clients in Go tests
- Testing goroutines with channels and timeouts

**Do:**
- [ ] Write unit tests for `service/cart_service.go`:
  - Add item → cart contains item with correct price
  - Add duplicate item → quantity increments
  - Update quantity to 0 → item removed
  - Clear cart → empty
  - Checkout with stale price → warning returned
  - Checkout with insufficient stock → error returned
- [ ] Write unit tests for circuit breaker behavior:
  - Product service available → returns data
  - Product service down → circuit opens → fallback to cache
- [ ] Write `cart-service/Dockerfile` (multi-stage Go build)
- [ ] Add to `docker-compose.yml` (port 8002, depends on postgres, redis, product-service)
- [ ] `docker-compose up --build` — verify cart-service starts and connects
- [ ] End-to-end test: register user → login → add to cart → get cart → checkout
- [ ] **Run `go test -race ./...`** — verify no data races in background goroutine
- [ ] Git: commit and push

**Deliverable:** Cart Service fully tested, containerized, race-detector clean.

---

#### Day 23 (3h) — Go Concurrency Deep-Dive: Patterns & Best Practices

**Learn:**
- **`sync.Pool`** — object reuse for reducing GC pressure
- **`singleflight`** — preventing cache stampede (multiple goroutines requesting the same cache key)
- **`sync.RWMutex`** — concurrent reads with exclusive writes
- **Go benchmarks: `func BenchmarkXxx(b *testing.B)`** — measure before optimizing
- **Go `pprof`** — CPU, memory, and goroutine profiling

**Do:**
- [ ] **Implement `sync.Pool` for JSON serialization buffers** in Cart Service:
  ```go
  var bufPool = sync.Pool{
      New: func() interface{} { return new(bytes.Buffer) },
  }
  ```
- [ ] **Add `pprof` endpoint to Cart Service** (on separate port 6062, not exposed to public):
  ```go
  import _ "net/http/pprof"
  go http.ListenAndServe(":6062", nil) // Profiling server
  ```
- [ ] **Write Go benchmarks** for cart serialization/deserialization:
  ```go
  func BenchmarkSerializeCart(b *testing.B) {
      cart := generateLargeCart(50)
      b.ResetTimer()
      for i := 0; i < b.N; i++ { serializeCart(cart) }
  }
  ```
  Run: `go test -bench=. -benchmem ./internal/cache/`
- [ ] **Implement singleflight for Product Service lookups**:
  - If 100 goroutines request the same product simultaneously, only 1 calls Product Service; others wait for the result
- [ ] **Write concurrent cart update test** (from proposal §4.4):
  - 10 goroutines adding different items to the same cart simultaneously
  - Verify: no items lost (Redis WATCH/MULTI/EXEC works)
- [ ] Profile the Cart Service with pprof:
  - Run a load test → visit `http://localhost:6062/debug/pprof/`
  - Take a CPU profile: `go tool pprof http://localhost:6062/debug/pprof/profile?seconds=10`
  - Check goroutine count: no leaks
- [ ] Document: pprof usage guide in a comment block in `main.go`

**Deliverable:** Cart Service with production-grade Go concurrency patterns. Benchmarks and profiling in place.

---

#### Day 24 (3h) — Redis Atomic Operations & Distributed Race Conditions

**Learn:**
- **Redis `WATCH/MULTI/EXEC`** — optimistic locking at Redis level (from proposal §4.4)
- **Redis Lua scripts** — atomic operations that can't be interrupted
- **Race condition scenario: two browser tabs updating the same cart** — how to prevent lost updates
- **Redis `SET NX` (SET if Not eXists)** — distributed locking primitive

**Do:**
- [ ] **Implement `AddItemAtomic()` with Redis WATCH/MULTI/EXEC** (from proposal §4.4):
  - WATCH the cart key → read → modify → EXEC
  - If another goroutine modified the cart between WATCH and EXEC → retry (up to 3x)
- [ ] **Write a Lua script for atomic stock check** (proposal §8.4):
  ```lua
  -- Check and decrement atomically — no race between check and update
  local current = tonumber(redis.call('GET', KEYS[1]) or 0)
  if current >= tonumber(ARGV[1]) then
      redis.call('DECRBY', KEYS[1], ARGV[1])
      return 1
  else
      return 0
  end
  ```
- [ ] **Write a concurrent cart update stress test**:
  - 50 goroutines adding items to the same cart
  - With WATCH/MULTI: no items lost
  - Without WATCH/MULTI (naive SET): items lost — demonstrate why atomicity matters
- [ ] **Implement a simple Redis distributed lock** (for future use):
  ```go
  func AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
      return redis.SetNX(ctx, "lock:"+key, "1", ttl).Result()
  }
  ```
- [ ] Test: verify atomic cart operations under concurrent load

**Deliverable:** Redis atomic operations implemented. Stress test proves no lost updates. Distributed lock ready.

---

### Week 4 Checkpoint ✅

- [ ] Redis-first cart with 30-min TTL
- [ ] **Background PostgreSQL persistence via goroutine + channel pattern**
- [ ] Product validation via HTTP + circuit breaker
- [ ] Checkout with **parallel** price re-validation (errgroup) and stock check
- [ ] **Redis WATCH/MULTI/EXEC for concurrent cart safety**
- [ ] **sync.Pool, singleflight, pprof profiling set up**
- [ ] **Go benchmarks for hot paths**
- [ ] Cart expiry → ABANDONED status
- [ ] Unit tests with mocked dependencies, `go test -race` clean
- [ ] Dockerized and running

---

### Week 5 — Order Service (Java/Spring Boot) — Includes Notifications

**Goal:** Order lifecycle with state machine, Kafka event publishing, stock orchestration, inline notification dispatch, **and Java concurrency patterns (CompletableFuture, @Async, @Transactional, pessimistic locking)**.

---

#### Day 25 (3h) — Order Model, State Machine & Basic CRUD

**Learn:**
- State machine pattern in Java (enum-based transitions with validation)
- JSONB column mapping in JPA (`@Type(JsonBinaryType.class)` or `@JdbcTypeCode`)
- Auditing: `@CreatedDate`, `@LastModifiedDate`, entity listeners
- **`canTransitionTo()` method — why state machine validation matters for concurrent transitions**

**Do:**
- [ ] Define JPA entities: `Order`, `OrderItem`, `OrderStatusHistory`, `Notification`
  - Order: `status` enum (PENDING, CONFIRMED, CANCELLED, SHIPPED, DELIVERED)
  - `shippingAddress` as JSONB
- [ ] Write Flyway migration `V1__create_order_schema.sql`
- [ ] **Implement `OrderStateMachine`** with `canTransitionTo()`:
  ```java
  public enum OrderStatus {
      PENDING, CONFIRMED, CANCELLED, SHIPPED, DELIVERED;
      
      public boolean canTransitionTo(OrderStatus target) {
          return switch (this) {
              case PENDING -> target == CONFIRMED || target == CANCELLED;
              case CONFIRMED -> target == SHIPPED;
              case SHIPPED -> target == DELIVERED;
              default -> false;
          };
      }
  }
  ```
- [ ] Implement `OrderService.createOrder(userId, cartId, shippingAddress)`:
  1. Validate cart exists and belongs to user
  2. Create Order with status=PENDING, copy items from cart
  3. Log status history: `null → PENDING`
- [ ] Implement controller: `POST /api/v1/orders`, `GET /api/v1/orders/{id}`, `GET /api/v1/orders`
- [ ] **Add `@EntityGraph(attributePaths = {"items", "statusHistory"})` to prevent N+1 queries**
- [ ] Test: create order → retrieve it → list user's orders

**Deliverable:** Basic order creation with state machine validation. N+1 queries eliminated.

---

#### Day 26 (3h) — Stock Reservation Integration & Kafka Producer

**Learn:**
- Spring `RestTemplate` or `WebClient` for HTTP calls to Product Service
- Resilience4j `@CircuitBreaker` on external calls
- Spring Kafka: `KafkaTemplate`, `ProducerConfig`, JSON serialization
- Kafka topic configuration: partitions, replication factor
- **`CompletableFuture.allOf()` for parallel stock reservation** (from proposal §4.5)

**Do:**
- [ ] Implement `client/ProductServiceClient`:
  - `reserveStock(productId, quantity)` → HTTP POST to Product Service
  - `releaseStock(productId, quantity)` → HTTP POST
  - `confirmStock(productId, quantity)` → HTTP POST
  - Add `@CircuitBreaker(name = "productService", fallbackMethod = "...")`
- [ ] **Update `createOrder` flow with parallel reservation** (from proposal §4.5):
  1. Launch `CompletableFuture` for each item → `reserveStock()` in parallel
  2. `CompletableFuture.allOf().join()` — wait for all
  3. On failure → compensate: release all successfully reserved items
  4. On success → save order as PENDING, publish Kafka event
- [ ] Implement `kafka/OrderEventProducer`:
  ```java
  kafkaTemplate.send("orders.created", orderId, OrderCreatedEvent.builder()
      .orderId(orderId).userId(userId).items(items).totalAmount(total).build());
  ```
- [ ] Configure Kafka producer in `application.yml`
- [ ] Test: create order → verify stock reserved → event published to Kafka

**Deliverable:** Order creation with **parallel stock reservation** and Kafka events.

---

#### Day 27 (3h) — Kafka Consumer & Payment Event Handling

**Learn:**
- Spring Kafka `@KafkaListener`: consumer groups, topic subscription, deserialization
- Handling Kafka consumer errors: retry + DLQ
- Transactional event processing: update DB only after successful processing
- **Pessimistic locking on order state transitions** (from proposal §4.5)

**Do:**
- [ ] Implement `kafka/PaymentEventConsumer`:
  ```java
  @KafkaListener(topics = "payments.completed", groupId = "order-service")
  public void handlePaymentCompleted(PaymentCompletedEvent event) {
      orderService.confirmOrder(event.getOrderId());
  }
  
  @KafkaListener(topics = "payments.failed", groupId = "order-service")
  public void handlePaymentFailed(PaymentFailedEvent event) {
      orderService.cancelOrder(event.getOrderId(), event.getReason());
  }
  ```
- [ ] **Implement `transitionStatus()` with pessimistic locking** (from proposal §4.5):
  ```java
  @Transactional
  public Order transitionStatus(Long orderId, OrderStatus newStatus, String reason) {
      Order order = orderRepository.findByIdWithLock(orderId) // SELECT FOR UPDATE
          .orElseThrow(() -> new OrderNotFoundException(orderId));
      if (!order.getStatus().canTransitionTo(newStatus)) {
          throw new InvalidStateTransitionException(order.getStatus(), newStatus);
      }
      order.setStatus(newStatus);
      // ... audit trail
  }
  ```
- [ ] Implement `OrderService.confirmOrder(orderId)` and `cancelOrder(orderId, reason)`
  - Confirm: PENDING → CONFIRMED, confirm stock, send notification
  - Cancel: PENDING → CANCELLED, release stock, send notification
- [ ] Implement `PUT /api/v1/orders/{id}/cancel` — user-initiated cancellation
- [ ] Test: manually publish events to Kafka → order transitions correctly
- [ ] **Test concurrent transition** (from proposal §10.4, Scenario 2):
  - Simultaneously send "completed" Kafka event + user cancel → one wins, other gets error

**Deliverable:** Order Service reacts to payment events with race-safe state transitions.

---

#### Day 28 (3h) — Notification Dispatcher & @Async Patterns

**Learn:**
- Template rendering in Java (Thymeleaf or simple string templates)
- **`@Async` with custom ThreadPoolTaskExecutor** — fire-and-forget without blocking
- **`CompletableFuture` return type for async methods** — allows the caller to ignore or await
- Java `@Async` error handling — exceptions don't propagate to caller

**Do:**
- [ ] **Configure custom async executor** (from proposal §10.3):
  ```java
  @Bean("notificationExecutor")
  public Executor notificationExecutor() {
      ThreadPoolTaskExecutor executor = new ThreadPoolTaskExecutor();
      executor.setCorePoolSize(2);
      executor.setMaxPoolSize(5);
      executor.setQueueCapacity(100);
      executor.setRejectedExecutionHandler(new CallerRunsPolicy());
      return executor;
  }
  ```
- [ ] **Implement `NotificationDispatcher` with @Async** (from proposal §4.5):
  - `sendOrderConfirmation(order)` — `@Async("notificationExecutor")`, returns `CompletableFuture<Void>`
  - `sendOrderCancellation(order, reason)` — same pattern
  - `sendShipmentNotification(order, trackingNumber)` — same
  - All methods: catch exceptions internally, log, save to `notifications` table, never throw
- [ ] Create simple email templates in `resources/templates/`
- [ ] Implement remaining endpoints:
  - `PUT /api/v1/orders/{id}/ship` — CONFIRMED→SHIPPED, send notification
  - `PUT /api/v1/orders/{id}/deliver` — SHIPPED→DELIVERED, send notification
  - `GET /api/v1/orders/{id}/history` — return status change audit trail
- [ ] Test full order lifecycle: create → confirm → ship → deliver (with notifications logged)
- [ ] **Verify @Async works: order response returns before notification is sent** (check timestamps)

**Deliverable:** Complete order lifecycle with fire-and-forget async notifications.

---

#### Day 29 (3h) — Order Service Testing

**Do:**
- [ ] Unit tests for `OrderStateMachine`: all valid and invalid transitions
- [ ] Unit tests for `OrderService`:
  - Create order → stock reserved in parallel → event published
  - **Confirm order with pessimistic lock → stock confirmed → notification sent async**
  - Cancel order → stock released → notification sent
  - Invalid state transition → exception
  - Stock reservation failure → **all reservations compensated (CompletableFuture compensation)**
- [ ] Unit tests for `PaymentEventConsumer`: verify correct methods called
- [ ] Unit tests for `NotificationDispatcher`: verify notification saved to DB
- [ ] **Concurrent state transition test: 2 threads transition same order → 1 wins, 1 fails cleanly**
- [ ] Run: `mvn test` → 70%+ coverage

**Deliverable:** Order Service with comprehensive test coverage including concurrency tests.

---

#### Day 30 (3h) — Order Service Dockerfile & Integration

**Do:**
- [ ] Write `order-service/Dockerfile` (with JVM container flags)
- [ ] Add to `docker-compose.yml` (port 8082, depends on postgres, redis, kafka, product-service)
- [ ] `docker-compose up --build` — verify all services start
- [ ] Integration test: create user → add products to cart → checkout → create order → verify stock reserved
- [ ] Test Kafka integration: order created → event in topic → (simulate payment manually for now)
- [ ] Git: commit and push

**Deliverable:** Order Service containerized, Kafka-integrated, ready for Payment Service.

---

### Week 5 Checkpoint ✅

- [ ] Order creation with **parallel stock reservation** (CompletableFuture)
- [ ] Order state machine with **pessimistic locking** on transitions
- [ ] Kafka event publishing (orders.created, orders.confirmed, orders.cancelled)
- [ ] Kafka consumer for payment events
- [ ] **Async notifications** (fire-and-forget with custom thread pool)
- [ ] Status change audit trail
- [ ] **Concurrent transition tests pass**
- [ ] 70%+ test coverage
- [ ] Dockerized and running

---

### Week 6 — Payment Service (Golang)

**Goal:** Complete the saga loop. Payment Service consumes order events, processes payments with idempotency, publishes results, **using Go worker pool pattern for Kafka consumption**.

---

#### Day 31 (3h) — Payment Model, Idempotency & Basic Processing

**Learn:**
- Idempotency keys: why they matter, how to implement
- **Race condition: duplicate Kafka events → how idempotency key + DB unique constraint prevents double charges** (proposal §10.4, Scenario 3)
- Transaction safety: create payment record before processing

**Do:**
- [ ] Define GORM models: `Payment`, `PaymentHistory`
- [ ] Implement `repository/payment_repository.go`: `Create`, `FindByOrderID`, `FindByIdempotencyKey`, `UpdateStatus`
- [ ] Implement `service/payment_service.go` (from proposal §4.6):
  - `ProcessPayment(ctx, req)`:
    1. Check idempotency key → return existing result if found
    2. Create payment record (status=PENDING) — **claim the idempotency key**
    3. Handle race: if `isDuplicateKeyError(err)` → another goroutine claimed it first → return existing
    4. Call mock gateway **with `context.WithTimeout(ctx, 5*time.Second)`**
    5. Update status (COMPLETED or FAILED)
    6. Publish Kafka event
- [ ] Implement `gateway/mock_gateway.go`:
  - Configurable success rate (default 90%)
  - Random latency 50-200ms
  - Generate realistic reference IDs
  - Special card `4000000000000002` → always fails (for testing)
- [ ] Implement `handler/payment_handler.go`: POST, GET endpoints
- [ ] Test: create payment → verify idempotency → same key returns same result

**Deliverable:** Payment processing with idempotency and mock gateway. Race-safe.

---

#### Day 32 (3h) — Kafka Worker Pool Consumer & Producer

**Learn:**
- `confluent-kafka-go` consumer: `Subscribe`, `Poll`, `CommitMessage`
- Consumer groups and partition assignment
- **Go worker pool pattern: fan-out from channel to N goroutines** (from proposal §4.6)
- Kafka producer: `Produce`, delivery reports
- **Graceful shutdown: drain workers before exit**

**Do:**
- [ ] **Implement Kafka worker pool consumer** (from proposal §4.6):
  ```go
  func (c *Consumer) Start(ctx context.Context, workerCount int) {
      events := make(chan OrderCreatedEvent, 100)
      var wg sync.WaitGroup
      for i := 0; i < workerCount; i++ {
          wg.Add(1)
          go func(id int) {
              defer wg.Done()
              for event := range events {
                  c.processPayment(ctx, event)
              }
          }(i)
      }
      // Kafka reader goroutine → events channel → workers
      go func() {
          defer close(events)
          for { /* read from Kafka, send to channel */ }
      }()
      <-ctx.Done()
      wg.Wait() // Wait for in-flight payments
  }
  ```
- [ ] Implement `kafka/producer.go`: `PublishPaymentCompleted()`, `PublishPaymentFailed()`
- [ ] Implement DLQ: failed events → `payments.dlq` after 3 retries
- [ ] **Implement graceful shutdown: `os.Signal` → cancel context → workers drain → close Kafka**
- [ ] Test the full saga manually:
  1. Create order (Order Service) → `orders.created` event
  2. Payment Service worker pool consumes → processes → publishes `payments.completed`
  3. Order Service consumes → confirms order
- [ ] Verify with Kafka CLI: `kafka-console-consumer` shows events flowing

**Deliverable:** Full saga loop with Go worker pool consumer.

---

#### Day 33 (3h) — Refunds, DLQ & Edge Cases

**Learn:**
- Refund patterns: full refund, partial refund
- Dead Letter Queue: monitoring and replay
- Handling duplicate Kafka messages (exactly-once processing via idempotency)

**Do:**
- [ ] Implement `POST /api/v1/payments/{id}/refund`
- [ ] Implement DLQ handling:
  - `payments.dlq` topic in Kafka
  - Admin endpoint: `GET /api/v1/payments/dlq` — list failed payments
  - Admin endpoint: `POST /api/v1/payments/dlq/{id}/replay` — reprocess
- [ ] Handle edge cases:
  - Payment for already-cancelled order → skip
  - **Duplicate `orders.created` event → idempotency key prevents double processing**
  - **Gateway timeout → context.WithTimeout expires → mark as FAILED**
- [ ] Test all edge cases: duplicate payment, refund, DLQ replay

**Deliverable:** Robust payment handling with refunds and DLQ.

---

#### Day 34 (3h) — Payment Service Testing & Dockerization

**Do:**
- [ ] Unit tests for `PaymentService`:
  - Successful payment → COMPLETED, event published
  - Failed payment → FAILED, event published
  - **Idempotent: same key → same result, no reprocessing**
  - **Race: two goroutines with same key → only one processes, other returns existing**
  - Refund: COMPLETED → REFUNDED
  - Refund: PENDING → error (invalid state)
- [ ] Unit tests for mock gateway: success rate, forced failure card
- [ ] Write `payment-service/Dockerfile`
- [ ] Add to `docker-compose.yml` (port 8003, depends on postgres, kafka)
- [ ] **Full end-to-end test with all 5 services** — the complete saga
- [ ] **`go test -race ./...`** — verify no data races in worker pool
- [ ] **🎉 Celebrate: the complete saga works!**
- [ ] Git: commit and push

**Deliverable:** Full e-commerce saga working end-to-end. All 5 services communicating. Race-detector clean.

---

#### Day 35 (3h) — Go Concurrency Patterns: Worker Pool & Channel Deep-Dive

**Learn:**
- **Worker pool with bounded concurrency** — why we use buffered channels as semaphores
- **Fan-out / Fan-in pattern** — distribute work, collect results
- **`select` statement with `time.After`** — timeout per-event processing
- **`atomic` package** — lock-free counters for metrics

**Do:**
- [ ] **Enhance the Kafka worker pool with per-event timeout**:
  ```go
  select {
  case events <- event:
      // Worker will process
  case <-time.After(10 * time.Second):
      log.Warn("Worker pool full, sending to DLQ")
      sendToDLQ(event)
  }
  ```
- [ ] **Add atomic metrics counters** to Payment Service:
  ```go
  var (
      processedCount int64  // atomic.AddInt64
      failedCount    int64
      dlqCount       int64
  )
  // Expose via GET /metrics
  ```
- [ ] **Implement fan-out/fan-in for batch payment status checks**:
  - Given N order IDs, check payment status for all concurrently
  - Collect results via results channel
- [ ] **Add `pprof` endpoint to Payment Service** (port 6063)
- [ ] **Write Go benchmarks for payment processing**:
  ```go
  func BenchmarkProcessPayment(b *testing.B) {
      // Benchmark the full flow: idempotency check → gateway → DB save
  }
  ```
- [ ] Profile: take a CPU profile during load, identify hotspots
- [ ] Document: add comments explaining why each concurrency pattern is used

**Deliverable:** Payment Service with advanced Go concurrency patterns. Profiling data captured.

---

#### Day 36 (3h) — Payment Concurrency Stress Testing

**Learn:**
- **Stress testing concurrent payment processing** — prove the system is correct under load
- **Go's `testing.T.Parallel()`** — run subtests concurrently
- **Scenario-based concurrency tests** — simulate real-world race conditions

**Do:**
- [ ] **Write stress test: 100 goroutines sending payments for the same order**:
  - All use the same idempotency key
  - Assert: exactly 1 payment created, 99 return the existing payment
  - Assert: exactly 1 Kafka event published
- [ ] **Write stress test: 50 goroutines sending payments for different orders simultaneously**:
  - All using the worker pool (5 workers)
  - Assert: all 50 payments processed correctly
  - Assert: no goroutine leaks (check goroutine count before/after)
- [ ] **Write stress test: payment + refund race**:
  - One goroutine processes a payment, another immediately tries to refund
  - Assert: refund waits for payment to complete, or fails cleanly if payment not yet COMPLETED
- [ ] **Run pprof goroutine dump during stress test**:
  - `go tool pprof http://localhost:6063/debug/pprof/goroutine`
  - Verify: no goroutine leaks (goroutine count returns to baseline after test)
- [ ] **Run the race detector during stress tests**:
  - `go test -race -count=5 ./internal/service/` (run 5 times to increase race detection probability)
- [ ] Document all stress test results

**Deliverable:** Comprehensive concurrency stress tests proving the Payment Service is race-condition free.

---

### Week 6 Checkpoint ✅

- [ ] Payment processing with mock gateway
- [ ] **Idempotency guarantee (proven with 100-goroutine stress test)**
- [ ] **Kafka worker pool consumer (fan-out via channels)**
- [ ] Dead Letter Queue for failed payments
- [ ] Refund support
- [ ] Full saga: Order → Payment → Confirmation/Cancellation
- [ ] **Go benchmarks and pprof profiling**
- [ ] **Atomic metrics counters**
- [ ] All 5 services running and communicating
- [ ] 70%+ test coverage, `go test -race` clean

---

## Phase 3: Integration

### Week 7 — Docker Compose, Nginx, End-to-End Wiring & Cloud Deployment

**Goal:** Production-grade Docker Compose, Nginx reverse proxy, verified end-to-end flows, **and cloud deployment to AWS or GCP**.

---

#### Day 37 (3h) — Nginx Configuration

**Learn:**
- Nginx `upstream` blocks and `proxy_pass`
- Nginx `limit_req_zone` for rate limiting
- CORS headers in Nginx
- Nginx access log format customization

**Do:**
- [ ] Create `nginx/nginx.conf` with full configuration:
  - Upstream blocks for all 5 services
  - Rate limiting: `limit_req_zone $binary_remote_addr zone=api:10m rate=100r/m`
  - CORS headers
  - `X-Correlation-ID` generation: `set $correlation_id $request_id;`
- [ ] Add Nginx to `docker-compose.yml`
- [ ] Test: `curl http://localhost/api/v1/products` routes through Nginx
- [ ] Test rate limiting: send 150 requests in 1 minute → 429 responses after limit

**Deliverable:** Nginx routing all traffic, rate limiting active, CORS configured.

---

#### Day 38 (3h) — Docker Compose Hardening

**Do:**
- [ ] Add health checks to all service containers
- [ ] Add `depends_on` with `condition: service_healthy`
- [ ] Add resource limits to all containers:
  ```yaml
  deploy:
    resources:
      limits:
        cpus: '0.5'
        memory: 512M
  ```
- [ ] Add Docker networks: `backend` (internal) + `frontend` (nginx + services)
- [ ] Add named volumes for data persistence: `postgres_data`, `redis_data`, `kafka_data`
- [ ] Verify: `docker-compose down && docker-compose up` — everything starts correctly

**Deliverable:** Production-grade Docker Compose with health checks, resource limits, ordered startup.

---

#### Day 39 (3h) — End-to-End Flow Testing

**Do:**
- [ ] Create Postman collection with full user journey:
  1. Register → Login → Get JWT
  2. Browse products → Search → Add to cart
  3. Checkout → Create order → Wait for Kafka saga → Verify confirmed
  4. Check payment status
- [ ] Create Postman collection for failure paths:
  1. Insufficient stock → 409
  2. Payment fails → order cancelled → stock released
  3. Duplicate payment → returns original
  4. Rate limiting → 429
  5. Invalid JWT → 401
- [ ] Export Postman collections to `api/postman/`
- [ ] Fix any bugs found during E2E testing

**Deliverable:** Verified end-to-end flows (happy + failure paths). Postman collections committed.

---

#### Day 40 (3h) — Seed Data, Scripts & Developer Experience

**Do:**
- [ ] Enhance `script/seed-data.sql`: 10 categories, 100 products, 5 test users
- [ ] Create `script/reset-dev.sh` for clean environment reset
- [ ] Create `Makefile` for common tasks:
  ```makefile
  up:           docker-compose up --build -d
  down:         docker-compose down
  reset:        docker-compose down -v && docker-compose up --build -d
  logs:         docker-compose logs -f
  test-go:      cd user-service && go test -race ./... -cover
  test-java:    cd product-service && mvn test
  profile:      curl http://localhost:6062/debug/pprof/profile?seconds=30 > cpu.prof
  ```
- [ ] Test: `make reset` → fresh environment → `make up` → everything works
- [ ] Git: commit and push

**Deliverable:** Developer-friendly scripts, rich seed data, Makefile with profiling commands.

---

#### Day 41 (3h) — Cloud Deployment: AWS Scenario

**Learn:**
- **AWS Free Tier: what's included** (EC2 t2.micro 750h/month, RDS 750h/month, ElastiCache 750h/month — all for 12 months)
- **What's NOT free**: MSK (Kafka), NAT Gateway, ECS Fargate
- EC2 instance setup: SSH, security groups, Docker installation
- **Docker Compose on a single EC2 instance** — simplest cloud deployment

**Do:**
- [ ] **Document AWS architecture** (from proposal §13.4, Scenario A):
  - Option 1: All-on-EC2 (Docker Compose on t3.small, ~$15/month)
  - Option 2: EC2 + RDS + ElastiCache (use managed services where free tier available)
- [ ] **Create `docs/deploy-aws.md` with step-by-step instructions**:
  1. Create AWS account (get 12-month free tier)
  2. Launch EC2 t3.small (or t2.micro free tier — tight but works)
  3. Configure security group: SSH (your IP), HTTP (0.0.0.0/0)
  4. SSH in, install Docker & Docker Compose
  5. Clone repo, update `.env` for cloud, `docker-compose up --build -d`
  6. Access via EC2 public IP
- [ ] **Create `docker-compose.cloud.yml`** — optimized for single-VM deployment:
  - Reduced memory limits for smaller VM
  - Health checks with longer start periods
  - Volume mounts for data persistence
- [ ] Test locally: `docker-compose -f docker-compose.cloud.yml up` works
- [ ] **Document AWS cost breakdown**:
  | Resource | Free Tier | After Free Tier |
  |---|---|---|
  | EC2 t3.small | NOT free ($15/mo) | $15/mo |
  | RDS db.t3.micro | 750h free (12 mo) | $13/mo |
  | ElastiCache cache.t3.micro | 750h free (12 mo) | $12/mo |
  | Kafka | Self-host on EC2 | $0 |

**Deliverable:** AWS deployment guide. Cloud-optimized docker-compose. Cost analysis documented.

---

#### Day 42 (3h) — Cloud Deployment: GCP Scenario

**Learn:**
- **GCP Free Tier: $300 credit for 90 days**, e2-micro always free
- **Cloud Run: serverless containers** — 2M requests/month free, scales to zero
- GCP Compute Engine vs Cloud Run trade-offs
- **Pub/Sub as Kafka alternative** — managed, serverless, cheaper for small projects

**Do:**
- [ ] **Document GCP architecture** (from proposal §13.4, Scenario B):
  - Option 1: Docker Compose on Compute Engine e2-small (~$13/month, covered by $300 credit)
  - Option 2: **Cloud Run** (deploy each service as a Cloud Run service — serverless, more impressive for interviews)
- [ ] **Create `docs/deploy-gcp.md` with step-by-step instructions**:
  - Option 1 (VM): same as AWS but on GCP Compute Engine
  - Option 2 (Cloud Run):
    1. Install `gcloud` CLI
    2. Create GCP project, enable billing ($300 free credit)
    3. Enable Cloud Run, Artifact Registry APIs
    4. For each service: `gcloud run deploy <service> --source .`
    5. Set up Cloud SQL for PostgreSQL, Memorystore for Redis
    6. Configure service-to-service auth
- [ ] **Document GCP cost breakdown**:
  | Resource | Free Credits | After Credits |
  |---|---|---|
  | $300 credit (90 days) | Covers everything | — |
  | Cloud Run | 2M req/month free | Pay-per-use (~$5-10) |
  | Cloud SQL | Covered by credits | ~$7/mo |
  | e2-small VM | Covered by credits | ~$13/mo |
- [ ] **Document AWS vs GCP recommendation** (from proposal §13.4):
  - Cheapest first 3 months: GCP ($300 credit)
  - Most impressive for interview: GCP Cloud Run (serverless)
  - Simplest deployment: Docker Compose on any VM
- [ ] Create comparison table in `docs/deploy-comparison.md`

**Deliverable:** GCP deployment guide. Cloud Run instructions. AWS vs GCP comparison.

---

### Week 7 Checkpoint ✅

- [ ] Nginx reverse proxy routing all traffic
- [ ] Rate limiting (100 req/min per IP)
- [ ] CORS configured
- [ ] Docker Compose with health checks and startup ordering
- [ ] Full E2E flow verified (happy + failure paths)
- [ ] Postman collections exported
- [ ] Seed data with 100 products, 5 users
- [ ] Makefile for developer workflows
- [ ] **AWS deployment guide with cost analysis**
- [ ] **GCP deployment guide with Cloud Run instructions**
- [ ] **Cloud-optimized docker-compose.cloud.yml**

---

## Phase 4: Frontend

### Week 8 — Minimal Frontend (React + Vite)

**Goal:** 4-page web app that demonstrates the full e-commerce flow. Not pretty — functional. Use a component library to look professional with minimal CSS work.

---

#### Day 43 (3h) — Project Setup, Auth Pages & API Client

**Learn:**
- React + Vite project scaffolding
- Ant Design (or shadcn/ui) component library
- Axios for HTTP requests + interceptors for JWT
- React Router for page navigation
- React Context for auth state management

**Do:**
- [ ] Scaffold frontend: `npm create vite@latest frontend -- --template react-ts`
- [ ] Install dependencies: `npm i antd axios react-router-dom @ant-design/icons`
- [ ] Set up project structure: `api/`, `components/`, `pages/`, `context/`, `hooks/`, `types/`
- [ ] Create `api/client.ts`: Axios instance with JWT interceptor + 401 redirect
- [ ] Create `context/AuthContext.tsx`: auth state management
- [ ] Create `pages/LoginPage.tsx`: login/register tabs with Ant Design Form
- [ ] Wire routing: `/login` → LoginPage, protected routes redirect to login
- [ ] Test: login page renders, can register + login, JWT stored

**Deliverable:** Frontend with working authentication (login/register).

---

#### Day 44 (3h) — Product Listing & Search Page

**Learn:**
- React `useEffect` + `useState` for data fetching
- Ant Design `Card`, `List`, `Input.Search`, `Pagination`, `Select`
- Debouncing search input (avoid API call on every keystroke)

**Do:**
- [ ] Create `api/products.ts`: API functions
- [ ] Create `pages/ProductsPage.tsx`:
  - Grid of product cards (image, name, price, stock badge)
  - Search bar (debounced, 300ms)
  - Category filter, price range filter
  - Pagination
  - "Add to Cart" button on each card
- [ ] Create `components/ProductCard.tsx` and `components/Header.tsx`
- [ ] Implement "Add to Cart" flow with toast notifications
- [ ] Test: browse products, search, filter, add to cart

**Deliverable:** Product browsing page with search, filters, and add-to-cart.

---

#### Day 45 (3h) — Cart Page

**Learn:**
- Ant Design `Table` for structured data display
- Optimistic UI updates (update UI before API responds)
- Number input with min/max constraints

**Do:**
- [ ] Create `api/cart.ts`: all cart API functions
- [ ] Create `pages/CartPage.tsx`:
  - Table: Product, Price, Quantity (editable), Subtotal, Remove
  - Summary section, "Clear Cart", "Proceed to Checkout"
- [ ] Implement quantity update and item removal
- [ ] Empty cart state with link to products
- [ ] Test: view cart, update quantities, remove items, see totals update

**Deliverable:** Functional cart page with quantity editing and item removal.

---

#### Day 46 (3h) — Checkout & Order Tracking Page

**Learn:**
- Polling for async status updates (order saga takes a few seconds)
- Ant Design `Steps`, `Result`, `Descriptions`

**Do:**
- [ ] Create `pages/CheckoutPage.tsx`:
  - Checkout summary from `POST /carts/me/checkout`
  - Price change warnings
  - Shipping address form
  - "Place Order" → `POST /orders`
  - **Poll `GET /orders/{id}` every 2 seconds until status != PENDING** (shows the async saga in action)
  - Show result: ✅ Confirmed or ❌ Failed
- [ ] Create `pages/OrdersPage.tsx`:
  - List of user's orders (paginated)
  - Status badges (PENDING=orange, CONFIRMED=green, CANCELLED=red, SHIPPED=blue, DELIVERED=purple)
- [ ] Test full flow: browse → add to cart → checkout → place order → see confirmation

**Deliverable:** Complete checkout flow and order tracking. **The full user journey works end-to-end!**

---

#### Day 47 (3h) — Frontend Polish & Nginx Static Serving

**Do:**
- [ ] Add error handling to all API calls (toast notifications)
- [ ] Add loading states: skeleton loaders, spinners
- [ ] Make responsive (Ant Design Grid system)
- [ ] Create `frontend/Dockerfile` (Node build → Nginx serve)
- [ ] Update main Nginx config: serve frontend at `/`, proxy API at `/api/`
- [ ] Add frontend to `docker-compose.yml`
- [ ] Test: `http://localhost` → React app loads → full flow works

**Deliverable:** Polished frontend served via Nginx. Full application accessible at `http://localhost`.

---

#### Day 48 (3h) — Performance Profiling: Go pprof & Java JFR

**Learn:**
- **Go `pprof` analysis: CPU flame graphs, memory allocation, goroutine leaks**
- **Java Flight Recorder (JFR): thread contention, GC pauses, method profiling**
- **Interpreting flame graphs** — wide bars = hot spots
- **Connecting profiling to optimization** — profile first, optimize second

**Do:**
- [ ] **Go profiling exercise**:
  1. Start all services with `docker-compose up`
  2. Run a simple load: `for i in {1..1000}; do curl http://localhost/api/v1/products; done`
  3. Capture CPU profile: `go tool pprof http://localhost:6062/debug/pprof/profile?seconds=30`
  4. Generate flame graph: `go tool pprof -http=:8080 cpu.prof`
  5. Capture heap profile: check for memory leaks
  6. Capture goroutine dump: verify no goroutine leaks
  7. Document findings in `docs/profiling-results.md`
- [ ] **Java profiling exercise**:
  1. Enable JFR: add `-XX:StartFlightRecording=duration=60s,filename=recording.jfr` to Dockerfile
  2. Run load test against Product Service
  3. Analyze with `jfr` CLI or JDK Mission Control:
     - Check GC pauses (target: < 50ms)
     - Check thread contention (lock waits)
     - Check method profiling (slow methods)
  4. Document findings
- [ ] **Identify top 3 hotspots** and decide if optimization is needed
- [ ] **Create `docs/profiling-results.md`** with flame graphs and analysis
- [ ] Update Makefile: `make profile-go`, `make profile-java` targets

**Deliverable:** Profiling data captured for Go and Java services. Hotspots identified. Profiling guide documented.

---

### Week 8 Checkpoint ✅

- [ ] Login/Register page
- [ ] Product listing with search, filter, pagination
- [ ] Shopping cart with quantity editing
- [ ] Checkout flow with order confirmation
- [ ] Order history page
- [ ] Frontend served via Nginx at `http://localhost`
- [ ] Full user journey works in browser
- [ ] **Go pprof profiling completed — flame graphs captured**
- [ ] **Java JFR profiling completed — GC and thread contention analyzed**
- [ ] **Profiling results documented**

---

## Phase 5: Quality

### Week 9 — Testing, Security & Performance

**Goal:** Harden the system. Add integration tests, security headers, run load tests, **and stress-test every race condition identified in the proposal**.

---

#### Day 49 (3h) — Cross-Service Integration Tests

**Do:**
- [ ] Write E2E test script (shell or Postman/Newman):
  - Happy path: register → login → browse → cart → checkout → order confirmed
  - Failure path: order with insufficient stock → 409
  - Failure path: payment fails → order cancelled → stock released
  - Failure path: rate limit exceeded → 429
  - Auth: expired JWT → 401, blacklisted JWT → 401
- [ ] Automate with Newman: `newman run api/postman/e2e-tests.json`
- [ ] Fix any bugs discovered

**Deliverable:** Automated E2E test suite that proves the system works.

---

#### Day 50 (3h) — Security Hardening

**Do:**
- [ ] Add security headers to Nginx (X-Content-Type-Options, X-Frame-Options, CSP, HSTS)
- [ ] Restrict CORS to specific origins
- [ ] Add input sanitization and request size limits
- [ ] Verify no SQL injection possible
- [ ] Run `govulncheck` on Go services
- [ ] Run OWASP Dependency-Check on Java services
- [ ] Review: no secrets in code

**Deliverable:** Security-hardened application with no critical vulnerabilities.

---

#### Day 51 (3h) — Load Testing with k6

**Learn:**
- k6 script structure: `options` (VUs, duration), `default function` (test scenario)
- k6 thresholds: `http_req_duration['p(95)'] < 500`
- k6 scenarios: ramping VUs, constant rate

**Do:**
- [ ] Write `script/loadtest/product-listing.js`: 50 VUs, 3 min, p95 < 500ms
- [ ] Write `script/loadtest/checkout-flow.js`: 20 VUs, 3 min, p95 < 2s
- [ ] **Write `script/loadtest/concurrent-stock.js`**: 100 VUs, 1 min
  - All reserving the same product simultaneously
  - **Verify: no overselling (total reserved ≤ total stock)**
- [ ] **Write `script/loadtest/concurrent-payment.js`**: 50 VUs, 1 min
  - **Duplicate idempotency keys → verify exactly 1 payment created**
- [ ] Run tests, record results in `docs/load-test-results.md`
- [ ] If p95 > target: identify bottleneck and optimize

**Deliverable:** Load test results proving performance targets and concurrency safety.

---

#### Day 52 (3h) — Performance Optimization (Based on Profiling)

**Do (based on pprof/JFR results from Day 48 and load test results from Day 51):**
- [ ] Database optimization:
  - Run `EXPLAIN ANALYZE` on slowest queries
  - Add missing indexes if needed
  - Optimize N+1 queries (add `JOIN FETCH` or `@EntityGraph`)
  - **Add partial indexes for active products** (proposal §11.4)
- [ ] **Tune connection pools** based on observed concurrency (proposal §5.3)
- [ ] Enable Nginx gzip compression for JSON responses
- [ ] Verify Redis cache hit rates (`redis-cli info stats | grep hit`)
- [ ] **Go: check sync.Pool effectiveness** (compare allocs/op before/after)
- [ ] **Java: check GC pause times** (tune ZGC if needed)
- [ ] Re-run load tests to verify improvements
- [ ] Document before/after metrics

**Deliverable:** Optimized system meeting all performance SLOs. Before/after comparison.

---

#### Day 53 (3h) — Concurrency Stress Testing & Code Quality

**Do:**
- [ ] **Run comprehensive concurrency stress tests**:
  - [ ] Stock: 200 threads/goroutines reserving 100 stock → exactly 100 succeed
  - [ ] Cart: 50 goroutines updating same cart → no lost updates
  - [ ] Payment: 100 goroutines with same idempotency key → exactly 1 payment
  - [ ] Order: simultaneous confirm + cancel → one wins cleanly
  - [ ] Login: 20 goroutines attempting login → lockout counter is accurate
- [ ] **Run `go test -race -count=5` across all Go services** — race detector with multiple runs
- [ ] Increase unit test coverage to 75%+ on critical paths
- [ ] Run linters: `golangci-lint run`, `mvn checkstyle:check`
- [ ] Review all TODO comments → resolve or document

**Deliverable:** All race conditions proven safe. Clean code, high coverage, no linter warnings.

---

#### Day 54 (3h) — Error Scenarios & Chaos Testing

**Do:**
- [ ] Test service failure scenarios:
  - Kill Product Service → Cart circuit breaker opens → returns cached data
  - Kill Payment Service → orders stay PENDING → recover when service restarts
  - Kill PostgreSQL → services report unhealthy → restart → reconnect
  - Kill Redis → services degrade gracefully
  - Kill Kafka → async events queue → process when recovered
- [ ] **Verify graceful shutdown**: send requests → stop service → in-flight requests complete (not dropped)
- [ ] Verify DLQ: force payment failure → message in `payments.dlq` → replay → processed
- [ ] Document all resilience behaviors in `docs/architecture.md`

**Deliverable:** Proven fault tolerance — system degrades gracefully, recovers automatically.

---

### Week 9 Checkpoint ✅

- [ ] Automated E2E test suite
- [ ] Security headers and hardening
- [ ] **Load test results with concurrency scenarios documented**
- [ ] **Performance optimized based on pprof/JFR profiling data**
- [ ] 75%+ test coverage on critical paths
- [ ] **All race conditions proven safe under concurrent stress**
- [ ] `go test -race` clean across all Go services
- [ ] Chaos testing: fault tolerance verified
- [ ] Clean code: no linter warnings

---

## Phase 6: Ship

### Week 10 — Documentation, CI/CD & Demo Prep

**Goal:** Make the project interview-ready. Professional README, CI/CD pipeline, architecture docs, and a polished demo.

---

#### Day 55 (3h) — README & Architecture Documentation

**Do:**
- [ ] Write `README.md` — the first thing interviewers see:
  ```markdown
  # 🛒 High-Throughput E-Commerce Platform
  
  A distributed e-commerce system built with **5 microservices**, 
  event-driven architecture, and polyglot tech stack (Go + Java).
  
  ## Key Engineering Challenges Solved
  - ⚡ Stock race conditions → @Version optimistic locking + retry
  - 💳 Double payments → Idempotency keys + DB unique constraints
  - 📦 Distributed transactions → Kafka choreography saga
  - 🔄 Concurrent cart updates → Redis WATCH/MULTI/EXEC
  - 🚀 10K concurrent users → Go goroutines + Java virtual threads
  
  ## Architecture
  [System diagram]
  
  ## Cloud Deployment
  - AWS Free Tier deployment guide
  - GCP Cloud Run deployment guide
  ```
- [ ] Write `docs/architecture.md`: detailed architecture overview with diagrams
- [ ] Write `docs/data-flows.md`: sequence diagrams for key flows (order saga, auth)
- [ ] Write `docs/use-case.md`: user stories mapped to API endpoints

**Deliverable:** Professional documentation that highlights engineering depth, not just CRUD.

---

#### Day 56 (3h) — GitHub Actions CI Pipeline

**Do:**
- [ ] Create `.github/workflows/ci.yml`:
  - Go: lint + test + race detector + coverage
  - Java: checkstyle + test + coverage
  - Docker: build all images
- [ ] Push → verify CI passes → green badge
- [ ] Add CI badge to README

**Deliverable:** CI pipeline running on every push. Green badge on README.

---

#### Day 57 (3h) — OpenAPI Spec & Swagger UI

**Do:**
- [ ] Complete `api/openapi.yaml` with all endpoints from all 5 services
- [ ] Add request/response examples and auth scheme documentation
- [ ] Set up Swagger UI served via Nginx at `/swagger`
- [ ] Add link to Swagger UI in README

**Deliverable:** Interactive API documentation accessible via browser.

---

#### Day 58 (3h) — Demo Script & Walkthrough

**Do:**
- [ ] Write `docs/demo-script.md` — step-by-step demo for interviews:
  1. Show architecture diagram (explain language choices)
  2. `docker-compose up` — show all 10 containers starting
  3. Open browser → login → browse → add to cart → checkout
  4. Show Kafka events in logs (the saga in action)
  5. **Show concurrent stock reservation: 100 requests → exactly correct number succeed**
  6. Show Redis caching: first product load vs second (cached)
  7. Show circuit breaker: kill product service → cart still works
  8. Show rate limiting: burst requests → 429 responses
  9. **Show pprof flame graph and profiling data**
  10. Walk through code: saga pattern, idempotency, optimistic locking, worker pool
- [ ] Practice the demo — time it to 10 minutes
- [ ] Prepare answers for interview questions:
  - "Why Go for some services and Java for others?"
  - "How do you prevent stock overselling under concurrent load?"
  - "What happens if a payment is processed twice?"
  - "How does the saga pattern handle failures?"
  - "How would you deploy this to production?"

**Deliverable:** Polished demo script. Prepared interview answers focused on concurrency and performance.

---

#### Day 59 (3h) — Final Polish & Code Review

**Do:**
- [ ] Self code review: read every file, fix inconsistencies
- [ ] Ensure consistent naming: camelCase (Go), snake_case (DB), PascalCase (Java)
- [ ] Remove all debug logs, unused imports, TODO comments
- [ ] Verify `.gitignore` is comprehensive
- [ ] Verify all tests pass: `make test-go && make test-java`
- [ ] Verify `docker-compose up --build` from clean state
- [ ] Verify frontend works end-to-end in browser

**Deliverable:** Production-quality codebase. Clean git history.

---

#### Day 60 (3h) — Final Testing & Ship 🚀

**Do:**
- [ ] Fresh clone test: `git clone ... && docker-compose up --build` → everything works
- [ ] Run full E2E test suite one final time
- [ ] **Run all concurrency stress tests one final time**
- [ ] Run load tests one final time → document final metrics
- [ ] Update README with final metrics (test coverage, load test results, profiling data)
- [ ] Create a GitHub Release (`v1.0.0`) with changelog
- [ ] Pin the repository (make it prominent on your GitHub profile)
- [ ] **Optional: Deploy to GCP Cloud Run** (most impressive for demo)
- [ ] **Optional: Write LinkedIn article** — "I built an event-driven e-commerce platform solving real concurrency challenges"

**Deliverable:** 🎉 **Project shipped!** Ready for internship applications.

---

### Week 10 Checkpoint ✅

- [ ] Professional README highlighting engineering challenges (not just CRUD)
- [ ] CI/CD pipeline (GitHub Actions, green badge)
- [ ] Interactive API docs (Swagger UI)
- [ ] Architecture & data flow documentation
- [ ] **Cloud deployment guides (AWS & GCP)**
- [ ] 10-minute demo script (practiced)
- [ ] Interview Q&A prepared (focus on concurrency, performance, language choices)
- [ ] Clean codebase (no warnings, no TODOs)
- [ ] Fresh clone test passes
- [ ] GitHub Release v1.0.0

---

## Final Success Criteria

| Criteria | Target | How to Verify |
|---|---|---|
| Services running | 5/5 healthy | `docker-compose ps` |
| Full saga works | Order → Payment → Confirmation | E2E test |
| **Stock integrity** | **No overselling under 200 concurrent requests** | **Concurrency stress test** |
| **Payment safety** | **No double charge under 100 concurrent requests** | **Idempotency stress test** |
| **Cart consistency** | **No lost updates under 50 concurrent requests** | **Redis WATCH/MULTI test** |
| Auth secure | JWT + refresh + blacklist | E2E test |
| Cache effective | Redis hit rate > 70% | `redis-cli info stats` |
| Rate limiting | 429 on excess requests | Burst test |
| Circuit breaker | Fallback on service failure | Chaos test |
| Test coverage | > 70% on critical paths | Test reports |
| **Race detector** | **No races detected** | **`go test -race` all services** |
| Load performance | p95 < 500ms (read), p95 < 2s (write) | k6 results |
| **Profiling** | **No goroutine leaks, GC pause < 50ms** | **pprof, JFR** |
| Frontend works | Full checkout flow in browser | Manual test |
| **Cloud deployable** | **Runs on AWS or GCP** | **Deployment guide + test** |
| One-command start | `docker-compose up --build` | Fresh clone |
| Documentation | README + ADRs + API docs + deployment guides | Visual check |

---

## Tips for Success

1. **Commit daily.** Small, meaningful commits. Never lose more than 3 hours of work.
2. **Test as you go.** Don't leave testing for the end. Write tests while the logic is fresh.
3. **Run `go test -race` on every commit.** Catch data races early, not in production.
4. **Profile before optimizing.** Use pprof/JFR to find the actual bottleneck — don't guess.
5. **If stuck > 30 minutes, move on.** Mark it as a TODO, build something else, come back later.
6. **Docker Compose early.** Run services in containers from Week 2 onward. Catch integration issues early.
7. **Don't gold-plate.** Good enough > perfect. Ship a working system, then improve.
8. **Keep a journal.** Note what you learned each day. Great for interviews ("tell me about a challenge you faced").
9. **Focus on concurrency stories.** Interviewers care about race conditions, distributed transactions, and why you chose specific patterns — not CRUD endpoints.

---

*Total: 60 days · 180 hours · 1 complete distributed system · Race-condition free · Cloud deployable · Ready to impress.*
