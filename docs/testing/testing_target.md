# System Testing Targets & Performance Benchmarks

This document defines the performance, resilience, and consistency targets for the e-commerce system running on a local development environment (Mac Pro M1).

## 1. Service Performance Targets (M1 Pro Baseline)

| Service | Key Operation | Dependencies | Latency (P95) | Target Throughput |
| :--- | :--- | :--- | :--- | :--- |
| **User (Go)** | `/auth/login` | CPU (Bcrypt), Redis | < 300ms | 100 RPS |
| **Cart (Go)** | `POST /cart/items` | Product SVC (Sync) | < 40ms | 500 RPS |
| **Product (Java)**| `GET /search` | AI Service (Python) | < 150ms | 150 RPS |
| **Order (Java)** | `POST /orders` | DB Txn, Product RPC | < 400ms | 50 RPS |
| **Payment (Go)** | Kafka Consumer | Async Processing | < 100ms | 200 msg/s |

## 2. Distributed System Consistency (Saga)

The system uses a Choreography-based Saga. Success is measured by "Time to Consistency" (TTC).

| Scenario | Expected Outcome | Target TTC (P95) |
| :--- | :--- | :--- |
| **Happy Path** | Order PENDING → CONFIRMED | < 2.0s |
| **Payment Failure**| Order PENDING → CANCELLED | < 1.5s |
| **Compensation** | Stock Restored after Failure | < 2.0s |

## 3. High-Concurrency & Failure Mode Targets

### A. Concurrent User Capacity
*   **Target:** The system must handle **50 concurrent Virtual Users (VUs)** performing full checkout flows without a jump in error rates.
*   **Threshold:** Error rate < 0.1% at 50 VUs; P95 response time < 1.0s.

### B. Race Condition (Inventory Integrity)
*   **Mechanism:** Product Service uses **Optimistic Locking** with 3 retries.
*   **Testing Scenario:** 10 concurrent users attempting to buy the *exact same* last item.
*   **Target:** Exactly 1 successful order; 9 rejected with `409 Conflict` (after retries exhausted). Stock quantity must remain exactly 0 (no "ghost" stock).

### C. Service Deadlock (Order State Machine)
*   **Mechanism:** Order Service uses **Pessimistic Locking** (`SELECT FOR UPDATE`) on the `orders` table.
*   **Testing Scenario:** Rapidly sequence Kafka events (e.g., `PAYMENT_COMPLETED` and `SHIPPED`) for the same order ID from multiple threads.
*   **Target:** Zero `DeadlockLoserDataAccessException` or `500 Internal Server Error`. The state machine must process events sequentially or reject overlapping transitions gracefully.

## 4. AI Semantic Search Targets

The AI search feature involves the Python AI Service (embeddings) and Postgres `pgvector` (similarity search).

### A. Search Latency Breakdown
*   **Total P95 Latency:** < 250ms for `/api/v1/products/ai-search`.
*   **Target (AI Service):** Embedding generation < 100ms.
*   **Target (Database):** Vector similarity search (`<=>` operator) < 50ms.
*   **Target (App):** Re-ranking logic < 30ms.

### B. Searchability Lag (Consistency)
*   **Definition:** Time elapsed between a product `POST/PUT` and its availability in `/ai-search`.
*   **Mechanism:** Async `@Async` call to AI service followed by a SQL `UPDATE`.
*   **Target:** P95 < 1.0s.

### C. Resource & Cold Start
*   **Cold Start:** AI Service must be healthy (`/health/ready`) within 15s of startup (model loading).
*   **Memory Limit:** AI Service container should not exceed **1.5GB RAM** under load.
*   **Throughput:** Handle **20 RPS** for semantic search on M1 Pro.

## 5. Infrastructure Targets

*   **Redis Latency:** < 0.2ms (Check using `redis-cli --latency`)
*   **Kafka Lag:** < 50 messages during peak load.
*   **Postgres Connections:** < 20 active connections per service (pooled).

## 6. Database Connection Pooling Targets

The system uses separate connection pools per service. All services share a single PostgreSQL instance.

### A. Pool Configuration Limits
| Service Tier | Max Open Conns | Min Idle Conns | Timeout |
| :--- | :--- | :--- | :--- |
| **Go Services** | 25 | 5 | N/A (GORM) |
| **Java Services**| 20 | 5 | 30s (HikariCP) |

### B. Acquisition & Leakage Targets
*   **Connection Acquisition Latency:** P95 < 5ms (Time to get a connection from the pool).
*   **Leak Detection:** Zero "Connection Leak" warnings in Java logs (`leak-detection-threshold` is set to 60s).
*   **Pool Exhaustion:** Under 100 concurrent requests to a single service, the system must not exceed its configured `Max Open Conns`.

### C. Total Infrastructure Load
*   **Global Postgres Connections:** Total active connections across all 5 databases must not exceed **150** (to avoid M1 disk I/O thrashing).
*   **Redis Pool Efficiency:** P99 acquisition < 1ms for the Redis pool (20 conns per service).

## 7. Resilience & Observability Targets

### A. Circuit Breaker & Degraded Mode
*   **Scenario:** `product-service` is down (HTTP 503).
*   **Target:** `cart-service` must transition to OPEN state after 5 failures.
*   **Degraded Performance:** `GET /cart` must still return 200 OK (from Redis) within **< 20ms**, even while the product dependency is failing.

### B. Saga Idempotency
*   **Scenario:** Kafka delivers the `orders.created` event multiple times to the `payment-service`.
*   **Target:** Exactly **one** payment record created; subsequent attempts must return `ErrDuplicateIdempotencyKey` and not trigger duplicate Saga flows.

### C. Observability (Correlation IDs)
*   **Requirement:** All logs across Nginx, Go, and Java must include the `X-Correlation-ID`.
*   **Target:** **100% propagation.** A single trace ID must be trackable from Nginx -> Order Svc -> Kafka -> Payment Svc.

### D. Rate Limiting (Nginx)
*   **General API:** 10 req/s per IP. Target: **429 Too Many Requests** on 11th request.
*   **Auth (Brute Force):** 5 req/min. Target: **429 Too Many Requests** on 6th request.

## 8. Frontend UX & Reliability Targets

Currently, the frontend relies on manual validation. Automated targets are needed to ensure consistent error handling and state management.

### A. Error Communication
*   **Target:** 100% of backend error responses (`{ "success": false, "error": { ... } }`) must be captured and displayed via a Toast or Inline Alert.
*   **Validation:** Trigger a `400 Bad Request` (e.g., invalid password) and verify the specific backend message "Password must be at least 8 characters" is visible, not a generic "An error occurred."

### B. State Consistency (Optimistic Updates)
*   **Mechanism:** Cart uses TanStack Query optimistic updates.
*   **Scenario:** User clicks "Add to Cart" while offline or if the backend returns an error.
*   **Target:** Navbar badge must increment instantly, then **rollback** to the previous value within 2s of the failed API call.

### C. Authentication Flow
*   **Scenario:** Access token expires (401) during a multi-component page load.
*   **Target:** `axios` interceptor must successfully refresh the token and replay **all** failed requests without the user noticing.
*   **Validation:** Zero "Login" redirects if a valid refresh token exists.

### D. Component Integrity
*   **Target:** Core UI components (Buttons, Inputs, Modals) must maintain 100% accessibility (Aria labels) and responsive behavior down to 320px width.

## 9. Codebase Coverage & Unit Testing Targets

Based on a comprehensive audit, the following areas represent "Testing Debt" that must be resolved to protect core business logic.

### A. Repository & Data Integrity
*   **Target:** 100% unit test coverage for Repository methods involving custom SQL or Locking.
*   **Debt:** `ProductRepository` (Native FTS/Vector SQL), `OrderRepository` (Pessimistic Locking), `PaymentRepository` (Idempotency logic).

### B. External Communication & Clients
*   **Target:** Mock-based unit tests for all RPC and Messaging clients.
*   **Debt:** `Cart Service: ProductClient` (Circuit Breakers/Retries), `Payment Service: Kafka Consumer`, `Product Service: EmbeddingClient`.

### C. Security & Utility logic (`user-service/pkg`)
*   **Target:** 100% coverage for security-sensitive utilities.
*   **Debt:** `pkg/blacklist` (Revocation), `pkg/verification` (Email codes), `pkg/reset` (Password tokens).

## 10. Verification Roadmap

1.  **Phase 1 (Functional):** Verify Saga compensation (Payment fail -> Stock restore).
2.  **Phase 2 (Load):** Stress test `POST /orders` with 50 Virtual Users.
3.  **Phase 3 (Resilience):** Kill a service instance mid-transaction and verify recovery.
4.  **Phase 4 (Integrity):** Resolve high-priority "Testing Debt" in Repositories and Security PKGs.
