# Order Service: Core + State Machine — Implementation Plan

## Context

The order-service is a Java/Spring Boot 3.5 (Java 21) microservice responsible for order lifecycle
management. It uses pessimistic locking (`SELECT ... FOR UPDATE`) to prevent concurrent state
transitions — critical for the payment Kafka saga where two events could race to change the same
order. This plan covers Phase 1: Core + State Machine (Week 7 of the six-month plan).

### What was already scaffolded
| File | Status |
|---|---|
| `V1__baseline_schema.sql` | Done — 4 tables + enums + indexes |
| `application.yaml` | Done — DB, Redis, Kafka, product-service URL |
| `config/AsyncConfig.java` | Done — thread pool for `@Async` |
| `controller/HealthController.java` | Done — GET `/health/live` |
| `pom.xml` / `Dockerfile` | Done |

---

## Implemented Package Structure

```
src/main/java/com/ecommerce/order_service/
├── OrderServiceApplication.java       (added @EnableJpaAuditing)
├── model/
│   ├── OrderStatus.java               enum: PENDING, CONFIRMED, CANCELLED, SHIPPED, DELIVERED
│   ├── Order.java                     JPA entity, UUID PK, @CreatedDate/@LastModifiedDate
│   ├── OrderItem.java                 JPA entity, ManyToOne → Order
│   ├── OrderStatusHistory.java        JPA entity, BIGSERIAL PK, audit trail
│   ├── ShippingAddress.java           @Embeddable POJO
│   └── ShippingAddressConverter.java  AttributeConverter → JSONB column
├── repository/
│   ├── OrderRepository.java           findByIdWithLock (@Lock PESSIMISTIC_WRITE) + paginated list
│   ├── OrderItemRepository.java       plain JpaRepository
│   └── OrderStatusHistoryRepository.java  findByOrderIdOrderByChangedAtAsc
├── dto/
│   ├── ApiResponse.java               generic { success, data, meta, error } wrapper
│   ├── CreateOrderRequest.java        @NotNull cartId, @NotEmpty items, @Valid address
│   ├── OrderItemRequest.java          productId + @Min(1) quantity
│   ├── ShippingAddressDto.java        @NotBlank fields
│   ├── OrderItemResponse.java         + computed subtotal
│   ├── OrderResponse.java             full order detail with items list
│   ├── OrderSummaryResponse.java      id, totalAmount, status, itemCount, createdAt
│   └── OrderStatusHistoryResponse.java
├── exception/
│   ├── OrderNotFoundException.java    → 404 ORDER_NOT_FOUND
│   ├── OrderAccessDeniedException.java → 403 ACCESS_DENIED
│   ├── InvalidOrderStateException.java → 409 INVALID_STATE_TRANSITION
│   ├── InsufficientStockException.java → 409 INSUFFICIENT_STOCK
│   └── GlobalExceptionHandler.java    @RestControllerAdvice, mirrors product-service
├── service/
│   ├── OrderService.java              interface (6 methods)
│   ├── OrderStateMachine.java         @Component, validates transitions via Map<Status, Set<Status>>
│   └── impl/
│       └── OrderServiceImpl.java      business logic
├── client/
│   └── ProductServiceClient.java      RestTemplate → product-service /api/v1/inventory
├── kafka/
│   ├── event/
│   │   ├── OrderCreatedEvent.java     outbound: orderId, userId, totalAmount, items[]
│   │   ├── PaymentCompletedEvent.java inbound: orderId, paymentId, amount
│   │   └── PaymentFailedEvent.java    inbound: orderId, reason
│   ├── OrderEventProducer.java        publishes to "orders.created"
│   └── PaymentEventConsumer.java      @KafkaListener for payments.completed / payments.failed
└── config/
    ├── AsyncConfig.java               (existing)
    ├── KafkaConfig.java               NewTopic beans: orders.created, payments.*
    └── RestTemplateConfig.java        @Bean RestTemplate
```

---

## State Machine

```
PENDING ──(payment ok)──→ CONFIRMED ──(seller ships)──→ SHIPPED ──(delivered)──→ DELIVERED
   │                           │
   └──(payment fail / user)──→ CANCELLED (terminal)
                           └──(user cancel)──→ CANCELLED
```

Terminal states: `DELIVERED`, `CANCELLED` — no transitions out.

Implemented in `OrderStateMachine.java`:
```java
Map.of(
    PENDING,   Set.of(CONFIRMED, CANCELLED),
    CONFIRMED, Set.of(SHIPPED,   CANCELLED),
    SHIPPED,   Set.of(DELIVERED)
)
```

---

## Pessimistic Locking Pattern

`updateOrderStatus` in `OrderServiceImpl` always calls `findByIdWithLock`:
```java
@Transactional
public OrderResponse updateOrderStatus(UUID orderId, OrderStatus newStatus, ...) {
    Order order = orderRepository.findByIdWithLock(orderId)   // SELECT ... FOR UPDATE
        .orElseThrow(() -> new OrderNotFoundException(orderId));
    stateMachine.validateTransition(order.getStatus(), newStatus);
    order.setStatus(newStatus);
    orderRepository.save(order);
    historyRepository.save(new OrderStatusHistory(...));
    return OrderResponse.from(order);
}
```

Two concurrent transitions on the same order: the second thread blocks at the lock, then reads the
already-updated status, and `validateTransition` throws `InvalidOrderStateException` → 409.

---

## REST Endpoints

All require `X-User-Id: <UUID>` header (gateway-forwarded identity).

| Method | Path | Status | Auth |
|---|---|---|---|
| POST | `/api/v1/orders` | 201 | owner only |
| GET | `/api/v1/orders/{id}` | 200 | owner only |
| GET | `/api/v1/orders` | 200 | paginated, owner's orders |
| PUT | `/api/v1/orders/{id}/cancel` | 200 | owner only |
| PUT | `/api/v1/orders/{id}/ship` | 200 | seller/admin |
| PUT | `/api/v1/orders/{id}/deliver` | 200 | admin |
| GET | `/api/v1/orders/{id}/history` | 200 | owner only |

---

## Kafka Topics

| Topic | Direction | Trigger |
|---|---|---|
| `orders.created` | Produced | POST /api/v1/orders |
| `payments.completed` | Consumed | → CONFIRMED transition |
| `payments.failed` | Consumed | → CANCELLED transition |

---

## Known TODOs (next iteration)

1. **Price snapshot**: `fetchUnitPrice()` in `OrderServiceImpl` currently returns `BigDecimal.ZERO`.
   It should call `GET /api/v1/products/{id}` on the product-service and snapshot the price at
   order time. This is intentionally deferred — the cart already holds the agreed price.

2. **Product name snapshot**: `OrderItem.productName` is currently set to `"Product {id}"`.
   Should be populated from the product-service response.

3. **Notification dispatcher**: `@Async` email notifications on status transitions (Week 8).

4. **Integration tests**: Concurrency test — two threads competing on `CONFIRMED → SHIPPED` vs
   `CONFIRMED → CANCELLED`, verify exactly one succeeds with real Postgres.

---

## Verification

```bash
# 1. Build
cd order-service && ./mvnw package -DskipTests

# 2. Start infra
docker compose up -d postgres redis kafka

# 3. Run service
./mvnw spring-boot:run

# 4. Smoke test
curl http://localhost:8082/health/live
# → {"status":"UP"}

# 5. Create order
curl -X POST http://localhost:8082/api/v1/orders \
  -H "X-User-Id: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{
    "cartId": "00000000-0000-0000-0000-000000000001",
    "items": [{"productId": 1, "quantity": 2}],
    "shippingAddress": {
      "street": "123 Main St", "city": "HCMC",
      "state": "HCM", "country": "VN", "zipCode": "70000"
    }
  }'
# → {"success":true,"data":{"id":"...","status":"PENDING",...}}

# 6. Concurrency test (once written)
./mvnw test -Dtest=OrderConcurrencyTest
```
