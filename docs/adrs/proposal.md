# High-Throughput E-Commerce Platform — Technical Proposal

**Version:** 3.0  
**Date:** 2025  
**Author:** Hung  
**Status:** Approved

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Architecture Overview](#2-architecture-overview)
3. [Technology Stack & Language Advantages](#3-technology-stack--language-advantages)
4. [Service Specifications](#4-service-specifications)
5. [Database Design](#5-database-design)
6. [API Design](#6-api-design)
7. [Inter-Service Communication](#7-inter-service-communication)
8. [Caching Strategy](#8-caching-strategy)
9. [Security](#9-security)
10. [Concurrency & Race Conditions](#10-concurrency--race-conditions)
11. [Performance & Scalability](#11-performance--scalability)
12. [Resilience & Fault Tolerance](#12-resilience--fault-tolerance)
13. [Deployment & Infrastructure](#13-deployment--infrastructure)
14. [Testing Strategy](#14-testing-strategy)
15. [Monitoring & Observability](#15-monitoring--observability)
16. [Future Enhancements](#16-future-enhancements)

---

## 1. Executive Summary

This document proposes a **distributed e-commerce platform** built with 5 microservices, leveraging the concurrency strengths of **Go** and the enterprise transaction capabilities of **Java/Spring Boot**. The system demonstrates production-grade patterns: event-driven architecture (Kafka), optimistic/pessimistic locking for race condition prevention, idempotent payment processing, Redis-first cart design, and circuit breaker resilience.

**Why this matters for an internship application:**
- Demonstrates understanding of **distributed systems** — not just CRUD
- Shows mastery of **concurrency control** — goroutines, channels, `@Transactional` isolation levels, optimistic locking
- Proves ability to choose the **right language for the right problem** — Go for I/O-heavy concurrent services, Java for complex business logic
- Includes **cloud-ready deployment** — AWS and GCP deployment scenarios

**Architecture at a glance:** 5 microservices, Nginx reverse proxy, PostgreSQL, Redis, Kafka, React frontend — all containerized with Docker Compose. Cloud-deployable to AWS (EC2 + managed services) or GCP (Cloud Run serverless).

---

## 2. Architecture Overview

### 2.1 System Architecture Diagram

```
                                    ┌─────────────────────────────┐
                                    │       React Frontend        │
                                    │     (Vite + TypeScript)     │
                                    └────────────┬────────────────┘
                                                 │ HTTP
                                    ┌────────────▼────────────────┐
                                    │      Nginx Reverse Proxy    │
                                    │   Rate Limiting · CORS · TLS│
                                    └────────────┬────────────────┘
                          ┌──────────┬───────────┼───────────┬──────────┐
                          │          │           │           │          │
                   ┌──────▼──────┐ ┌─▼─────────┐ ┌▼────────┐ ┌▼────────┐ ┌▼────────────┐
                   │   User Svc  │ │Product Svc │ │Cart Svc │ │Order Svc│ │Payment Svc  │
                   │   (Golang)  │ │(Java/Boot) │ │(Golang) │ │(Java)   │ │  (Golang)   │
                   │   :8001     │ │  :8081     │ │ :8002   │ │ :8082   │ │   :8003     │
                   │             │ │+Inventory  │ │         │ │+Notify  │ │             │
                   │ goroutines  │ │@Version    │ │Redis 1st│ │Saga     │ │ idempotent  │
                   │ channels    │ │@Transact.  │ │goroutin │ │Kafka    │ │ worker pool │
                   └──────┬──────┘ └─┬─────────┘ └┬───┬────┘ └┬───┬───┘ └┬────────┬───┘
                          │          │             │   │       │   │      │        │
                   ┌──────▼──────────▼─────────────▼───┘       │   │      │        │
                   │          PostgreSQL 15+                    │   │      │        │
                   │  (5 logical DBs, connection pooling)       │   │      │        │
                   └───────────────────────────────────────────┘   │      │        │
                                                                   │      │        │
                   ┌───────────────────────────────────────────────▼──────▼────────┘
                   │                Apache Kafka 3.x                              │
                   │  orders.created → payments.completed/failed → order confirm  │
                   │           (Choreography Saga with DLQ)                       │
                   └──────────────────────────────────────────────────────────────┘
                   ┌──────────────────────────────────────────────────────────────┐
                   │                    Redis 7+                                  │
                   │  Sessions · Cart (primary) · Cache · Blacklist · Rate Limit  │
                   └──────────────────────────────────────────────────────────────┘
```

### 2.2 Service Decomposition

| Service | Language | Port | Bounded Context |
|---|---|---|---|
| User Service | Golang | 8001 | Authentication, Authorization, User profiles |
| Product Service | Java/Spring Boot | 8081 | Product catalog, Search, **Inventory & stock management** |
| Cart Service | Golang | 8002 | Shopping cart lifecycle, Redis-backed storage |
| Order Service | Java/Spring Boot | 8082 | Order lifecycle, State machine, Kafka events, **inline notifications** |
| Payment Service | Golang | 8003 | Payment processing, Idempotency, Refunds |

**Nginx** acts as the reverse proxy (not a custom service) — handles routing, rate limiting, CORS, and TLS termination via configuration only.

### 2.3 Communication Patterns

- **Synchronous (REST/HTTP):** Client requests routed through Nginx; inter-service queries (e.g., Cart → Product for price/stock validation).
- **Asynchronous (Kafka):** Event-driven workflows for order→payment→confirmation saga.

---

## 3. Technology Stack & Language Advantages

### 3.1 Languages & Frameworks

| Technology | Version | Purpose | Justification |
|---|---|---|---|
| **Golang** | 1.21+ | User, Cart, Payment services | High concurrency via goroutines, low memory footprint, fast compilation |
| **Java** | 17/21 LTS | Product, Order services | Rich ecosystem (Spring Boot), strong ORM (JPA/Hibernate), mature transaction support |
| **Gin** | v1.9+ | Go HTTP framework | Lightweight, high-performance router with middleware support |
| **Spring Boot** | 3.x | Java framework | Convention-over-configuration, dependency injection, production-ready features |
| **GORM** | v2 | Go ORM | Auto-migration, associations, hooks, PostgreSQL dialect |
| **Spring Data JPA** | 3.x | Java ORM | Repository abstraction, query derivation, auditing |

### 3.2 Why Go — Concurrency Without Complexity

Go is chosen for **User, Cart, and Payment** services because these services are **I/O-bound and need massive concurrency** with minimal resource overhead:

| Go Advantage | How We Use It | Alternative Cost |
|---|---|---|
| **Goroutines** (~2KB stack each) | Handle 10,000+ concurrent HTTP requests per service | Java threads: ~1MB stack each, 10,000 threads = 10GB RAM |
| **Channels** (CSP model) | Kafka consumer worker pool in Payment Service — fan-out events to N goroutine workers | Java: `BlockingQueue` + `ExecutorService` — more boilerplate |
| **`context.Context`** | Propagate request deadlines/cancellation across goroutines, HTTP calls, and DB queries | Java: manual `Future.cancel()` or reactive chains |
| **`sync.Pool`** | Reuse JSON encoder/decoder buffers to reduce GC pressure in Cart Service | Java: thread-local allocation or object pool libraries |
| **`sync.Mutex` / `sync.RWMutex`** | Protect shared in-memory state (e.g., circuit breaker counters) | Java: `synchronized` or `ReentrantLock` — functionally equivalent |
| **`errgroup`** | Parallel HTTP calls (e.g., validate multiple cart items simultaneously) | Java: `CompletableFuture.allOf()` — similar but more verbose |
| **Built-in race detector** | `go test -race` catches data races during development | Java: requires external tools (ThreadSanitizer, FindBugs) |
| **Static binary (~15MB)** | Docker images are tiny, fast to deploy, minimal attack surface | Java: JRE adds ~200MB to image |
| **`pprof`** | Built-in CPU/memory/goroutine profiling — zero external dependencies | Java: requires JFR/VisualVM setup |

**Go Concurrency Model in This Project:**
```go
// Example: Cart Service validates all items in parallel using errgroup
func (s *CartService) ValidateCartItems(ctx context.Context, items []CartItem) error {
    g, ctx := errgroup.WithContext(ctx)
    
    for _, item := range items {
        item := item // capture loop variable
        g.Go(func() error {
            // Each validation runs in its own goroutine
            // If ctx is cancelled (e.g., one item fails), others stop early
            product, err := s.productClient.GetProduct(ctx, item.ProductID)
            if err != nil {
                return fmt.Errorf("product %d: %w", item.ProductID, err)
            }
            if product.AvailableStock < item.Quantity {
                return fmt.Errorf("product %d: insufficient stock", item.ProductID)
            }
            return nil
        })
    }
    
    return g.Wait() // Returns first error, cancels remaining goroutines
}
```

### 3.3 Why Java — Transactions and Business Logic

Java/Spring Boot is chosen for **Product and Order** services because these services have **complex business logic, transactional requirements, and need strong type safety:**

| Java Advantage | How We Use It | Go Alternative Cost |
|---|---|---|
| **`@Transactional`** with isolation levels | Order creation: reserve stock → save order → publish event, all in one transaction | Go: manual `tx.Begin()` / `tx.Commit()` / `tx.Rollback()` — easy to forget rollback |
| **`@Version` optimistic locking** | Prevent stock overselling — JPA automatically checks version on save | Go: manual `WHERE version = ?` clauses and retry loops |
| **JPA `@EntityGraph` / `JOIN FETCH`** | Eliminate N+1 queries when loading orders with items | Go/GORM: `Preload()` works but less flexible for complex graphs |
| **Spring `@Async` + `CompletableFuture`** | Fire-and-forget notifications without blocking the order flow | Go: goroutines (actually simpler here — Go wins on this point) |
| **Resilience4j annotations** | `@CircuitBreaker`, `@Retry`, `@Bulkhead` as declarative annotations | Go: `gobreaker` works but requires explicit wrapping |
| **Bean Validation (`@Valid`)** | Declarative input validation with auto-error messages | Go: struct tags work but need manual error message formatting |
| **Flyway** | Version-controlled schema migrations with rollback | Go: `golang-migrate` exists but less mature |
| **Virtual Threads (Java 21)** | Lightweight threads for Kafka consumers — block without wasting OS threads | Go: goroutines already lightweight — not needed |
| **TestContainers** | Spin up real PostgreSQL/Redis/Kafka in tests | Go: `testcontainers-go` exists but Java's version is more mature |
| **Spring Profiles** | `dev` / `staging` / `prod` config switching | Go: manual env-var-based config |

**Java Concurrency Model in This Project:**
```java
// Example: Order Service creates order with transactional stock reservation
@Transactional(isolation = Isolation.READ_COMMITTED)
public Order createOrder(CreateOrderRequest request) {
    // 1. Everything inside this method is ONE transaction
    // 2. If any step throws, ALL changes roll back automatically
    
    Cart cart = cartClient.getCart(request.getUserId());
    
    // Reserve stock for all items (calls Product Service)
    for (OrderItem item : cart.getItems()) {
        inventoryClient.reserveStock(item.getProductId(), item.getQuantity());
    }
    // If reserveStock throws for any item, the transaction rolls back
    // AND previously reserved items are compensated in the catch block
    
    Order order = Order.builder()
        .userId(request.getUserId())
        .status(OrderStatus.PENDING)
        .totalAmount(cart.getTotalAmount())
        .build();
    
    order = orderRepository.save(order); // JPA flush within transaction
    
    // Publish Kafka event (with transactional outbox pattern ideally)
    kafkaTemplate.send("orders.created", order.getId().toString(), 
        OrderCreatedEvent.from(order));
    
    // Send notification asynchronously (does NOT block the response)
    notificationDispatcher.sendOrderCreated(order); // @Async
    
    return order;
}
```

### 3.4 Data Stores

| Technology | Version | Purpose |
|---|---|---|
| **PostgreSQL** | 15+ | Primary relational database (1 instance, 5 logical databases) |
| **Redis** | 7+ | Session caching, cart storage, token blacklist, product caching |

### 3.5 Messaging & Infrastructure

| Technology | Version | Purpose |
|---|---|---|
| **Apache Kafka** | 3.x | Asynchronous event streaming between Order ↔ Payment |
| **Nginx** | 1.25+ | Reverse proxy, rate limiting, CORS, static file serving |
| **Docker** | 24+ | Containerization of all services |
| **Docker Compose** | 2.x | Local development orchestration |

### 3.6 Supporting Libraries

| Library | Language | Purpose |
|---|---|---|
| `golang-jwt/jwt/v5` | Go | JWT generation and validation |
| `golang.org/x/crypto` | Go | bcrypt password hashing |
| `go-redis/redis/v9` | Go | Redis client |
| `confluent-kafka-go` | Go | Kafka producer/consumer |
| `gobreaker` | Go | Circuit breaker implementation |
| `testify` | Go | Testing assertions and mocks |
| `errgroup` | Go | Parallel goroutine coordination with error handling |
| `pprof` | Go | CPU/memory/goroutine profiling |
| Spring Security | Java | Authentication and authorization |
| Spring Kafka | Java | Kafka integration |
| Resilience4j | Java | Circuit breaker, retry, bulkhead |
| Mockito + JUnit 5 | Java | Unit test mocking |
| TestContainers | Java | Integration test databases |
| Lombok | Java | Boilerplate reduction |
| Flyway | Java | Database migration versioning |

---

## 4. Service Specifications

### 4.1 Nginx Reverse Proxy (Configuration Only)

**Responsibility:** Single entry point for all client requests. Not a custom service — pure configuration.

**Capabilities:**
- Path-based request routing to backend services
- Rate limiting (100 requests/min per IP via `limit_req_zone`)
- CORS headers (`Access-Control-Allow-Origin`, etc.)
- Request/response logging (access log with timing)
- Static file serving (Swagger UI)
- Health check endpoint aggregation

**Routing Configuration:**

| Path Prefix | Target Upstream |
|---|---|
| `/api/v1/auth/*` | `http://user-service:8001` |
| `/api/v1/users/*` | `http://user-service:8001` |
| `/api/v1/products/*` | `http://product-service:8081` |
| `/api/v1/inventory/*` | `http://product-service:8081` |
| `/api/v1/carts/*` | `http://cart-service:8002` |
| `/api/v1/orders/*` | `http://order-service:8082` |
| `/api/v1/payments/*` | `http://payment-service:8003` |
| `/swagger/*` | Static files |

**Why Nginx instead of a custom gateway:**
- Zero application code to maintain
- Proven at scale (serves ~30% of all websites)
- Built-in rate limiting, caching, gzip, TLS
- JWT validation can be done per-service (more secure — defense in depth)

---

### 4.2 User Service (Golang)

**Responsibility:** User identity lifecycle — registration, authentication, authorization, and profile management.

**Why Go for Auth:** The auth service is I/O-bound (bcrypt hash, Redis lookup, DB query) and handles every authenticated request in the system. Go's goroutine-per-request model handles 10K+ concurrent logins with <50MB RAM.

**Key Go Concurrency Patterns Used:**
- **Goroutine-per-request**: Gin spawns a goroutine for each HTTP request — handles thousands concurrently
- **`context.Context` propagation**: Request timeout (5s) propagated through handler → service → repository → Redis, ensuring no goroutine leak on slow DB
- **`sync.RWMutex`** on JWT public key cache: multiple goroutines read the cached key concurrently; write lock only on key rotation
- **Channel-based rate limiter**: Token bucket implemented with `time.Ticker` channel for login attempt throttling

**Data Model:**

| Table | Key Columns |
|---|---|
| `users` | id (UUID), email (unique), password_hash, role, is_locked, failed_login_attempts, created_at, updated_at |
| `user_profiles` | id, user_id (FK), first_name, last_name, phone, avatar_url |
| `user_addresses` | id, user_id (FK), street, city, state, zip, country, is_default |
| `auth_tokens` | id, user_id (FK), refresh_token_hash, expires_at, revoked, created_at |

**API Endpoints:**

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | Public | Register new user account |
| POST | `/api/v1/auth/login` | Public | Authenticate and return JWT + refresh token |
| POST | `/api/v1/auth/refresh` | Public | Issue new JWT from refresh token |
| POST | `/api/v1/auth/logout` | Bearer | Revoke tokens, add JWT to Redis blacklist |
| GET | `/api/v1/users/profile` | Bearer | Get current user profile + addresses |
| PUT | `/api/v1/users/profile` | Bearer | Update user profile |
| GET | `/api/v1/users/{id}` | Internal | Get user by ID (service-to-service only) |

**Authentication Flow:**
1. User submits email + password → bcrypt hash comparison (cost factor 12)
2. Check `is_locked` flag and `failed_login_attempts` (lockout after 5 consecutive failures)
3. On success → reset `failed_login_attempts`, generate:
   - **Access token:** JWT (RS256), 15-minute TTL, claims: `{userId, email, role}`
   - **Refresh token:** Opaque random string, 7-day TTL, stored hashed in `auth_tokens`
4. On failure → increment `failed_login_attempts`, lock account if threshold exceeded
5. Logout → add JWT `jti` to Redis blacklist with TTL matching token remaining lifetime

**Race Condition: Concurrent Login Attempts**
```go
// Problem: Two goroutines reading failed_login_attempts simultaneously
// could both see "4 attempts" and both allow login, skipping lockout.

// Solution: Use SELECT ... FOR UPDATE in the login transaction
func (r *UserRepository) FindByEmailForUpdate(ctx context.Context, email string) (*User, error) {
    var user User
    err := r.db.WithContext(ctx).
        Clauses(clause.Locking{Strength: "UPDATE"}).  // Pessimistic lock
        Where("email = ?", email).
        First(&user).Error
    return &user, err
}

// This ensures only one goroutine can read+update the login attempt counter at a time
```

**JWT Middleware (shared across Go services):**
- Extracts Bearer token from `Authorization` header
- Validates RS256 signature against public key
- Checks `jti` against Redis blacklist
- Injects `userId` and `role` into request context
- Returns 401 on invalid/expired/blacklisted token

**Redis Usage:**
- `session:{userId}` — cached user profile (30 min TTL)
- `blacklist:{jti}` — revoked JWT IDs (TTL matches token expiry)
- `login_attempts:{email}` — failed login counter (15 min sliding window)

**Project Structure:**
```
user-service/
├── cmd/server/main.go
├── internal/
│   ├── handler/        # HTTP handlers (thin — delegates to service)
│   ├── service/        # Business logic
│   ├── repository/     # Database access (GORM)
│   ├── model/          # Domain entities
│   ├── dto/            # Request/response DTOs
│   └── middleware/      # JWT auth, logging, recovery
├── pkg/
│   ├── jwt/            # Token generation/validation (reusable)
│   ├── password/       # bcrypt helpers (reusable)
│   └── response/       # Standard API response helpers
├── config/config.go
├── Dockerfile
├── go.mod
└── go.sum
```

---

### 4.3 Product Service (Java/Spring Boot) — Includes Inventory

**Responsibility:** Product catalog management, search, category browsing, **and stock/inventory management**.

Merging inventory into Product Service eliminates an inter-service call on every checkout and keeps stock as a first-class attribute of a product.

**Why Java for Product+Inventory:** This service has the most complex data model (categories, images, stock movements, full-text search) and the highest need for **transactional integrity** (stock reservation must be atomic). JPA's `@Version` + `@Transactional` makes concurrency-safe stock operations declarative.

**Key Java Concurrency Patterns Used:**
- **`@Version` optimistic locking**: Prevents stock overselling without database-level locks — the JPA provider automatically adds `WHERE version = ?` to UPDATE statements
- **`@Transactional(isolation = Isolation.READ_COMMITTED)`**: Default isolation level — prevents dirty reads while allowing concurrent product reads
- **`@Retry` (Resilience4j)**: Automatic retry on `OptimisticLockException` — handles concurrent stock reservation gracefully
- **`@Async` + `CompletableFuture`**: Cache warming on startup runs in a background thread, doesn't block the application boot
- **`@Cacheable` / `@CacheEvict`**: Thread-safe Redis cache operations via Spring's cache abstraction

**Data Model:**

| Table | Key Columns |
|---|---|
| `products` | id, name, description, price (DECIMAL(10,2)), category_id (FK), seller_id, status (ACTIVE/INACTIVE/DELETED), stock_quantity, stock_reserved, version (optimistic lock), created_at, updated_at |
| `categories` | id, name, slug, parent_id (self-referencing for hierarchy), sort_order |
| `product_images` | id, product_id (FK), url, alt_text, sort_order |
| `stock_movements` | id, product_id (FK), type (IN/OUT/RESERVE/RELEASE), quantity, reference_id, reason, created_at |

**Indexes:**
- `idx_products_category_id` on `products(category_id)` — category filtering
- `idx_products_seller_id` on `products(seller_id)` — seller dashboard
- `idx_products_created_at` on `products(created_at DESC)` — recency sorting
- `idx_products_status` on `products(status)` — active product filtering
- GIN full-text index on `products(name, description)` — search queries
- `idx_stock_movements_product_id` on `stock_movements(product_id)` — audit trail

**API Endpoints — Product:**

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/products` | Seller/Admin | Create product with initial stock |
| GET | `/api/v1/products/{id}` | Public | Get product by ID (includes stock availability) |
| PUT | `/api/v1/products/{id}` | Seller/Admin | Update product details |
| DELETE | `/api/v1/products/{id}` | Admin | Soft-delete product (set status=DELETED) |
| GET | `/api/v1/products` | Public | List products with pagination, filters, sorting |
| GET | `/api/v1/products/search?q={query}` | Public | Full-text search with relevance ranking |
| GET | `/api/v1/categories` | Public | List all categories (tree structure) |
| GET | `/api/v1/categories/{id}/products` | Public | List products in a category |

**API Endpoints — Inventory (within Product Service):**

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/inventory/{productId}` | Internal | Check available stock (quantity - reserved) |
| POST | `/api/v1/inventory/{productId}/reserve` | Internal | Reserve stock for order (atomic) |
| POST | `/api/v1/inventory/{productId}/release` | Internal | Release reserved stock (on cancellation) |
| POST | `/api/v1/inventory/{productId}/confirm` | Internal | Deduct reserved stock (order confirmed) |
| PUT | `/api/v1/inventory/{productId}` | Admin | Manually adjust stock levels |
| GET | `/api/v1/inventory/{productId}/movements` | Admin | View stock movement audit trail |

**Concurrency Control for Stock — Deep Dive:**
```java
// OPTIMISTIC LOCKING — Primary Strategy
// How it works: @Version column is auto-incremented on every UPDATE.
// If two transactions read version=5, the first to save sets version=6.
// The second transaction's save FAILS because its WHERE version=5 matches nothing.

@Entity
public class Product {
    @Id @GeneratedValue
    private Long id;
    
    private int stockQuantity;
    private int stockReserved;
    
    @Version  // JPA auto-manages this — incremented on every save
    private Long version;
    
    public int getAvailableStock() {
        return stockQuantity - stockReserved;
    }
}

@Service
public class InventoryService {
    
    @Transactional
    @Retry(name = "stockRetry", maxAttempts = 3, 
           retryExceptions = OptimisticLockException.class)
    public StockReservation reserveStock(Long productId, int quantity, String orderId) {
        // 1. Load product (reads current version from DB)
        Product product = productRepository.findById(productId)
            .orElseThrow(() -> new ProductNotFoundException(productId));
        
        // 2. Business validation
        if (product.getAvailableStock() < quantity) {
            throw new InsufficientStockException(productId, quantity, 
                product.getAvailableStock());
        }
        
        // 3. Modify state
        product.setStockReserved(product.getStockReserved() + quantity);
        
        // 4. Save — JPA generates:
        //    UPDATE products SET stock_reserved=?, version=version+1 
        //    WHERE id=? AND version=?
        //    If version changed → OptimisticLockException → @Retry catches it
        productRepository.save(product);
        
        // 5. Audit trail
        StockMovement movement = StockMovement.builder()
            .productId(productId)
            .type(MovementType.RESERVE)
            .quantity(quantity)
            .referenceId(orderId)
            .build();
        stockMovementRepository.save(movement);
        
        return new StockReservation(productId, quantity, orderId);
    }
}

// WHAT HAPPENS UNDER CONTENTION:
// 100 users buy the last 10 items simultaneously:
// 1. All 100 goroutines/threads read the product (version=42, stock=10)
// 2. First 10 to reach save() succeed (version increments 42→43→44...→52)
// 3. Remaining 90 get OptimisticLockException
// 4. @Retry retries them — they re-read, see stock=0, throw InsufficientStockException
// 5. Result: exactly 10 reservations, 90 clear error responses — NO OVERSELLING
```

**When to use Pessimistic Locking instead:**
```java
// Use pessimistic locking when contention is VERY HIGH and retries are expensive
// Example: flash sale where 10,000 users hit the same product simultaneously

@Query("SELECT p FROM Product p WHERE p.id = :id")
@Lock(LockModeType.PESSIMISTIC_WRITE)  // SELECT ... FOR UPDATE
Product findByIdWithPessimisticLock(@Param("id") Long id);

// Trade-off:
// ✅ No retries needed — transactions queue up
// ❌ Reduced throughput — only one transaction at a time per product
// ❌ Risk of deadlocks if locking multiple products
// 
// Our default: OPTIMISTIC (good for normal traffic)
// Switch to PESSIMISTIC for flash-sale endpoints only
```

**Caching (Redis):**
- `@Cacheable("product:{id}")` on `getProduct(id)` — 10 min TTL
- `@CacheEvict("product:{id}")` on update/delete
- Cache warming on startup: pre-load top-50 products by view count
- Category list cached for 30 minutes (rarely changes)

**Project Structure:**
```
product-service/
├── src/main/java/com/ecommerce/product/
│   ├── controller/          # REST controllers
│   ├── service/             # Business logic
│   │   ├── ProductService.java
│   │   └── InventoryService.java
│   ├── repository/          # Spring Data JPA repositories
│   ├── model/               # JPA entities
│   ├── dto/                 # Request/response DTOs
│   ├── exception/           # Custom exceptions + global handler
│   ├── config/              # Redis, Kafka, security config
│   └── ProductServiceApplication.java
├── src/main/resources/
│   ├── application.yml
│   └── db/migration/       # Flyway SQL migrations
├── src/test/
├── pom.xml
└── Dockerfile
```

---

### 4.4 Cart Service (Golang)

**Responsibility:** Shopping cart lifecycle with Redis-backed fast access and PostgreSQL durability.

**Why Go for Cart:** The cart is the most latency-sensitive service (users expect instant responses). Redis operations are I/O-bound — Go's goroutines handle thousands of concurrent cart operations with minimal memory. The background persistence goroutine showcases Go's lightweight concurrency.

**Key Go Concurrency Patterns Used:**
- **Background goroutine** for debounced PostgreSQL persistence — writes cart to DB every 5 minutes via channel signal
- **`sync.Pool`** for JSON encoder/decoder buffers — reduces GC pressure under high cart traffic
- **`errgroup`** for parallel product validation — validate all cart items concurrently via Product Service
- **Channel-based debouncer** — `chan string` collects dirty cart IDs, single goroutine batches writes
- **`context.WithTimeout`** on Product Service calls — 3s timeout prevents goroutine leaks when Product Service is slow

**Data Model (PostgreSQL — for persistence only):**

| Table | Key Columns |
|---|---|
| `carts` | id (UUID), user_id, status (ACTIVE/CHECKED_OUT/ABANDONED), expires_at, created_at, updated_at |
| `cart_items` | id, cart_id (FK), product_id, product_name (denormalized), quantity, unit_price, added_at |

**Primary Data Model (Redis — for fast access):**

```json
// Key: cart:{userId}  |  TTL: 30 minutes  |  Type: Hash
{
  "cartId": "uuid",
  "userId": "uuid",
  "items": [
    {
      "productId": 42,
      "productName": "Wireless Mouse",
      "quantity": 2,
      "unitPrice": 29.99
    }
  ],
  "totalAmount": 59.98,
  "updatedAt": "2026-02-23T10:30:00Z"
}
```

**API Endpoints:**

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/carts` | Bearer | Create a new cart (or return existing active cart) |
| GET | `/api/v1/carts/me` | Bearer | Get current user's active cart with computed total |
| POST | `/api/v1/carts/me/items` | Bearer | Add item to cart (validates product exists + in stock) |
| PUT | `/api/v1/carts/me/items/{productId}` | Bearer | Update item quantity |
| DELETE | `/api/v1/carts/me/items/{productId}` | Bearer | Remove item from cart |
| DELETE | `/api/v1/carts/me` | Bearer | Clear entire cart |
| POST | `/api/v1/carts/me/checkout` | Bearer | Validate cart and initiate order creation |

**Race Condition: Concurrent Cart Updates**
```go
// Problem: User has 2 browser tabs open, adds items simultaneously.
// Both goroutines read the cart from Redis, modify locally, write back.
// Second write overwrites the first — item lost!

// Solution: Redis WATCH + MULTI/EXEC (optimistic locking at Redis level)
func (c *CartCache) AddItemAtomic(ctx context.Context, userId string, item CartItem) error {
    key := fmt.Sprintf("cart:%s", userId)
    
    // Retry loop for optimistic locking
    for retries := 0; retries < 3; retries++ {
        err := c.redis.Watch(ctx, func(tx *redis.Tx) error {
            // WATCH: Redis will abort EXEC if key changes between WATCH and EXEC
            cartJSON, err := tx.Get(ctx, key).Result()
            if err != nil && err != redis.Nil {
                return err
            }
            
            cart := deserializeCart(cartJSON)
            cart.AddItem(item)
            
            // MULTI/EXEC: atomic write — fails if another goroutine modified the key
            _, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
                pipe.Set(ctx, key, serializeCart(cart), 30*time.Minute)
                return nil
            })
            return err
        }, key)
        
        if err == nil {
            return nil // Success
        }
        if err == redis.TxFailedErr {
            continue // Key was modified by another goroutine — retry
        }
        return err // Real error
    }
    return fmt.Errorf("cart update failed after 3 retries (high contention)")
}
```

**Background Persistence — Goroutine + Channel Pattern:**
```go
// Debounced background writer — batches dirty carts to PostgreSQL
func (s *CartService) StartBackgroundPersistence(ctx context.Context) {
    dirtyCarts := make(chan string, 1000) // Buffered channel for dirty cart IDs
    
    // Writer goroutine — runs for the lifetime of the service
    go func() {
        pending := make(map[string]time.Time) // userId → last modified time
        ticker := time.NewTicker(5 * time.Minute)
        defer ticker.Stop()
        
        for {
            select {
            case userId := <-dirtyCarts:
                pending[userId] = time.Now()
                
            case <-ticker.C:
                // Flush all pending carts to PostgreSQL
                for userId, modifiedAt := range pending {
                    if time.Since(modifiedAt) > 5*time.Minute {
                        cart, _ := s.cache.GetCart(ctx, userId)
                        s.repo.UpsertCart(ctx, cart) // Write to PostgreSQL
                        delete(pending, userId)
                    }
                }
                
            case <-ctx.Done():
                // Graceful shutdown: flush all pending carts before exit
                for userId := range pending {
                    cart, _ := s.cache.GetCart(context.Background(), userId)
                    s.repo.UpsertCart(context.Background(), cart)
                }
                return
            }
        }
    }()
}
```

**Circuit Breaker (Product Service calls):**
```go
breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name:        "product-service",
    MaxRequests: 3,                    // Half-open: allow 3 probe requests
    Interval:    60 * time.Second,     // Closed: reset failure count every 60s
    Timeout:     10 * time.Second,     // Open → Half-open after 10s
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        return counts.ConsecutiveFailures > 3
    },
})
```

**Project Structure:**
```
cart-service/
├── cmd/server/main.go
├── internal/
│   ├── handler/
│   ├── service/
│   ├── repository/        # PostgreSQL (GORM)
│   ├── cache/             # Redis cart storage
│   ├── client/            # Product Service HTTP client + circuit breaker
│   ├── model/
│   ├── dto/
│   └── middleware/
├── pkg/                   # Shared with User Service via Go module
├── config/
├── Dockerfile
├── go.mod
└── go.sum
```

---

### 4.5 Order Service (Java/Spring Boot) — Includes Notifications

**Responsibility:** Order lifecycle management with state machine, Kafka event publishing, stock orchestration, **and inline notification dispatch**.

Notification logic is handled within the Order Service's Kafka consumer as a side-effect of state transitions — no separate service needed.

**Why Java for Orders:** The order service has the most complex state machine, multi-step transactions (reserve stock → create order → publish event), and needs robust Kafka consumer/producer management. Spring's `@Transactional` and `@KafkaListener` make this declarative rather than imperative.

**Key Java Concurrency Patterns Used:**
- **`@Transactional`** with compensation: if stock reservation fails mid-order, previously reserved items are automatically released via catch block
- **`CompletableFuture.allOf()`** for parallel stock reservation: reserve all items concurrently, fail fast if any fails
- **`@Async`** for notification dispatch: fire-and-forget — never blocks the order response
- **`@KafkaListener` consumer group**: Spring manages consumer threads, partition rebalancing, and offset commits automatically
- **State machine with `synchronized` transitions**: `OrderStatus.canTransitionTo()` prevents invalid concurrent state changes

**Data Model:**

| Table | Key Columns |
|---|---|
| `orders` | id, user_id, cart_id, total_amount, status, shipping_address (JSONB), created_at, updated_at |
| `order_items` | id, order_id (FK), product_id, product_name, quantity, unit_price |
| `order_status_history` | id, order_id (FK), old_status, new_status, reason, changed_by, changed_at |
| `notifications` | id, order_id (FK), user_id, type (EMAIL/SMS), channel, subject, body, status (SENT/FAILED), sent_at |

**Order State Machine:**

```
                    ┌─────────────┐
                    │   PENDING   │  (Order created, awaiting payment)
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │                         │
     ┌────────▼────────┐      ┌────────▼────────┐
     │   CONFIRMED     │      │   CANCELLED     │
     │ (Payment success)│      │ (Payment fail / │
     └────────┬────────┘      │  user cancel)   │
              │               └─────────────────┘
     ┌────────▼────────┐
     │    SHIPPED      │
     │(Seller dispatch)│
     └────────┬────────┘
              │
     ┌────────▼────────┐
     │   DELIVERED     │
     │ (Confirmation)  │
     └─────────────────┘
```

| Transition | Trigger | Side Effects |
|---|---|---|
| → PENDING | `POST /orders` | Reserve stock (sync call to Product Service), publish `orders.created` to Kafka |
| PENDING → CONFIRMED | `payments.completed` Kafka event | Confirm stock deduction, send confirmation email |
| PENDING → CANCELLED | `payments.failed` event or `PUT /orders/{id}/cancel` | Release reserved stock, send cancellation email |
| CONFIRMED → SHIPPED | `PUT /orders/{id}/ship` (Seller/Admin) | Send shipment email with tracking |
| SHIPPED → DELIVERED | `PUT /orders/{id}/deliver` | Send delivery confirmation email |

**Race Condition: Concurrent Order State Transitions**
```java
// Problem: Payment service sends "completed" event at the same moment
// the user clicks "cancel". Two threads try to transition from PENDING simultaneously.
// One should win, the other should fail cleanly.

// Solution: Pessimistic locking on the order row during state transitions
@Transactional
public Order transitionStatus(Long orderId, OrderStatus newStatus, String reason) {
    // SELECT ... FOR UPDATE — locks the row until transaction commits
    Order order = orderRepository.findByIdWithLock(orderId)
        .orElseThrow(() -> new OrderNotFoundException(orderId));
    
    if (!order.getStatus().canTransitionTo(newStatus)) {
        throw new InvalidStateTransitionException(
            order.getStatus(), newStatus, orderId);
    }
    
    OrderStatus oldStatus = order.getStatus();
    order.setStatus(newStatus);
    orderRepository.save(order);
    
    // Audit trail
    statusHistoryRepository.save(OrderStatusHistory.builder()
        .orderId(orderId)
        .oldStatus(oldStatus)
        .newStatus(newStatus)
        .reason(reason)
        .build());
    
    return order;
}

// In repository:
@Query("SELECT o FROM Order o WHERE o.id = :id")
@Lock(LockModeType.PESSIMISTIC_WRITE)
Optional<Order> findByIdWithLock(@Param("id") Long id);
```

**Parallel Stock Reservation with CompletableFuture:**
```java
// Reserve stock for all order items in parallel — fail fast if any fails
@Transactional
public Order createOrder(CreateOrderRequest request) {
    List<OrderItem> items = request.getItems();
    
    // Launch all reservations in parallel
    List<CompletableFuture<StockReservation>> futures = items.stream()
        .map(item -> CompletableFuture.supplyAsync(() ->
            inventoryClient.reserveStock(item.getProductId(), item.getQuantity()),
            taskExecutor  // Custom thread pool, not ForkJoinPool
        ))
        .toList();
    
    try {
        // Wait for all — if any fails, we get CompletionException
        CompletableFuture.allOf(futures.toArray(new CompletableFuture[0])).join();
    } catch (CompletionException e) {
        // Compensation: release all successfully reserved items
        futures.stream()
            .filter(f -> !f.isCompletedExceptionally())
            .forEach(f -> {
                StockReservation res = f.join();
                inventoryClient.releaseStock(res.getProductId(), res.getQuantity());
            });
        throw new StockReservationFailedException(e.getCause());
    }
    
    // All reservations succeeded — create the order
    Order order = orderRepository.save(Order.from(request, OrderStatus.PENDING));
    kafkaTemplate.send("orders.created", OrderCreatedEvent.from(order));
    return order;
}
```

**API Endpoints:**

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/orders` | Bearer | Create order from validated cart |
| GET | `/api/v1/orders/{id}` | Bearer | Get order details (owner or admin) |
| GET | `/api/v1/orders` | Bearer | List current user's orders (paginated) |
| PUT | `/api/v1/orders/{id}/cancel` | Bearer | Cancel order (only if PENDING) |
| PUT | `/api/v1/orders/{id}/ship` | Seller/Admin | Mark order as shipped |
| PUT | `/api/v1/orders/{id}/deliver` | Admin | Mark order as delivered |
| GET | `/api/v1/orders/{id}/history` | Bearer | View order status change history |

**Kafka Events Published:**

| Topic | Event | Trigger | Payload |
|---|---|---|---|
| `orders.created` | ORDER_CREATED | New order placed | orderId, userId, items[], totalAmount |
| `orders.confirmed` | ORDER_CONFIRMED | Payment succeeded | orderId, userId |
| `orders.cancelled` | ORDER_CANCELLED | Cancel action | orderId, userId, reason, items[] (for stock release) |

**Kafka Events Consumed:**

| Topic | Handler | Action |
|---|---|---|
| `payments.completed` | `PaymentEventConsumer` | Transition order PENDING→CONFIRMED, confirm stock, send email |
| `payments.failed` | `PaymentEventConsumer` | Transition order PENDING→CANCELLED, release stock, send failure email |

**Inline Notification Logic:**
```java
@Component
public class NotificationDispatcher {
    @Async("notificationExecutor")  // Runs in dedicated thread pool
    public CompletableFuture<Void> sendOrderConfirmation(Order order) {
        try {
            String body = templateEngine.render("order-confirmation", order);
            Notification notification = Notification.builder()
                .orderId(order.getId())
                .userId(order.getUserId())
                .type(NotificationType.EMAIL)
                .subject("Order #" + order.getId() + " Confirmed")
                .body(body)
                .status(NotificationStatus.SENT)
                .build();
            notificationRepository.save(notification);
            log.info("Notification sent for order {}", order.getId());
        } catch (Exception e) {
            log.error("Notification failed for order {}", order.getId(), e);
            // Fire-and-forget: notification failure does NOT affect order flow
        }
        return CompletableFuture.completedFuture(null);
    }
}
```

**Project Structure:**
```
order-service/
├── src/main/java/com/ecommerce/order/
│   ├── controller/
│   ├── service/
│   │   ├── OrderService.java
│   │   ├── OrderStateMachine.java
│   │   └── NotificationDispatcher.java
│   ├── kafka/
│   │   ├── OrderEventProducer.java
│   │   └── PaymentEventConsumer.java
│   ├── client/               # Product Service Feign/RestTemplate client
│   ├── repository/
│   ├── model/
│   ├── dto/
│   ├── exception/
│   └── config/
├── src/main/resources/
│   ├── application.yml
│   ├── templates/            # Email notification templates
│   └── db/migration/
├── src/test/
├── pom.xml
└── Dockerfile
```

---

### 4.6 Payment Service (Golang)

**Responsibility:** Payment processing with idempotency guarantees, Kafka event consumption/production, and refund handling.

**Why Go for Payments:** Payment processing is I/O-bound (HTTP to mock gateway, Kafka pub/sub, DB writes) and needs the highest reliability. Go's explicit error handling forces every error path to be considered — no hidden exceptions. The goroutine-based Kafka consumer worker pool handles event spikes efficiently.

**Key Go Concurrency Patterns Used:**
- **Worker pool pattern** for Kafka consumption: N goroutines read from a channel, processing payment events in parallel
- **`sync.Mutex`** on idempotency cache: protect in-memory idempotency check before DB lookup
- **`context.WithTimeout`** on gateway calls: 5s timeout prevents goroutine leaks when gateway is slow
- **Graceful shutdown** via `os.Signal` + `context.Done()`: drain Kafka consumer, finish in-flight payments, then exit

**Data Model:**

| Table | Key Columns |
|---|---|
| `payments` | id (UUID), order_id, user_id, amount (DECIMAL(10,2)), currency (default: USD), status (PENDING/COMPLETED/FAILED/REFUNDED), method (MOCK_CARD/MOCK_WALLET), idempotency_key (unique), gateway_reference, created_at, updated_at |
| `payment_history` | id, payment_id (FK), old_status, new_status, reason, created_at |

**API Endpoints:**

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/payments` | Internal | Initiate payment (idempotency key required) |
| GET | `/api/v1/payments/{id}` | Bearer | Get payment status |
| GET | `/api/v1/payments/order/{orderId}` | Bearer | Get payment by order ID |
| POST | `/api/v1/payments/{id}/refund` | Admin | Initiate full refund |

**Kafka Worker Pool — Go Channel Pattern:**
```go
// Instead of processing Kafka events sequentially (slow),
// we fan-out to a pool of worker goroutines via channels.

func (c *PaymentConsumer) Start(ctx context.Context, workerCount int) {
    events := make(chan OrderCreatedEvent, 100) // Buffered channel
    
    // Start N worker goroutines
    var wg sync.WaitGroup
    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            for event := range events { // Blocks until channel has data
                if err := c.processPayment(ctx, event); err != nil {
                    log.Error("worker %d: payment failed for order %s: %v",
                        workerID, event.OrderID, err)
                    c.sendToDLQ(event, err) // Dead letter queue
                }
            }
        }(i)
    }
    
    // Kafka consumer goroutine — reads messages and sends to worker channel
    go func() {
        defer close(events) // Close channel when consumer stops
        for {
            select {
            case <-ctx.Done():
                return // Graceful shutdown
            default:
                msg, err := c.consumer.ReadMessage(time.Second)
                if err != nil {
                    continue // Timeout — try again
                }
                var event OrderCreatedEvent
                json.Unmarshal(msg.Value, &event)
                events <- event // Send to worker pool
            }
        }
    }()
    
    // Wait for shutdown
    <-ctx.Done()
    wg.Wait() // Wait for all workers to finish in-flight payments
}
```

**Idempotency — Preventing Double Charges:**
```go
func (s *PaymentService) ProcessPayment(ctx context.Context, req PaymentRequest) (*Payment, error) {
    // 1. Check idempotency key (DB unique constraint as safety net)
    existing, err := s.repo.FindByIdempotencyKey(ctx, req.IdempotencyKey)
    if err == nil {
        return existing, nil // Return original result — no reprocessing
    }

    // 2. Create payment record (status=PENDING) — claim the idempotency key
    payment := &model.Payment{
        OrderID:        req.OrderID,
        Amount:         req.Amount,
        IdempotencyKey: req.IdempotencyKey,
        Status:         model.PaymentPending,
    }
    if err := s.repo.Create(ctx, payment); err != nil {
        if isDuplicateKeyError(err) {
            // Race condition: another goroutine claimed the key first
            existing, _ := s.repo.FindByIdempotencyKey(ctx, req.IdempotencyKey)
            return existing, nil
        }
        return nil, err
    }

    // 3. Call mock payment gateway (with timeout)
    gatewayCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    gatewayResult := s.gateway.Charge(gatewayCtx, payment.Amount, payment.Method)

    // 4. Update status and publish Kafka event
    if gatewayResult.Success {
        payment.Status = model.PaymentCompleted
        payment.GatewayReference = gatewayResult.ReferenceID
        s.kafka.Publish("payments.completed", PaymentCompletedEvent{
            OrderID:   payment.OrderID,
            PaymentID: payment.ID,
            Amount:    payment.Amount,
        })
    } else {
        payment.Status = model.PaymentFailed
        s.kafka.Publish("payments.failed", PaymentFailedEvent{
            OrderID: payment.OrderID,
            Reason:  gatewayResult.ErrorMessage,
        })
    }

    return s.repo.Update(ctx, payment)
}
```

**Dead Letter Queue:**
- Failed events routed to `payments.dlq` after 3 retry attempts
- Retry backoff: 100ms → 200ms → 400ms (exponential)
- DLQ messages can be manually replayed via admin endpoint

**Mock Payment Gateway:**
- Simulates real gateway behavior with configurable success rate (default: 90%)
- Adds random latency (50–200ms) to simulate network call
- Returns realistic gateway reference IDs
- Supports forced failure via special card numbers (for testing)

**Project Structure:**
```
payment-service/
├── cmd/server/main.go
├── internal/
│   ├── handler/
│   ├── service/
│   ├── repository/
│   ├── kafka/
│   │   ├── consumer.go     # Worker pool consumer
│   │   └── producer.go     # Produces payments.completed/failed
│   ├── gateway/            # Mock payment gateway
│   ├── model/
│   ├── dto/
│   └── middleware/
├── config/
├── Dockerfile
├── go.mod
└── go.sum
```

---

## 5. Database Design

### 5.1 Strategy

- **Database-per-service:** One PostgreSQL instance with 5 logical databases. Each service connects with its own credentials and can only access its own database.
- **Normalization:** All schemas follow 3NF. Denormalization only where justified (e.g., `product_name` in `cart_items` and `order_items` to avoid cross-service joins).
- **Migrations:** Flyway for Java services (versioned SQL files); GORM AutoMigrate for Go services (development), manual SQL migrations for production.
- **UUIDs:** All primary keys use UUID v4 to avoid sequential ID leakage and simplify distributed ID generation.

### 5.2 Schema Ownership

| Database | Owner Service | Key Tables |
|---|---|---|
| `ecommerce_users` | User Service | users, user_profiles, user_addresses, auth_tokens |
| `ecommerce_products` | Product Service | products, categories, product_images, stock_movements |
| `ecommerce_carts` | Cart Service | carts, cart_items |
| `ecommerce_orders` | Order Service | orders, order_items, order_status_history, notifications |
| `ecommerce_payments` | Payment Service | payments, payment_history |

### 5.3 Connection Pooling

| Service | Pool Config | Rationale |
|---|---|---|
| Go services | `MaxOpenConns=25`, `MaxIdleConns=5`, `ConnMaxLifetime=5m` | Goroutines share fewer connections efficiently — Go's scheduler multiplexes goroutines onto OS threads |
| Java services | HikariCP: `maximumPoolSize=20`, `minimumIdle=5`, `idleTimeout=300000` | Thread-per-request model needs more connections; HikariCP is the fastest Java connection pool |

**Connection Pool Tuning Methodology:**
1. Start with `MaxOpenConns = 2 × CPU cores + 1` (PostgreSQL recommendation)
2. Monitor `pg_stat_activity` for connection count during load tests
3. If `waiting` connections appear → increase pool size or optimize slow queries
4. If pool is consistently <50% utilized → decrease to free DB resources for other services

### 5.4 Indexing Strategy

| Table | Index | Type | Reason |
|---|---|---|---|
| `users` | `email` | UNIQUE | Login lookups — must be fast for every auth request |
| `products` | `category_id` | B-TREE | Category filtering |
| `products` | `name, description` | GIN (full-text) | Search queries — GIN is faster than B-TREE for text search |
| `products` | `created_at DESC` | B-TREE | Sorting by recency |
| `products` | `status` | B-TREE | Active product filtering (partial index: `WHERE status = 'ACTIVE'`) |
| `orders` | `user_id` | B-TREE | User order history |
| `orders` | `created_at DESC` | B-TREE | Recent orders listing |
| `orders` | `status` | B-TREE | Order filtering by state |
| `payments` | `idempotency_key` | UNIQUE | Duplicate prevention — critical for payment safety |
| `payments` | `order_id` | B-TREE | Payment lookup by order |
| `stock_movements` | `product_id` | B-TREE | Audit trail queries |

---

## 6. API Design

### 6.1 Conventions

- **Base URL:** `http://{host}/api/v1` (via Nginx on port 80)
- **Format:** JSON request/response bodies with `Content-Type: application/json`
- **Versioning:** URI path prefix (`/api/v1/`)
- **Authentication:** Bearer JWT in `Authorization` header
- **Pagination:** Query params `?page=0&size=20&sort=createdAt,desc`
- **Filtering:** Query params `?category=electronics&minPrice=10&maxPrice=100`

### 6.2 Standard Response Envelope

**Success:**
```json
{
  "success": true,
  "data": { ... },
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
    "details": {
      "productId": 42,
      "requested": 5,
      "available": 3
    }
  }
}
```

### 6.3 HTTP Status Code Usage

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

### 6.4 API Specification

Full API specification authored in **OpenAPI 3.0**:
- File: `api/openapi.yaml`
- Served via Swagger UI at `/swagger` through Nginx
- Reusable schemas under `components/schemas`
- Request/response examples for every endpoint
- Authentication scheme documentation

---

## 7. Inter-Service Communication

### 7.1 Synchronous (HTTP/REST)

| Caller | Target | Purpose | Failure Handling |
|---|---|---|---|
| Cart Service | Product Service | Validate product price + stock on add/checkout | Circuit breaker + fallback to cached price |
| Order Service | Product Service | Reserve stock at order creation | Retry 3x, then fail order with clear error |
| Order Service | Product Service | Release stock on cancellation | Retry with DLQ fallback |
| Order Service | Product Service | Confirm stock deduction on payment success | Retry with DLQ fallback |

### 7.2 Asynchronous (Kafka Events)

**Kafka Topics:**

| Topic | Producer | Consumer(s) | Purpose |
|---|---|---|---|
| `orders.created` | Order Service | Payment Service | Trigger payment processing |
| `orders.confirmed` | Order Service | — (logged for audit) | Record order confirmation |
| `orders.cancelled` | Order Service | — (logged for audit) | Record order cancellation |
| `payments.completed` | Payment Service | Order Service | Confirm order, deduct stock, send email |
| `payments.failed` | Payment Service | Order Service | Cancel order, release stock, send email |

**Event Schema:**
```json
{
  "eventId": "550e8400-e29b-41d4-a716-446655440000",
  "eventType": "ORDER_CREATED",
  "timestamp": "2026-02-23T10:30:00Z",
  "version": "1.0",
  "source": "order-service",
  "payload": {
    "orderId": "uuid",
    "userId": "uuid",
    "items": [
      { "productId": 42, "quantity": 2, "unitPrice": 29.99 }
    ],
    "totalAmount": 59.98,
    "currency": "USD"
  }
}
```

### 7.3 Saga Pattern — Order Processing Flow

```
┌─────────┐     POST /orders      ┌──────────────┐
│  Client  │ ────────────────────► │ Order Service │
└─────────┘                       └──────┬───────┘
                                         │
                          1. Validate cart contents
                          2. Call Product Service: reserve stock (parallel)
                          3. Create order (status=PENDING)
                          4. Publish "orders.created" to Kafka
                                         │
                                  ┌──────▼────────┐
                                  │  Kafka Broker  │
                                  └──────┬────────┘
                                         │
                               ┌─────────▼──────────┐
                               │  Payment Service   │
                               │  (worker pool)     │
                               │  1. Consume event   │
                               │  2. Check idempotency│
                               │  3. Process payment  │
                               │  4. Publish result   │
                               └─────────┬──────────┘
                                         │
                                  ┌──────▼────────┐
                                  │  Kafka Broker  │
                                  └──────┬────────┘
                                         │
                              ┌──────────▼───────────┐
                              │   Order Service      │
                              │   1. Consume result   │
                              │   2. Lock order row   │
                              │   3. Transition state  │
                              │   4. Confirm/release  │
                              │      stock            │
                              │   5. Send notification│
                              │      (@Async)         │
                              └──────────────────────┘
```

**Compensation (on failure):**

| Failure Point | Compensation | Idempotent? |
|---|---|---|
| Stock reservation fails | Return 409 to client (order not created) | N/A |
| Payment fails | Order→CANCELLED, release reserved stock | ✅ |
| Order confirmation fails | Payment stays completed, retry via Kafka | ✅ (idempotency key) |
| Notification fails | Logged to DB, does NOT block order flow | ✅ |

---

## 8. Caching Strategy

### 8.1 Cache Patterns

| Pattern | Use Case | Service |
|---|---|---|
| **Cache-Aside** | Product lookups (read-heavy, write-rare) | Product Service |
| **Write-Through** | Session data (must be consistent on write) | User Service |
| **TTL-Based Expiry** | Cart data, rate limit counters | Cart Service, Nginx |

### 8.2 Redis Key Design

| Key Pattern | Value | TTL | Service |
|---|---|---|---|
| `session:{userId}` | User profile JSON | 30 min | User Service |
| `blacklist:{jti}` | `"revoked"` | Matches JWT remaining lifetime | User Service |
| `login_attempts:{email}` | Integer counter | 15 min (sliding) | User Service |
| `product:{productId}` | Product JSON (with stock) | 10 min | Product Service |
| `category:list` | All categories JSON | 30 min | Product Service |
| `cart:{userId}` | Cart JSON with items | 30 min (extended on write) | Cart Service |
| `idempotency:{key}` | Payment result JSON | 24 hours | Payment Service |

### 8.3 Cache Invalidation

| Trigger | Action | Mechanism |
|---|---|---|
| Product updated | Evict `product:{id}` | `@CacheEvict` annotation |
| Product stock changed | Evict `product:{id}` | Explicit delete after stock operation |
| User profile updated | Overwrite `session:{userId}` | Write-through on update |
| User logs out | Delete `session:{userId}`, set `blacklist:{jti}` | Explicit delete |
| Cart modified | Overwrite `cart:{userId}`, reset TTL | Full key replacement |
| Service startup | Warm top-50 products | Async `@PostConstruct` |

### 8.4 Redis Atomic Operations — Preventing Cache Race Conditions

```go
// Problem: Two goroutines increment a counter simultaneously
// Solution: Redis INCR is atomic — no need for application-level locks

// Login attempt counter — atomic increment
func (r *RedisClient) IncrementLoginAttempts(ctx context.Context, email string) (int64, error) {
    key := fmt.Sprintf("login_attempts:%s", email)
    
    // INCR is atomic in Redis — safe for concurrent goroutines
    count, err := r.client.Incr(ctx, key).Result()
    if err != nil {
        return 0, err
    }
    
    // Set TTL only on first increment (sliding window)
    if count == 1 {
        r.client.Expire(ctx, key, 15*time.Minute)
    }
    
    return count, nil
}
```

```lua
-- Problem: Check stock + reserve must be atomic (no other thread between check and reserve)
-- Solution: Redis Lua script — executes atomically on Redis server

-- lua/reserve_stock.lua
local key = KEYS[1]                    -- "stock:{productId}"
local requested = tonumber(ARGV[1])     -- quantity to reserve
local current = tonumber(redis.call('GET', key) or 0)

if current >= requested then
    redis.call('DECRBY', key, requested)
    return 1  -- Success
else
    return 0  -- Insufficient stock
end

-- This entire script runs atomically — no race condition possible
```

---

## 9. Security

### 9.1 Authentication & Authorization

| Mechanism | Details |
|---|---|
| Password storage | bcrypt, cost factor 12, salted automatically |
| Access token | JWT RS256, 15-min TTL, claims: `{sub, email, role, jti, iat, exp}` |
| Refresh token | Cryptographically random, 7-day TTL, stored hashed in DB |
| Token revocation | Redis blacklist keyed by `jti`, TTL = remaining token lifetime |
| RBAC roles | `ADMIN`, `SELLER`, `CUSTOMER` — enforced at handler level |
| Account lockout | 5 consecutive failures → 15-min lockout (tracked in Redis) |
| Password policy | Minimum 12 characters, at least 1 uppercase, 1 digit, 1 special char |

### 9.2 API Security

| Control | Implementation |
|---|---|
| Rate limiting | Nginx `limit_req`: 100 req/min per IP (burst 20) |
| Input validation | Go: struct tags + custom validators; Java: `@Valid` + `@NotNull` + `@Size` |
| SQL injection | Parameterized queries only (GORM, JPA) — no raw SQL concatenation |
| XSS | `Content-Security-Policy` + `X-Content-Type-Options: nosniff` headers |
| CORS | Nginx `add_header` — configurable allowed origins |
| Service-to-service | Internal network only (Docker network isolation); API key header for added security |
| HTTPS | TLS termination at Nginx (self-signed in dev, Let's Encrypt in prod) |

### 9.3 Dependency Security

- Go: `govulncheck` in CI pipeline
- Java: OWASP Dependency-Check Maven plugin
- No critical CVEs allowed in production builds
- Dependabot enabled on GitHub repository

---

## 10. Concurrency & Race Conditions

This section consolidates all concurrency challenges and their solutions. This is the **core engineering challenge** of the project — distinguishing it from simple CRUD applications.

### 10.1 Race Condition Inventory

| Race Condition | Where | Severity | Solution |
|---|---|---|---|
| **Stock overselling** | Product Service | 🔴 Critical | `@Version` optimistic locking + retry |
| **Double payment** | Payment Service | 🔴 Critical | Idempotency key (DB unique constraint) |
| **Concurrent state transition** | Order Service | 🔴 Critical | `SELECT ... FOR UPDATE` pessimistic lock |
| **Cart item lost update** | Cart Service (Redis) | 🟡 High | Redis `WATCH/MULTI/EXEC` optimistic lock |
| **Login attempt race** | User Service | 🟡 High | `SELECT ... FOR UPDATE` on user row |
| **Cache stampede** | Product Service | 🟡 High | Single-flight pattern (singleflight package) |
| **Kafka duplicate delivery** | Payment Service | 🟡 High | Idempotency key prevents reprocessing |
| **Token blacklist race** | User Service | 🟢 Medium | Redis SET is atomic — no race |
| **Background cart sync** | Cart Service | 🟢 Medium | Channel-based serialization |

### 10.2 Go Concurrency Toolkit

**Patterns used across User, Cart, and Payment services:**

```go
// 1. ERRGROUP — Parallel tasks with error propagation and cancellation
// Used in: Cart checkout (validate all items in parallel)
g, ctx := errgroup.WithContext(ctx)
for _, item := range items {
    item := item
    g.Go(func() error {
        return validateItem(ctx, item) // Cancelled if any fails
    })
}
if err := g.Wait(); err != nil { /* first error */ }

// 2. SYNC.POOL — Object reuse to reduce GC pressure
// Used in: Cart Service JSON serialization (high-frequency)
var bufPool = sync.Pool{
    New: func() interface{} { return new(bytes.Buffer) },
}
func serializeCart(cart *Cart) []byte {
    buf := bufPool.Get().(*bytes.Buffer)
    defer bufPool.Put(buf)
    buf.Reset()
    json.NewEncoder(buf).Encode(cart)
    return buf.Bytes()
}

// 3. CONTEXT WITH TIMEOUT — Prevent goroutine leaks
// Used in: Every inter-service HTTP call
ctx, cancel := context.WithTimeout(parentCtx, 3*time.Second)
defer cancel()
resp, err := httpClient.Do(req.WithContext(ctx))
// If timeout expires, the HTTP call is aborted and goroutine can exit

// 4. SYNC.RWMUTEX — Concurrent reads, exclusive writes
// Used in: Circuit breaker state, in-memory config cache
type CircuitState struct {
    mu       sync.RWMutex
    failures int
    state    string // "closed", "open", "half-open"
}
func (s *CircuitState) GetState() string {
    s.mu.RLock()         // Multiple goroutines can read simultaneously
    defer s.mu.RUnlock()
    return s.state
}
func (s *CircuitState) RecordFailure() {
    s.mu.Lock()          // Exclusive access for writes
    defer s.mu.Unlock()
    s.failures++
}

// 5. CHANNEL-BASED WORKER POOL — Fan-out for Kafka consumption
// Used in: Payment Service Kafka consumer (see §4.6)
events := make(chan Event, 100)
for i := 0; i < numWorkers; i++ {
    go worker(events) // Each worker processes events from channel
}

// 6. SINGLEFLIGHT — Prevent cache stampede
// Used in: Product Service cache misses
var group singleflight.Group
func GetProduct(ctx context.Context, id string) (*Product, error) {
    val, err, _ := group.Do(id, func() (interface{}, error) {
        // Only ONE goroutine fetches from DB, others wait for the result
        return productRepo.FindByID(ctx, id)
    })
    return val.(*Product), err
}
```

### 10.3 Java Concurrency Toolkit

**Patterns used across Product and Order services:**

```java
// 1. @TRANSACTIONAL ISOLATION LEVELS
// READ_COMMITTED (default): prevents dirty reads
// REPEATABLE_READ: prevents non-repeatable reads
// SERIALIZABLE: full isolation (slowest, use sparingly)
@Transactional(isolation = Isolation.READ_COMMITTED) // Default for most operations
public Product getProduct(Long id) { ... }

@Transactional(isolation = Isolation.SERIALIZABLE)  // For flash-sale stock check
public void reserveStockFlashSale(Long productId, int qty) { ... }

// 2. @VERSION OPTIMISTIC LOCKING (see §4.3 for full example)
// JPA adds WHERE version=? to every UPDATE
// On conflict → OptimisticLockException → @Retry handles it

// 3. COMPLETABLEFUTURE — Parallel async operations
// Used in: Order Service parallel stock reservation (see §4.5)
CompletableFuture.allOf(futures.toArray(new CompletableFuture[0]))
    .thenAccept(v -> log.info("All reservations completed"))
    .exceptionally(ex -> { compensate(); return null; });

// 4. @ASYNC WITH CUSTOM THREAD POOL
// Used in: Notification dispatch (fire-and-forget)
@Configuration
@EnableAsync
public class AsyncConfig {
    @Bean("notificationExecutor")
    public Executor notificationExecutor() {
        ThreadPoolTaskExecutor executor = new ThreadPoolTaskExecutor();
        executor.setCorePoolSize(2);
        executor.setMaxPoolSize(5);
        executor.setQueueCapacity(100);
        executor.setThreadNamePrefix("notification-");
        executor.setRejectedExecutionHandler(new CallerRunsPolicy());
        // CallerRunsPolicy: if queue full, caller thread processes it
        // This prevents notification loss under load
        return executor;
    }
}

// 5. PESSIMISTIC LOCKING — SELECT FOR UPDATE
// Used in: Order state transitions (see §4.5)
// Ensures only one thread can transition an order's state at a time
@Lock(LockModeType.PESSIMISTIC_WRITE)
@Query("SELECT o FROM Order o WHERE o.id = :id")
Optional<Order> findByIdWithLock(@Param("id") Long id);

// 6. VIRTUAL THREADS (Java 21) — Lightweight concurrency
// Used in: Kafka consumers (block without wasting OS threads)
@Bean
public Executor virtualThreadExecutor() {
    return Executors.newVirtualThreadPerTaskExecutor();
    // Each Kafka message processed in a virtual thread (~1KB stack)
    // vs platform thread (~1MB stack)
    // Allows 100K+ concurrent consumers without memory pressure
}
```

### 10.4 Distributed Race Conditions & Solutions

**Scenario 1: 100 users buy the last 10 items simultaneously**
```
Timeline:
T0: User A reads product (stock=10, version=42)
T0: User B reads product (stock=10, version=42)
T0: User C reads product (stock=10, version=42)
...
T1: User A saves (stock_reserved=1, version=43) ✅
T1: User B saves (version=42 ≠ 43) ❌ OptimisticLockException → RETRY
T2: User B re-reads (stock=10, reserved=1, version=43)
T2: User B saves (stock_reserved=2, version=44) ✅
...
T5: User K tries to reserve (available=0) → InsufficientStockException
```
**Result:** Exactly 10 reservations, 90 rejections, zero overselling.

**Scenario 2: User pays, then immediately cancels**
```
Timeline:
T0: Kafka delivers "payments.completed" to Order Service (Thread A)
T0: User clicks "Cancel" (Thread B)
T1: Thread A: SELECT order FOR UPDATE (acquires row lock)
T1: Thread B: SELECT order FOR UPDATE (BLOCKS — waits for Thread A)
T2: Thread A: PENDING → CONFIRMED, COMMIT (releases lock)
T3: Thread B: reads order (status=CONFIRMED)
T3: Thread B: canTransitionTo(CANCELLED) = false → InvalidStateTransitionException
```
**Result:** Payment wins, cancel fails cleanly with a clear error message.

**Scenario 3: Kafka delivers the same event twice**
```
Timeline:
T0: Kafka delivers ORDER_CREATED (eventId=abc-123) to Payment worker 1
T1: Kafka rebalances, re-delivers same event to Payment worker 2
T2: Worker 1: INSERT payment (idempotency_key="order-uuid") → ✅
T3: Worker 2: INSERT payment (idempotency_key="order-uuid") → DUPLICATE KEY ERROR
T3: Worker 2: SELECT by idempotency_key → returns existing payment → no double charge
```
**Result:** Exactly one payment processed, duplicate safely ignored.

### 10.5 Testing Concurrency

```go
// Go: Race detector catches data races at test time
// Run: go test -race ./...
// This instruments all memory accesses and reports races

// Go: Concurrent stock reservation test
func TestConcurrentStockReservation(t *testing.T) {
    product := createProductWithStock(100) // 100 units
    
    var wg sync.WaitGroup
    var successCount int64
    var failCount int64
    
    for i := 0; i < 200; i++ { // 200 goroutines reserving 1 each
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

```java
// Java: Concurrent reservation test with ExecutorService
@Test
void testConcurrentStockReservation() throws Exception {
    Product product = createProductWithStock(100);
    
    ExecutorService executor = Executors.newFixedThreadPool(50);
    CountDownLatch startLatch = new CountDownLatch(1); // Start all at once
    AtomicInteger successCount = new AtomicInteger(0);
    
    List<Future<?>> futures = new ArrayList<>();
    for (int i = 0; i < 200; i++) {
        futures.add(executor.submit(() -> {
            startLatch.await(); // All threads start simultaneously
            try {
                inventoryService.reserveStock(product.getId(), 1, UUID.randomUUID());
                successCount.incrementAndGet();
            } catch (Exception e) { /* expected for 100 of them */ }
            return null;
        }));
    }
    
    startLatch.countDown(); // Release all threads at once
    futures.forEach(f -> { try { f.get(); } catch (Exception e) {} });
    
    assertEquals(100, successCount.get()); // Exactly 100 reservations
    assertEquals(0, productRepository.findById(product.getId()).getAvailableStock());
}
```

---

## 11. Performance & Scalability

### 11.1 Performance Targets (SLA/SLO)

| Metric | Target | How We Achieve It |
|---|---|---|
| Median latency (p50) | < 200ms | Redis caching, connection pooling |
| Tail latency (p99) | < 1 second | Circuit breakers prevent slow cascading failures |
| Concurrent users | 10,000 | Go goroutines + Java virtual threads |
| Kafka throughput | 1,000 events/second | Partitioned topics + worker pool consumers |
| Availability | 99.9% | Health checks, graceful shutdown, DLQ |
| Product search | < 500ms for full-text queries | GIN index on PostgreSQL |
| Cart operations | < 50ms (Redis-backed) | Redis-first architecture, no DB on hot path |

### 11.2 Go-Specific Performance Optimizations

```go
// 1. PPROF PROFILING — Built into Go standard library
import _ "net/http/pprof"

func main() {
    // Expose pprof endpoints on a separate port (not public)
    go func() {
        log.Println(http.ListenAndServe(":6060", nil))
    }()
    
    // Access at:
    // http://localhost:6060/debug/pprof/          — index
    // http://localhost:6060/debug/pprof/goroutine  — goroutine dump
    // http://localhost:6060/debug/pprof/heap       — memory allocation
    // http://localhost:6060/debug/pprof/profile    — CPU profile (30s)
    
    // Generate flame graph:
    // go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile
}

// 2. ZERO-ALLOCATION JSON SERIALIZATION
// Use jsoniter or sonic instead of encoding/json for hot paths
import jsoniter "github.com/json-iterator/go"
var json = jsoniter.ConfigCompatibleWithStandardLibrary

// 3. SYNC.POOL FOR BUFFER REUSE
// Reduces GC pressure on high-throughput paths (Cart reads/writes)
var bufferPool = sync.Pool{
    New: func() interface{} {
        return bytes.NewBuffer(make([]byte, 0, 4096))
    },
}

// 4. CONTEXT-BASED TIMEOUT PROPAGATION
// Every HTTP handler sets a deadline — all downstream calls respect it
func (h *Handler) GetCart(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
    defer cancel()
    cart, err := h.service.GetCart(ctx, userId) // 5s budget for entire request
}

// 5. GO BENCHMARKS — Measure before optimizing
func BenchmarkSerializeCart(b *testing.B) {
    cart := generateLargeCart(50) // 50 items
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        serializeCart(cart)
    }
    // Run: go test -bench=BenchmarkSerializeCart -benchmem
    // Output: 50000 ops, 2048 B/op, 3 allocs/op
}
```

### 11.3 Java-Specific Performance Optimizations

```java
// 1. JPA QUERY OPTIMIZATION — Avoid N+1 queries
// BAD: Loading orders → each order triggers separate query for items
// GOOD: Use @EntityGraph or JOIN FETCH
@EntityGraph(attributePaths = {"items", "statusHistory"})
Optional<Order> findById(Long id);

// Or with JPQL:
@Query("SELECT o FROM Order o JOIN FETCH o.items WHERE o.userId = :userId")
List<Order> findByUserIdWithItems(@Param("userId") String userId);

// 2. JPA BATCH OPERATIONS — Reduce round-trips
spring:
  jpa:
    properties:
      hibernate:
        jdbc:
          batch_size: 50        # Batch INSERT/UPDATE 50 at a time
          order_inserts: true   # Group inserts by entity type
          order_updates: true
          
// 3. HIKARICP TUNING
spring:
  datasource:
    hikari:
      maximum-pool-size: 20     # 2 × CPU cores + 1
      minimum-idle: 5
      idle-timeout: 300000      # 5 min
      max-lifetime: 1800000     # 30 min
      connection-timeout: 30000 # 30s
      leak-detection-threshold: 60000 # Log if connection held > 60s

// 4. JVM TUNING FOR CONTAINERS
// In Dockerfile:
ENTRYPOINT ["java", 
    "-XX:+UseContainerSupport",       // Respect Docker memory limits
    "-XX:MaxRAMPercentage=75.0",      // Use 75% of container memory
    "-XX:+UseZGC",                    // Low-latency garbage collector
    "-XX:+ZGenerational",             // Generational ZGC (Java 21)
    "-jar", "app.jar"]

// 5. JAVA FLIGHT RECORDER (JFR) — Production-safe profiling
// Start recording: java -XX:StartFlightRecording=duration=60s,filename=recording.jfr
// Analyze with JDK Mission Control (jmc)
// Captures: method profiling, thread contention, GC activity, I/O
```

### 11.4 Database Performance

```sql
-- EXPLAIN ANALYZE every critical query before shipping
EXPLAIN ANALYZE 
SELECT * FROM products 
WHERE category_id = 5 
  AND status = 'ACTIVE'
ORDER BY created_at DESC 
LIMIT 20;

-- Expected: Index Scan on idx_products_category_id
-- Red flag: Seq Scan (add missing index)
-- Red flag: Sort (add ORDER BY to index: category_id, created_at DESC)

-- PARTIAL INDEX — Only index active products (saves space + speed)
CREATE INDEX idx_products_active ON products(category_id, created_at DESC) 
WHERE status = 'ACTIVE';

-- Monitor slow queries
ALTER SYSTEM SET log_min_duration_statement = 100; -- Log queries > 100ms
SELECT pg_reload_conf();
```

### 11.5 Load Testing

- **Tool:** k6 (https://k6.io/)
- **Profile:** Ramp to 100 VUs over 2 min → sustain 5 min → ramp down 2 min
- **Scenarios:**
  1. Product listing + search (read-heavy, expect p95 < 500ms)
  2. Add to cart + update (mixed read/write, expect p95 < 200ms)
  3. Full checkout flow (write-heavy, multi-service, expect p95 < 2s)
  4. Concurrent stock reservation — 200 VUs reserving same product (contention test)
- **Results:** Documented in `docs/load-test-results.md` with before/after comparisons

---

## 12. Resilience & Fault Tolerance

### 12.1 Circuit Breaker Configuration

| Caller → Target | Library | Failure Threshold | Timeout | Half-Open Probes |
|---|---|---|---|---|
| Cart → Product | `gobreaker` | 3 consecutive failures | 10s | 3 requests |
| Order → Product | Resilience4j | 50% failure rate (10 calls) | 15s | 5 requests |

### 12.2 Retry Strategy

| Context | Max Retries | Backoff | Idempotent? |
|---|---|---|---|
| HTTP calls (service-to-service) | 3 | Exponential: 100ms, 200ms, 400ms | GET: yes, POST: only with idempotency key |
| Kafka consumer (on processing failure) | 3 | Exponential: 100ms, 200ms, 400ms | Yes (idempotency key) |
| Kafka consumer (after max retries) | — | Route to DLQ | — |
| Database connection | 5 | Fixed: 1s | N/A |
| Optimistic lock conflict | 3 | Immediate (re-read + retry) | Yes (same operation) |

### 12.3 Graceful Shutdown

All services implement graceful shutdown:
1. Stop accepting new requests
2. Finish in-flight requests (30s timeout)
3. Close database connections
4. Close Kafka consumers/producers (drain in-flight messages)
5. Close Redis connections

**Go implementation:**
```go
func main() {
    srv := &http.Server{Addr: ":8001", Handler: router}
    
    go func() { srv.ListenAndServe() }()
    
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit // Block until signal
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    srv.Shutdown(ctx)     // Stop accepting, finish in-flight
    kafkaConsumer.Close() // Drain consumer
    db.Close()            // Close DB pool
    redisClient.Close()   // Close Redis
}
```

### 12.4 Health Checks

Every service exposes:

| Endpoint | Probe Type | Checks |
|---|---|---|
| `GET /health/live` | Liveness | Process is running |
| `GET /health/ready` | Readiness | DB connected, Redis reachable, Kafka connected (where applicable) |

Health checks are configured in Docker Compose with `healthcheck` directives. Services depend on infrastructure readiness.

---

## 13. Deployment & Infrastructure

### 13.1 Containerization

Multi-stage Docker builds for minimal image sizes:

**Golang Services (~15MB final image):**
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o app cmd/server/main.go

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /app/app /usr/local/bin/app
EXPOSE 8001
USER nobody
CMD ["app"]
```

**Java Services (~200MB final image):**
```dockerfile
FROM maven:3.9-eclipse-temurin-21 AS builder
WORKDIR /app
COPY pom.xml .
RUN mvn dependency:go-offline -B
COPY src ./src
RUN mvn clean package -DskipTests -B

FROM eclipse-temurin:21-jre-alpine
COPY --from=builder /app/target/*.jar app.jar
EXPOSE 8081
USER nobody
ENTRYPOINT ["java", "-XX:+UseContainerSupport", "-XX:MaxRAMPercentage=75.0", "-XX:+UseZGC", "-jar", "app.jar"]
```

### 13.2 Docker Compose (Local Development)

| Container | Image | Ports | Health Check |
|---|---|---|---|
| nginx | nginx:1.25-alpine | 80 | `curl localhost:80/health` |
| postgres | postgres:15-alpine | 5432 | `pg_isready` |
| redis | redis:7-alpine | 6379 | `redis-cli ping` |
| zookeeper | confluentinc/cp-zookeeper:7.5.0 | 2181 | — |
| kafka | confluentinc/cp-kafka:7.5.0 | 9092 | — |
| user-service | ./user-service | 8001 | `/health/ready` |
| product-service | ./product-service | 8081 | `/health/ready` |
| cart-service | ./cart-service | 8002 | `/health/ready` |
| order-service | ./order-service | 8082 | `/health/ready` |
| payment-service | ./payment-service | 8003 | `/health/ready` |

**Startup order:** postgres, redis → kafka → services → nginx

### 13.3 CI/CD Pipeline (GitHub Actions)

```
Pull Request                              Push to main
    │                                         │
    ▼                                         ▼
┌──────────┐                           ┌──────────┐
│   Lint   │ go vet, golangci-lint      │   Lint   │
│          │ checkstyle, spotbugs       │          │
└────┬─────┘                           └────┬─────┘
     ▼                                      ▼
┌──────────┐                           ┌──────────┐
│  Tests   │ Unit + Integration         │  Tests   │
│  70%+    │ (TestContainers)           │  70%+    │
└────┬─────┘                           └────┬─────┘
     ▼                                      ▼
┌──────────┐                           ┌──────────┐
│ Security │ govulncheck, OWASP         │  Build   │ Docker images
│  Scan    │                            │          │
└──────────┘                           └────┬─────┘
                                            ▼
                                       ┌──────────┐
                                       │  Deploy  │ (cloud — see §13.4)
                                       └──────────┘
```

### 13.4 Cloud Deployment — AWS & GCP Scenarios

> **Is AWS free?** AWS has a **12-month Free Tier** for new accounts with limited resources. It's not permanently free — after 12 months, you pay full price. GCP offers **$300 in credits for 90 days** for new accounts, plus some "Always Free" resources. Neither is truly free for running 5 microservices long-term, but both are very affordable.

#### Scenario A: AWS Deployment (Free Tier → ~$0–30/month first year)

```
┌─────────────────────────────────────────────────────────────┐
│                     AWS Architecture                         │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              EC2 t2.micro (Free Tier)                 │   │
│  │              1 vCPU · 1GB RAM · Docker Compose        │   │
│  │  ┌─────┐ ┌──────┐ ┌─────┐ ┌──────┐ ┌───────┐       │   │
│  │  │User │ │Prod. │ │Cart │ │Order │ │Payment│       │   │
│  │  │:8001│ │:8081 │ │:8002│ │:8082 │ │:8003  │       │   │
│  │  └─────┘ └──────┘ └─────┘ └──────┘ └───────┘       │   │
│  │  ┌──────────┐  ┌────────────────────────┐            │   │
│  │  │  Nginx   │  │  Kafka+Zookeeper       │            │   │
│  │  │  :80     │  │  (self-hosted in Docker)│            │   │
│  │  └──────────┘  └────────────────────────┘            │   │
│  └──────────────────────────────────────────────────────┘   │
│                          │                                   │
│  ┌───────────────────────▼──────────────────────────────┐   │
│  │  RDS db.t3.micro PostgreSQL (Free Tier)               │   │
│  │  20GB storage · 750 hours/month free                  │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  ElastiCache cache.t3.micro Redis (Free Tier)         │   │
│  │  750 hours/month free                                 │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  Security Group: SSH (your IP), HTTP/HTTPS (0.0.0.0/0)      │
└─────────────────────────────────────────────────────────────┘
```

| AWS Resource | Free Tier | After Free Tier | Notes |
|---|---|---|---|
| EC2 t2.micro | 750 hrs/month (12 months) | ~$8.50/month | 1 vCPU, 1GB RAM — tight for 5 services |
| EC2 t3.small (recommended) | NOT free | ~$15/month | 2 vCPU, 2GB RAM — comfortable for Docker Compose |
| RDS PostgreSQL db.t3.micro | 750 hrs/month (12 months) | ~$13/month | 20GB SSD, automated backups |
| ElastiCache Redis cache.t3.micro | 750 hrs/month (12 months) | ~$12/month | 0.5GB, single-node |
| MSK (Kafka) | ❌ NOT free | ~$100+/month | Too expensive — self-host Kafka on EC2 instead |
| **Total (first year)** | **~$0–15/month** | **~$48/month** | Self-host Kafka to stay cheap |

**AWS Deployment Steps:**
```bash
# 1. Launch EC2 instance (t3.small recommended)
# 2. SSH into instance
ssh -i key.pem ec2-user@<public-ip>

# 3. Install Docker & Docker Compose
sudo yum update -y && sudo yum install docker -y
sudo service docker start
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose

# 4. Clone repo and start
git clone https://github.com/hungCS22hcmiu/ecommrece-system.git
cd ecommrece-system

# 5. Update .env to point to RDS and ElastiCache (if using managed services)
# Or just use Docker Compose with all containers on the EC2 instance

docker-compose up --build -d
```

#### Scenario B: GCP Deployment (Free Credits → ~$0–25/month for 90 days)

```
┌─────────────────────────────────────────────────────────────┐
│                     GCP Architecture                         │
│                                                              │
│  Option 1: Single VM (like AWS)                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Compute Engine e2-small ($13/month, covered by $300) │   │
│  │  2 vCPU · 2GB RAM · Docker Compose                    │   │
│  │  (Same layout as AWS — all containers on one VM)      │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  Option 2: Cloud Run (Serverless — more impressive)          │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Cloud Run Services (2M free requests/month)          │   │
│  │  ┌─────┐ ┌──────┐ ┌─────┐ ┌──────┐ ┌───────┐       │   │
│  │  │User │ │Prod. │ │Cart │ │Order │ │Payment│       │   │
│  │  │ Run │ │ Run  │ │ Run │ │ Run  │ │ Run   │       │   │
│  │  └─────┘ └──────┘ └─────┘ └──────┘ └───────┘       │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Cloud SQL PostgreSQL ($7/month, smallest)            │   │
│  │  Memorystore Redis ($30/month) or self-host on VM     │   │
│  │  Pub/Sub (instead of Kafka — simpler, serverless)     │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

| GCP Resource | Free Tier / Credits | Cost After Credits | Notes |
|---|---|---|---|
| $300 credit (90 days) | Covers everything below | — | Great for initial development |
| e2-micro | Always free (1 instance) | $0 | 0.25 vCPU, 1GB — too small for 5 services |
| e2-small | Covered by credits | ~$13/month | 2 vCPU, 2GB — good for Docker Compose |
| Cloud Run | 2M requests/month free | Pay-per-use | Serverless — auto-scales to zero |
| Cloud SQL (PostgreSQL) | Covered by credits | ~$7/month | Smallest instance |
| Memorystore (Redis) | Covered by credits | ~$30/month | Expensive — consider self-hosting |
| Pub/Sub | 10GB/month free | Pay-per-use | Alternative to Kafka (simpler, managed) |
| **Total (with credits)** | **~$0/month** | **~$50/month** | Cloud Run is cheapest long-term |

**Recommendation for this project:**
| Criteria | AWS | GCP | Winner |
|---|---|---|---|
| Cheapest first year | $0–15/month (free tier) | $0 for 90 days ($300 credit) | 🏆 GCP (first 3 months) |
| Cheapest long-term | ~$48/month | ~$50/month (VM) or ~$30/month (Cloud Run) | 🏆 GCP Cloud Run |
| Simplest deployment | Docker Compose on EC2 | Docker Compose on Compute Engine | Tie |
| Most impressive for interview | ECS/EKS (complex but looks good) | Cloud Run (serverless) | 🏆 GCP Cloud Run |
| Kafka support | MSK ($100+) or self-host | Pub/Sub (managed, cheaper) | 🏆 GCP Pub/Sub |

**Our recommendation:** Start with **Docker Compose on a single VM** (works on both AWS and GCP). For the interview demo, deploy to **GCP Cloud Run** — it's serverless, auto-scales, and sounds impressive.

---

## 14. Testing Strategy

### 14.1 Test Pyramid

| Level | Coverage | Tools | What to Test |
|---|---|---|---|
| **Unit** | 70%+ | Go: `testify`; Java: `JUnit 5 + Mockito` | Business logic, validation, state transitions, edge cases |
| **Integration** | Critical paths | Go: `testcontainers-go`; Java: `TestContainers + @SpringBootTest` | DB queries, Redis ops, Kafka pub/sub, HTTP clients |
| **Concurrency** | Race conditions | Go: `-race` flag; Java: `ExecutorService` + `CountDownLatch` | Stock contention, cart updates, payment idempotency |
| **E2E** | Happy + error paths | Postman/Newman, k6 | Full user journeys through Nginx |

### 14.2 Critical Test Scenarios

| Scenario | Type | What It Proves |
|---|---|---|
| Register → Login → Access Profile | Integration | JWT auth flow, Redis session, bcrypt |
| Product CRUD + cache hit / cache miss | Integration | Redis cache-aside pattern, eviction |
| Add to cart → price changes → checkout detects stale price | Integration | Price snapshot vs re-validation |
| **200 goroutines reserve 100 stock** | **Concurrency** | **Optimistic locking, no overselling** |
| **Simultaneous pay + cancel** | **Concurrency** | **Pessimistic lock on order state** |
| **Duplicate Kafka event** | **Concurrency** | **Idempotency key prevents double charge** |
| **Concurrent cart updates** | **Concurrency** | **Redis WATCH/MULTI prevents lost updates** |
| Order → Payment success → stock confirmed | E2E | Full Kafka saga, idempotency |
| Order → Payment failure → stock released | E2E | Compensation logic |
| Circuit breaker opens after Product Service failure | Integration | Fallback behavior |
| Rate limiting (burst 150 requests) | Integration | Nginx rejects excess with 429 |
| Expired JWT rejected, blacklisted JWT rejected | Unit | Token validation completeness |

### 14.3 Test Data

- Seed scripts in `script/` generate: 100 products, 50 users, 10 categories, realistic stock levels
- Idempotent — safe to run multiple times (`INSERT ... ON CONFLICT DO NOTHING`)
- Separate seed for dev (small dataset) and load-test (10K products, 1K users)

---

## 15. Monitoring & Observability

### 15.1 Structured Logging

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

- **Correlation ID:** Generated at Nginx, propagated via `X-Correlation-ID` header through all services
- **Log levels:** ERROR (actionable failures), WARN (degraded but functional), INFO (key business events), DEBUG (development only)

### 15.2 Health Check Dashboard

Every service exposes `/health/live` and `/health/ready`. Nginx aggregates:

| Endpoint | Purpose |
|---|---|
| `GET /health` | Aggregated health of all services (Nginx upstream checks) |
| `GET /health/live` per service | Process alive |
| `GET /health/ready` per service | Dependencies reachable |

### 15.3 Key Metrics to Track

| Metric | Source | Alert Threshold |
|---|---|---|
| Request latency (p50, p95, p99) | Application logs | p99 > 2s for 5 min |
| Error rate (5xx) | Nginx access logs | > 1% in 5 min |
| Kafka consumer lag | Kafka consumer metrics | > 10,000 messages |
| Redis hit/miss ratio | Redis INFO stats | < 70% hit rate |
| DB connection pool utilization | Application metrics | > 80% for 5 min |
| Stock reservation conflicts | Application logs (OptimisticLockException count) | > 10/min (high contention) |
| Goroutine count (Go) | pprof `/debug/pprof/goroutine` | > 10,000 (potential leak) |
| GC pause time (Java) | JFR / JVM metrics | > 100ms (needs GC tuning) |

---

## 16. Future Enhancements

Out of scope for the initial release, prioritized by impact:

| Priority | Enhancement | Description | Effort |
|---|---|---|---|
| 🔴 P1 | **Minimal Frontend** | React/Vite: Login, Product list, Cart, Checkout (4 pages) | 3–4 days |
| 🔴 P1 | **Cloud Deployment** | Deploy to GCP Cloud Run or AWS EC2 | 2 days |
| 🟡 P2 | **Elasticsearch** | Replace PostgreSQL full-text search for better relevance | 2–3 days |
| 🟡 P2 | **WebSocket Notifications** | Real-time order status updates to client | 2 days |
| 🟡 P2 | **GCP Pub/Sub** | Replace Kafka with managed Pub/Sub for serverless deployment | 1–2 days |
| 🟢 P3 | **Admin Dashboard** | Product/order management UI | 3–4 days |
| 🟢 P3 | **Prometheus + Grafana** | Metrics collection and dashboards | 2 days |
| 🟢 P3 | **GraphQL API Layer** | Alternative to REST for flexible client queries | 2–3 days |
| ⚪ P4 | **Service Mesh (Istio)** | Advanced traffic management, mTLS | 2–3 days |
| ⚪ P4 | **Kubernetes (K8s)** | Container orchestration for production scale | 3–4 days |

---

## Appendix A: Project Structure

```
ecommerce-system/
├── user-service/           # Golang — authentication, profiles
├── product-service/        # Java/Spring Boot — catalog, search, inventory
├── cart-service/           # Golang — shopping cart (Redis-backed)
├── order-service/          # Java/Spring Boot — order lifecycle, notifications
├── payment-service/        # Golang — payment processing, idempotency
├── nginx/
│   └── nginx.conf          # Reverse proxy configuration
├── script/
│   ├── init-databases.sql  # Create 5 logical databases
│   ├── seed-data.sql       # Test data (products, users, categories)
│   └── loadtest/           # k6 load test scripts
├── docs/
│   ├── requirements.md
│   ├── architecture.md
│   ├── data-flows.md
│   ├── use-case.md
│   ├── load-test-results.md
│   └── adr/
│       ├── proposal.md     # ← This document
│       └── preview.md      # Implementation roadmap
├── api/
│   └── openapi.yaml        # Full API specification
├── docker-compose.yml
└── README.md
```

## Appendix B: Environment Setup

### Prerequisites

| Tool | Version | Install |
|---|---|---|
| Golang | 1.21+ | `brew install go` |
| Java | 17 or 21 LTS | `brew install openjdk@21` |
| Docker | 24+ | `brew install --cask docker` |
| Docker Compose | 2.x | Included with Docker Desktop |

### Quick Start

```bash
git clone https://github.com/hungCS22hcmiu/ecommrece-system.git
cd ecommrece-system
docker-compose up --build    # Starts everything
# Access API at http://localhost/api/v1/
# Access Swagger UI at http://localhost/swagger
```

### Verification

```bash
# Check all services are healthy
curl http://localhost/health

# Test auth flow
curl -X POST http://localhost/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"SecurePass123!"}'
```

## Appendix C: Concurrency Decision Matrix

| Situation | Optimistic Lock | Pessimistic Lock | Redis Atomic | Idempotency Key |
|---|---|---|---|---|
| Stock reservation (normal) | ✅ Primary | Fallback (flash sale) | — | — |
| Order state transition | — | ✅ Primary | — | — |
| Cart update | — | — | ✅ WATCH/MULTI | — |
| Payment processing | — | — | — | ✅ Primary |
| Login attempt counter | — | ✅ SELECT FOR UPDATE | ✅ Redis INCR | — |
| Cache operations | — | — | ✅ Built-in | — |

---

*This proposal is a living document. All architectural decisions are recorded as ADRs in `docs/adr/`. Version 3.0 adds dedicated concurrency/race condition analysis, cloud deployment scenarios, and language-specific performance optimization.*
