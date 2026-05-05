# order-service: How It Works

---

## 1. What Is It?

The `order-service` is a Java/Spring Boot microservice that manages the **order lifecycle** for the ecommerce platform — from creation through fulfilment to cancellation.

**Analogy:** Think of it as a bank teller processing a withdrawal. The teller (order-service) receives your request, locks your account record (pessimistic lock), validates the state of your account (state machine), updates the balance (reserves stock), and records every step in a ledger (status history). If two tellers try to process the same account simultaneously, only one gets the lock — the second waits or fails. There's no "maybe both succeed" — correctness is non-negotiable for financial operations.

**Responsibilities:**
- Create orders with parallel stock reservation across multiple products (with compensation on partial failure)
- Enforce a strict state machine: `PENDING → CONFIRMED/CANCELLED → SHIPPED → DELIVERED`
- Pessimistic row-level locking on all state transitions — concurrent transitions on the same order are serialized, never duplicated
- Publish `orders.created` Kafka events to trigger the payment saga
- Consume `payments.completed` / `payments.failed` Kafka events to advance or roll back order state
- Append-only `order_status_history` for a complete audit trail of every transition

---

## 2. Why It Matters

### In this project
- The order service is where the distributed saga begins. It creates the order, reserves stock via synchronous REST calls to product-service, then fires a Kafka event to start the async payment flow. If payment fails, it receives the failure event and releases the reserved stock — the entire compensation chain is driven from here.
- Pessimistic locking on state transitions is the critical safety property. Without `SELECT FOR UPDATE`, two concurrent requests (e.g., `CONFIRMED → SHIPPED` and `CONFIRMED → CANCELLED`) can both read state=CONFIRMED, both pass the transition check, and both commit — leaving the order in an inconsistent terminal state. With the lock, exactly one wins.
- The `order_status_history` table is non-negotiable for customer support. Every state change is recorded with a timestamp and reason. "When did this order ship?" and "why was it cancelled?" are answerable without digging through logs.

### In real-world systems
- Pessimistic locking for order state transitions is the standard approach in financial and order management systems. The cost (lock wait under contention) is acceptable because the operation is rare (one transition per order per lifecycle stage) and correctness is paramount.
- The choreography saga pattern (Kafka events driving state changes across services) is used by Uber, Netflix, and Shopify for distributed transactions. Compared to orchestration (one coordinator driving all services), choreography decouples services — payment-service doesn't know about order-service's internal state.
- Parallel stock reservation with compensation (release-all-on-any-failure) mirrors the two-phase approach used in distributed transactions without requiring a two-phase commit protocol across services.

---

## 3. How It Works — Step-by-Step Flows

### Create Order (the critical path)
```
POST /api/v1/orders  (X-User-Id: <UUID>)  body:{cartId, items[], shippingAddress}
    │
    ├─ Parse + @Valid validate CreateOrderRequest
    │
    ├─ Parallel stock reservation (CompletableFuture per item):
    │     For each item in items[]:
    │       productServiceClient.reserveStock(productId, quantity, "order-{userId}")
    │         └─ POST product-service:8081/api/v1/inventory/{id}/reserve
    │     All futures joined: if ANY fails → compensation
    │       For each successfully reserved item: releaseStock(...)
    │       Throw InsufficientStockException → 409
    │
    ├─ Fetch product details (name, price) for each item:
    │     productServiceClient.getProduct(productId)
    │       └─ GET product-service:8081/api/v1/products/{id}
    │
    ├─ Compute order total (sum of quantity × unitPrice per item)
    ├─ Persist Order (status=PENDING) + OrderItems + initial OrderStatusHistory row
    │
    ├─ Publish Kafka event:
    │     orderEventProducer.publishOrderCreated(OrderCreatedEvent{
    │       orderId, userId, totalAmount, items[], idempotencyKey, timestamp
    │     })
    │     → topic: orders.created (3 partitions, partition key = userId)
    │
    └─ Return OrderResponse (201)
```

### State Transition (any PATCH that changes status)
```
PUT /api/v1/orders/{id}/cancel  (X-User-Id: <UUID>)
    │
    ├─ OrderServiceImpl.cancelOrder(orderId, userId)
    │
    ├─ orderRepository.findByIdWithLock(orderId)
    │     └─ SELECT * FROM orders WHERE id=? FOR UPDATE
    │        ← row is locked; concurrent requests wait here
    │
    ├─ Ownership check: order.userId == userId → else 403
    ├─ stateMachine.validateTransition(current, CANCELLED)
    │     └─ PENDING → CANCELLED: ✓
    │        SHIPPED  → CANCELLED: ✗ throws InvalidOrderStateException → 422
    │
    ├─ For each item: productServiceClient.releaseStock(productId, qty, orderId)
    │     └─ Failures swallowed (best-effort release, stock not stranded forever — audit reconciles)
    │
    ├─ order.status = CANCELLED
    ├─ orderStatusHistoryRepository.save(history row: PENDING→CANCELLED, reason, timestamp)
    ├─ orderRepository.save(order)  ← commits the lock
    └─ Return OrderResponse
```

### Kafka: Payment Event Consumer
```
Topic: payments.completed  (consumer group: order-service)
    │
    └─ PaymentEventConsumer.handlePaymentCompleted(PaymentCompletedEvent)
          └─ orderService.updateOrderStatus(event.orderId, CONFIRMED, "payment succeeded")
                ├─ findByIdWithLock → lock row
                ├─ PENDING → CONFIRMED: ✓
                └─ save + record history

Topic: payments.failed
    │
    └─ PaymentEventConsumer.handlePaymentFailed(PaymentFailedEvent)
          └─ orderService.updateOrderStatus(event.orderId, CANCELLED, "payment failed")
                ├─ findByIdWithLock → lock row
                ├─ For each item: releaseStock(...)
                ├─ PENDING → CANCELLED: ✓
                └─ save + record history
```

---

## 4. System Design — Components & Architecture

```
                ┌──────────────────────────────────────────────────────────────┐
                │                      order-service                            │
                │                                                               │
HTTP ───────────┤  OrderController                                              │
(X-User-Id hdr) │      │                                                       │
                │  OrderServiceImpl ◄──── OrderStateMachine                   │
                │      │                       (validates transitions)         │
                │      ├── OrderRepository (findByIdWithLock — FOR UPDATE)     │
                │      ├── OrderItemRepository                                 │
                │      ├── OrderStatusHistoryRepository                        │
                │      ├── ProductServiceClient (RestTemplate)                 │
                │      └── OrderEventProducer ─────────────────────────────►  │
                │                                                  Kafka        │
                │  PaymentEventConsumer ◄────────────────────────────────────  │
                └──────────────────────────────────────────────────────────────┘
                         │                              │
          ┌──────────────┴───────────┐     ┌───────────┴───────────────┐
          │        PostgreSQL         │     │  product-service:8081      │
          │                           │     │                            │
          │ orders (+ status)         │     │ POST /inventory/reserve    │
          │ order_items               │     │ POST /inventory/release    │
          │ order_status_history      │     │ GET  /products/{id}        │
          │   (append-only)           │     └────────────────────────────┘
          └───────────────────────────┘
```

### Kafka topics

| Topic | Direction | Partitions | Key |
|---|---|---|---|
| `orders.created` | order-service → payment-service | 3 | userId |
| `payments.completed` | payment-service → order-service | 3 | orderId |
| `payments.failed` | payment-service → order-service | 3 | orderId |

Partition key = `userId` for `orders.created` ensures ordering of events per user within a partition.

### Order state machine

```
                 ┌─────────────────────────────────────────┐
                 │                                          ▼
  [CREATE] ──► PENDING ──► CONFIRMED ──► SHIPPED ──► DELIVERED
                  │             │
                  ▼             ▼
              CANCELLED     CANCELLED
```

Valid transitions enforced by `OrderStateMachine`:
- `PENDING → CONFIRMED` (payment succeeded) or `PENDING → CANCELLED` (payment failed / user cancels)
- `CONFIRMED → SHIPPED` (fulfilment) or `CONFIRMED → CANCELLED` (exceptional)
- `SHIPPED → DELIVERED` (delivery confirmed)
- `DELIVERED` and `CANCELLED` are terminal — no further transitions allowed

### Data model

```
orders
  id               UUID PK (gen_random_uuid)
  user_id          UUID NOT NULL
  cart_id          UUID                      ← reference to originating cart
  total_amount     NUMERIC(12,2)
  status           order_status              ← state machine current state
  shipping_address JSONB                     ← serialized ShippingAddress value object
  created_at       TIMESTAMPTZ
  updated_at       TIMESTAMPTZ

order_items
  id          UUID PK
  order_id    UUID FK CASCADE
  product_id  BIGINT
  product_name VARCHAR                       ← snapshotted at order time
  quantity    INT
  unit_price  NUMERIC(12,2)                  ← snapshotted at order time

order_status_history (append-only)
  id          BIGSERIAL PK
  order_id    UUID FK
  old_status  order_status                   ← null for the initial PENDING entry
  new_status  order_status
  reason      VARCHAR
  changed_by  VARCHAR                        ← "user", "payment-service", "admin"
  changed_at  TIMESTAMPTZ
```

---

## 5. Code Examples

### Pessimistic lock on state transition

```java
// OrderRepository.java
@Lock(LockModeType.PESSIMISTIC_WRITE)
@Query("SELECT o FROM Order o WHERE o.id = :id")
Optional<Order> findByIdWithLock(@Param("id") UUID id);
// Generates: SELECT * FROM orders WHERE id=? FOR UPDATE
// Row is locked until the enclosing @Transactional method commits or rolls back.
```

```java
// OrderServiceImpl.java
@Transactional
public OrderResponse updateOrderStatus(UUID orderId, OrderStatus newStatus, String reason, String changedBy) {
    Order order = orderRepository.findByIdWithLock(orderId)  // acquires FOR UPDATE
        .orElseThrow(() -> new OrderNotFoundException(orderId));

    stateMachine.validateTransition(order.getStatus(), newStatus); // throws if invalid

    OrderStatus oldStatus = order.getStatus();
    order.setStatus(newStatus);
    orderRepository.save(order); // releases lock on TX commit

    orderStatusHistoryRepository.save(
        OrderStatusHistory.of(orderId, oldStatus, newStatus, reason, changedBy));
    return toResponse(order);
}
```

### Parallel stock reservation with compensation

```java
// OrderServiceImpl.java
List<CompletableFuture<Void>> reservations = items.stream()
    .map(item -> CompletableFuture.runAsync(() ->
        productServiceClient.reserveStock(item.getProductId(), item.getQuantity(), referenceId)))
    .toList();

List<OrderItemRequest> reserved = new ArrayList<>();
try {
    for (int i = 0; i < reservations.size(); i++) {
        reservations.get(i).join(); // blocks until this item's reservation completes
        reserved.add(items.get(i));
    }
} catch (CompletionException e) {
    // At least one reservation failed — release everything reserved so far
    reserved.forEach(item ->
        productServiceClient.releaseStock(item.getProductId(), item.getQuantity(), referenceId));
    throw new InsufficientStockException("Stock reservation failed: " + e.getMessage());
}
// All reservations succeeded → proceed to create order
```

### Kafka event publishing

```java
// OrderEventProducer.java
public void publishOrderCreated(OrderCreatedEvent event) {
    ProducerRecord<String, OrderCreatedEvent> record =
        new ProducerRecord<>("orders.created", event.getUserId().toString(), event);
    // partition key = userId → events from same user go to same partition → ordered
    kafkaTemplate.send(record)
        .whenComplete((result, ex) -> {
            if (ex != null) log.error("Failed to publish order.created for {}", event.getOrderId(), ex);
            else log.info("Published order.created for {} to partition {}",
                event.getOrderId(), result.getRecordMetadata().partition());
        });
}
```

---

## 6. Trade-offs

### Pessimistic vs. optimistic locking for order state transitions

| | Pessimistic (`FOR UPDATE`) | Optimistic (`@Version`) |
|---|---|---|
| Concurrent transitions | Serialized — second waiter blocks until first commits | Race — both may read old state, one commit wins, other retries |
| Correctness guarantee | Always — only one writer can be in the critical section | With retries — but a retry after `CONFIRMED → SHIPPED` could still attempt the same transition |
| Read performance | Blocked during write | Reads never blocked |
| **Our choice** | ✅ Order transitions are rare but catastrophic if duplicated | Fine for inventory (read-heavy, same outcome on retry) |

An order cannot be both SHIPPED and CANCELLED. The pessimistic lock guarantees it. Optimistic locking with a correct `@Version` could also work, but the state machine validation must happen inside the lock — and reasoning about retry semantics for state machines is harder than for simple increment/decrement.

### Choreography saga vs. orchestration

| | Choreography (our approach) | Orchestration (saga coordinator) |
|---|---|---|
| Coupling | Services react to events independently | Central orchestrator knows the full flow |
| Failure visibility | Hard — must trace events across topics | Easy — orchestrator tracks saga state |
| New service addition | Add consumer + events | Update orchestrator |
| **Our choice** | ✅ Simple 3-service saga; low coupling | Better for 5+ step sagas with complex rollback |

With only order → payment → order, choreography is simpler. If the saga grew to include shipping, notifications, invoicing, and loyalty points, an orchestrator (like Temporal or a custom saga state machine) would be easier to reason about.

### Synchronous stock reservation at order creation

Reserving stock synchronously (blocking the `POST /orders` response) means the user gets immediate confirmation that stock was secured. The alternative — reserve stock asynchronously via Kafka — would require polling or push notifications to tell the user whether their order succeeded. Synchronous is simpler and more reliable for small item counts. The parallel `CompletableFuture` approach means reserving 5 items takes the time of the slowest call, not the sum of all calls.

---

## 7. When to Use / Avoid

### Use this pattern when:
- **State machine correctness is critical**: orders, payments, shipments — any domain where "double-processing" causes real harm needs pessimistic locking.
- **Small-to-medium saga** (2–4 services): choreography via Kafka is clean and decoupled. Payment-service doesn't import any order-service code.
- **Parallel independent operations**: stock reservations for multiple items are independent — `CompletableFuture.runAsync` makes them genuinely parallel, not sequential.

### Avoid when:
- **High concurrent transitions on the same order**: the `FOR UPDATE` lock serializes all concurrent requests for the same row. If thousands of systems race to update a single order, use an event queue to serialize updates before hitting the DB.
- **Long-running saga with many services**: choreography becomes hard to debug at 5+ services. Missed events, retry storms, and partial compensation chains become operational nightmares. Use Temporal or AWS Step Functions.
- **You need exactly-once Kafka delivery**: this service currently relies on Kafka's at-least-once delivery with `updateOrderStatus` being idempotent via the state machine (transitioning an already-CONFIRMED order to CONFIRMED again is a no-op). If state machine idempotency isn't enforced, duplicate Kafka messages could corrupt order state.

---

## 8. Interview Insights

### Q: Why use `SELECT FOR UPDATE` for order state transitions instead of optimistic locking?

**A:** Order state transitions are low-frequency but high-stakes. Consider the race: two threads both read status=CONFIRMED, both validate that CONFIRMED → SHIPPED is allowed, and both commit. Now you have two SHIPPED history entries and an ambiguous final state. With `SELECT FOR UPDATE`, the second thread waits until the first commits. It then reads status=SHIPPED and the state machine rejects SHIPPED → SHIPPED, returning a clean error. Optimistic locking would require adding a `version` column and reasoning about whether a retry after `CONFIRMED → SHIPPED` is safe (it is, since the state machine would reject it) — but the reasoning is subtle and the cost of getting it wrong is high. Pessimistic locking is the conservative, obviously-correct choice for critical state machines.

### Q: Explain the parallel stock reservation with compensation. What does "compensation" mean?

**A:** Compensation is the distributed system equivalent of a rollback. When we create an order with 3 items, we fire 3 parallel HTTP calls to product-service to reserve stock. If the third call fails (product out of stock), the first two have already decremented `stock_reserved`. Without compensation, those two items would stay "reserved" forever — stock would appear unavailable to other buyers even though no order exists. The compensation logic releases them synchronously: for each successfully reserved item, we call `releaseStock`. This is the "undo" of a saga step. It's not a database rollback (we're in a distributed system) — it's a compensating transaction.

### Q: How does the choreography saga work end-to-end?

**A:** Order-service creates the order in PENDING state and publishes `orders.created` to Kafka. Payment-service consumes that event, attempts payment, and publishes either `payments.completed` or `payments.failed`. Order-service consumes those events and transitions the order: PENDING → CONFIRMED (success) or PENDING → CANCELLED with stock release (failure). No direct call from payment-service to order-service — they communicate only through events. This means they can be deployed, scaled, and restarted independently. The Kafka consumer group ensures each event is processed by exactly one instance of order-service.

### Q: What happens if the Kafka consumer crashes between consuming an event and committing the offset?

**A:** Kafka's at-least-once delivery means the event will be redelivered. The consumer will process `payments.completed` again for an order already in CONFIRMED state. The state machine `validateTransition(CONFIRMED, CONFIRMED)` returns an error — but we handle this idempotently: we catch `InvalidOrderStateException` for the target state already being the current state and treat it as a no-op (log and commit the offset). Without this idempotency guard, a redelivered event would surface as a 422 error, the consumer would retry, and the message might end up in the dead letter queue incorrectly.

### Q: Why snapshot product name and price in `order_items` instead of storing just the product ID?

**A:** Product prices change, products get deleted, and sellers can update names. If order items stored only `product_id`, a historical order viewed months later would show the current price and name — or worse, a 404 for a deleted product. Snapshotting the price and name at order creation time creates an immutable record of exactly what the customer paid for, independent of future catalog changes. This is the standard approach in commerce systems: the order is a legal record, not a view of the current catalog.
