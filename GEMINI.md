# GEMINI.md

## Project Overview
This is a high-throughput microservices-based e-commerce platform designed with a focus on concurrency, resilience, and scalability. It employs a polyglot architecture, using Go for I/O-intensive services and Java (Spring Boot) for complex business logic, all coordinated via synchronous REST APIs and asynchronous Kafka events (Choreography Saga pattern).

### Core Architecture
- **User Service (Go 1.25.3):** Handles authentication, profiles, and addresses using Gin and GORM.
- **Product Service (Java 21):** Manages the product catalog, search, and inventory using Spring Boot 3.5.11.
- **Cart Service (Go 1.25.3):** Manages shopping carts with Redis as the primary store.
- **Order Service (Java 21):** Orchestrates the order lifecycle and state machine transitions using Spring Boot 3.5.11 and Kafka.
- **Payment Service (Go 1.25.3):** Processes payments and integrates with Kafka for the Saga pattern.
- **AI Service (Python):** (Planned/Architectural Design) Provides vector embeddings for product search.
- **Nginx:** Acts as the entry point for all client traffic, handling routing, rate limiting, and TLS.

### Tech Stack
- **Databases:** PostgreSQL 15 (5 logical databases), Redis 7 (caching/sessions).
- **Messaging:** Apache Kafka 3.x (Saga orchestration) with Zookeeper 7.5.
- **Communication:** REST (JSON) via Nginx; Kafka for async flows.
- **Security:** RS256 JWT (15-min access, 7-day refresh), bcrypt hashing.

---

## Building and Running

### Prerequisites
- Docker & Docker Compose 2.x
- Go 1.25+
- Java 21 LTS

### Key Commands
- **Environment Setup:** `make env` (copies `.env.example` to `.env`)
- **Infrastructure:** `make infra-up` (starts PostgreSQL and Redis)
- **Full Stack:** `make up` (starts all services and infrastructure)
- **Service-Specific:** `docker compose up -d <service-name>`
- **Database Reset (User Service):** `make db-reset-user`
- **Seed Data:** `make db-seed` (inserts sample users)

### Local Development (Outside Docker)
- **Go Services:** `cd <service> && go run ./cmd/server/main.go`
- **Java Services:** `cd <service> && ./mvnw spring-boot:run`

### Testing
- **Go Unit Tests:** `make test-user` (runs with race detector)
- **Go Integration:** `make test-integration-user` (requires infra-up)
- **Java Tests:** `./mvnw test`

---

## Development Conventions

### API Standards
- **Response Envelope:** All responses follow a `{ "success": boolean, "data": { ... }, "error": { ... } }` structure.
- **Status Codes:** Strict adherence to RESTful principles (200/201/204 for success, 400/401/403/404/409 for client errors).
- **UUIDs:** All primary keys use UUID v4.

### Coding Practices
- **Go:**
  - Dependency wiring in `main.go` (db -> repository -> service -> handler).
  - Services depend on repository **interfaces**.
  - Context propagation is mandatory throughout the call chain.
- **Java:**
  - Constructor injection is required for Spring beans.
  - `@Transactional` at the service layer.
  - Flyway for versioned database migrations.

### Logging & Observability
- **Structured Logging:** All services emit JSON logs.
- **Correlation IDs:** Propagated via `X-Correlation-ID` headers across service boundaries and Kafka events.
- **Health Checks:** Every service provides `/health/live` and `/health/ready` endpoints.

### Distributed Transactions (Saga)
- **Pattern:** Choreography-based Saga using Kafka.
- **Flow:** `Order Created -> Payment Processed -> Order Confirmed/Cancelled`.
- **Compensation:** Failure at any step triggers automated compensation events to maintain consistency.
