# order-service: How It Works

---

## 1. What Is It?

The `order-service` is a Java/Spring Boot microservice that manages the **order lifecycle, in-app notifications, and seller fulfilment views** for the ecommerce platform — from creation through delivery to cancellation.

**Analogy:** Think of it as a bank teller processing a withdrawal. The teller (order-service) receives your request, locks your account record (pessimistic lock), validates the state of your account (state machine), updates the balance (reserves stock), and records every step in a ledger (status history). If two tellers try to process the same account simultaneously, only one gets the lock — the second waits or fails. There's no "maybe both succeed" — correctness is non-negotiable. Beyond the transaction itself, the teller also sends messages: a notification to the warehouse when a new order arrives, a receipt to the customer when payment clears, and an alert to the shop owner when a customer leaves a product review.

**Responsibilities:**
- Create orders with parallel stock reservation across multiple products (synchronized tracking, compensation on any failure)
- Validate all items are from the **same seller** at create time — mixed-seller orders are rejected with full stock compensation
- Enforce a strict state machine: `PENDING → CONFIRMED/CANCELLED → SHIPPED → DELIVERED`
- Pessimistic row-level locking on all state transitions — concurrent transitions are serialized, never duplicated
- **Transactional outbox**: `OutboxEvent` written atomically with the order; `OutboxPublisher` polls every 100ms (`FOR UPDATE SKIP LOCKED`) and publishes to Kafka — decouples order creation from Kafka availability
- Consume `payments.completed` / `payments.failed` Kafka events to advance (`CONFIRMED`) or cancel the order
- `releaseStockForOrder` runs `@Async` on payment failure — Kafka consumer thread returns immediately
- Append-only `order_status_history` for a complete audit trail of every transition
- In-app notifications: seller on new order + cancellation + payment confirmed; buyer on shipment; seller on review (via internal endpoint)
- `purchase-verification` endpoint — product-service calls this synchronously before accepting a review to confirm the user actually bought the product at DELIVERED status
- Seller order view: paginated order list scoped to a seller ID, filterable by status
- Internal review notification endpoint — blocked at nginx externally; only reachable from product-service within Docker network

---

## 2. Why It Matters

### In this project
- The order service is where the distributed saga begins. It creates the order, reserves stock via synchronous REST calls to product-service, then fires a Kafka event to start the async payment flow. If payment fails, it receives the failure event and releases the reserved stock — the entire compensation chain is driven from here.
- Pessimistic locking on state transitions is the critical safety property. Without `SELECT FOR UPDATE`, two concurrent requests (e.g., `CONFIRMED → SHIPPED` and `CONFIRMED → CANCELLED`) can both read state=CONFIRMED, both pass the transition check, and both commit — leaving the order in an inconsistent terminal state. With the lock, exactly one wins.
- The `order_status_history` table is non-negotiable for customer support. Every state change is recorded with a timestamp and reason.
- The notification system gives both buyers and sellers real-time visibility into order events and customer feedback without requiring them to poll. The `productId` vs `orderId` split on the `Notification` entity lets the frontend route clicks correctly: order notifications navigate to the order detail page, review notifications navigate to the product page.
- The VARCHAR status migration (V6) fixed a PostgreSQL enum incompatibility with Hibernate 6's JDBC parameter binding. Understanding this trade-off (enum type safety vs. driver compatibility) is a production-relevant lesson.

### In real-world systems
- Pessimistic locking for order state transitions is the standard approach in financial and order management systems.
- The choreography saga pattern (Kafka events driving state changes) is used by Uber, Netflix, and Shopify for distributed transactions. Choreography decouples services — payment-service doesn't know about order-service's internal state.
- In-app notifications for marketplace sellers (Shopify, Etsy) are essential for seller engagement. The pattern of storing `orderId` vs `productId` to differentiate notification types is exactly how most notification systems distinguish navigation targets.

---

## 3. How It Works — Step-by-Step Flows

### Create Order (the critical path)
```
POST /api/v1/orders  (X-User-Id: <UUID> header — set by Nginx after JWT validation)
    body:{cartId, sellerId, totalAmount, items[], shippingAddress}
    │
    ├─ 1. Parallel stock reservation (CompletableFuture.supplyAsync per item):
    │        reserveStock(productId, quantity, "order-{userId}")
    │          └─ POST product-service:8081/api/v1/inventory/{id}/reserve
    │        Tracks successfully reserved IDs in synchronized(reservedProductIds)
    │        CompletableFuture.allOf(...).join()  ← waits for ALL
    │        Any failure → compensate:
    │          For each reservedProductId: releaseStock(...)
    │          Re-throw original exception
    │
    ├─ 2. Fetch live product details + validate same seller:
    │        For each item: productServiceClient.getProduct(productId)
    │          → snapshot productName, unitPrice from product-service
    │          → validate productSellerId matches across all items
    │             Mismatch → release ALL stock → throw IllegalArgumentException
    │        Compute totalAmount = Σ (product.price × quantity)
    │
    ├─ 3. Persist Order (status=PENDING) + OrderItems + initial StatusHistory
    │        oldStatus=null, newStatus=PENDING, reason="Order created"
    │
    ├─ 4. Write OutboxEvent to orders_outbox (SAME TRANSACTION — atomic with order save)
    │        payload = OrderCreatedEvent{orderId, userId, totalAmount, items[]}
    │        headers = {"X-Correlation-ID": correlationId}
    │        ← OutboxPublisher polls every 100ms with FOR UPDATE SKIP LOCKED
    │          and publishes to topic: orders.created (partition key = orderId)
    │
    ├─ 5. notificationService.notifySeller(sellerId, orderId, "New order #...", "$total")
    │        (wrapped in try-catch — failure is logged WARN, order still succeeds)
    │
    └─ Return OrderResponse (201)
```

### State Transition — cancelOrder
```
PUT /api/v1/orders/{id}/cancel  (X-User-Id header)
    │
    ├─ findById → ownership check (order.userId == userId → else 403)
    ├─ updateOrderStatus(orderId, CANCELLED, "User requested cancellation", userId)
    │     └─ findByIdWithLock(orderId)
    │          └─ SELECT * FROM orders WHERE id=? FOR UPDATE
    │             ← row is locked; concurrent requests queue here
    │        stateMachine.validateTransition(current, CANCELLED)
    │          PENDING → CANCELLED ✓ | SHIPPED → CANCELLED ✗ → InvalidOrderStateException → 409
    │        order.status = CANCELLED
    │        orderRepository.save(order)
    │        historyRepository.save(old→new, reason, changedBy)
    │
    ├─ For each item: productServiceClient.releaseStock(productId, qty, orderId)
    │     (each wrapped in try-catch — release failures are logged, not thrown)
    │
    ├─ notificationService.notifySeller(sellerId, orderId, "Order cancelled by customer", ...)
    │     (wrapped in try-catch — logged WARN on failure)
    │
    └─ Return OrderResponse
```

### State Transition — updateOrderStatus (shared by all transitions)
```
updateOrderStatus(orderId, newStatus, reason, changedBy)
    │
    ├─ orderRepository.findByIdWithLock(orderId) → FOR UPDATE
    ├─ stateMachine.validateTransition(oldStatus, newStatus)
    ├─ order.setStatus(newStatus)
    ├─ orderRepository.save(order)
    └─ historyRepository.save(orderId, oldStatus, newStatus, reason, changedBy)
```

### Kafka: Payment Event Consumer
```
Topic: payments.completed  (consumer group: order-service)
    │
    └─ PaymentEventConsumer.onPaymentCompleted(record)
          ├─ MDC.put("correlationId", extractCorrelationId(record))  ← from Kafka header
          ├─ orderService.updateOrderStatus(orderId, CONFIRMED, "Payment completed", "payment-service")
          │     └─ findByIdWithLock → stateMachine.validate → save + record history
          └─ notificationService.notifySeller(order.sellerId, orderId,
                  "Payment confirmed — ready to ship", "Order #... has been paid")
             (notifies SELLER, not buyer — seller needs to act by shipping)

Topic: payments.failed  (consumer group: order-service)
    │
    └─ PaymentEventConsumer.onPaymentFailed(record)
          ├─ orderService.updateOrderStatus(orderId, CANCELLED, "Payment failed: {reason}", "payment-service")
          └─ orderService.releaseStockForOrder(orderId)  ← @Async("taskExecutor")
                └─ For each item: releaseStock(productId, qty, orderId.toString())
                   (each wrapped in try-catch; Kafka consumer thread returns immediately)
             (NO buyer notification — payment-service sends its own failure notification)
```

### Purchase Verification (called by product-service before allowing a review)
```
GET /api/v1/orders/purchase-verification?productId=&orderItemId=
    header: X-User-Id: {customerId}
    │
    └─ orderItemRepository.findVerifiedDeliveredItem(orderItemId, userId, productId)
          └─ SELECT oi FROM OrderItem oi JOIN oi.order o
               WHERE oi.id = :itemId AND o.userId = :userId
                 AND o.status = 'DELIVERED' AND oi.productId = :productId
             Present → { verified: true }  |  Empty → { verified: false }
    ← product-service maps false → PurchaseNotVerifiedException → 403
```

### Ship Order (seller action — internal only)
```
PUT /api/v1/orders/{id}/ship  (X-User-Id: sellerId — nginx blocks external)
    │
    ├─ findByIdWithLock → seller ownership check (order.sellerId == sellerId)
    ├─ updateOrderStatus(orderId, SHIPPED, "Shipped by seller", sellerId)
    └─ notificationService.notifyBuyer(order.userId, orderId,
              "Your order has been shipped", "Order #... is on its way.")
```

### Internal Review Notification (from product-service)
```
POST /api/v1/orders/notifications/internal/review
    body:{sellerId, productId, title, body}
    ← This endpoint is blocked by nginx (return 403) for external requests.
      Only reachable within the Docker internal network from product-service.
    │
    ├─ notificationService.notifySellerReview(sellerId, productId, title, body)
    │     └─ Persist Notification{
    │              userId=sellerId,
    │              orderId=null,        ← key difference: no order associated
    │              productId=productId, ← set so frontend navigates to /products/:id
    │              title=title,
    │              body=body,
    │              isRead=false
    │            }
    └─ Return 200 ApiResponse<Void>
```

### Notification Routing (why orderId vs productId matters)
```
Buyer clicks notification (order event):
    Notification.orderId = "abc-123", productId = null
    → frontend: navigate("/orders/abc-123")

Seller clicks notification (review event):
    Notification.orderId = null, productId = 42
    → frontend: navigate("/products/42")

Seller clicks notification (new order):
    Notification.orderId = "def-456", productId = null
    → frontend: navigate("/orders/def-456")
```

### Seller Order List
```
GET /api/v1/orders/seller[?status=CONFIRMED]&page=0&size=20  (X-User-Id: sellerId)
    │
    ├─ status set → findBySellerIdAndStatusOrderByCreatedAtDesc(sellerId, status, pageable)
    ├─ status null → findBySellerIdOrderByCreatedAtDesc(sellerId, pageable)
    │     └─ VARCHAR(50) status; Hibernate binds as String → no enum cast error
    │
    └─ Return Page<OrderSummaryResponse>

GET /api/v1/orders/seller/{id}  ← get single order detail as seller (ownership check)
```

### Notifications API
```
GET /api/v1/orders/notifications  (X-User-Id header)
    └─ Returns NotificationSummaryResponse{
             unreadCount: long,          ← count of unread
             items: top 20 notifications ordered newest first
           }

PUT /api/v1/orders/notifications/mark-read  (X-User-Id header)
    └─ Marks ALL notifications read for the user (bulk update)
```

---

## 4. System Design — Components & Architecture

```
                ┌────────────────────────────────────────────────────────────────────┐
                │                          order-service                               │
                │                                                                     │
HTTP ───────────┤  CorrelationFilter (X-Correlation-ID → MDC)                        │
(X-User-Id      │  OrderController (orders + seller view + notifications + internal)  │
 set by Nginx)  │      │                                                              │
                │  OrderServiceImpl ◄──── OrderStateMachine                         │
                │      │                       (VALID_TRANSITIONS map)              │
                │      ├── OrderRepository (findByIdWithLock — FOR UPDATE)           │
                │      ├── OutboxEventRepository  ← written atomically with order    │
                │      ├── OrderItemRepository (findVerifiedDeliveredItem)            │
                │      ├── OrderStatusHistoryRepository                              │
                │      ├── NotificationService (seller/buyer/review)                 │
                │      └── ProductServiceClient (reserve/release/getProduct)         │
                │                                                                     │
                │  OutboxPublisher (@Scheduled 100ms) ──────────────────────────►   │
                │      FOR UPDATE SKIP LOCKED → publish → mark published_at   Kafka  │
                │  OutboxPublisher.reapStuckPendingOrders (@Scheduled 60s)           │
                │  PaymentEventConsumer ◄────────────────────────────────────────    │
                └──────────────────────────────────────────────────────────────────  ┘
                         │                              │
          ┌──────────────┴───────────┐     ┌───────────┴───────────────┐
          │        PostgreSQL         │     │  product-service:8081      │
          │                           │     │                            │
          │ orders (status VARCHAR)   │     │ POST /inventory/reserve    │
          │ order_items               │     │ POST /inventory/release    │
          │ order_status_history      │     │ GET  /products/{id}        │
          │ notifications             │     └────────────────────────────┘
          │   (orderId|productId)     │
          │ orders_outbox             │
          │   (published_at NULL=new) │
          └───────────────────────────┘
```

### Kafka topics

| Topic | Direction | Partitions | Key |
|---|---|---|---|
| `orders.created` | order-service → payment-service | 3 | userId |
| `payments.completed` | payment-service → order-service | 3 | orderId |
| `payments.failed` | payment-service → order-service | 3 | orderId |

### Order state machine

```
  [CREATE] ──► PENDING ──► CONFIRMED ──► SHIPPED ──► DELIVERED
                  │             │
                  ▼             ▼
              CANCELLED     CANCELLED
```

Valid transitions (defined in `OrderStateMachine.VALID_TRANSITIONS` map):
- `PENDING → CONFIRMED` (payment succeeded via Kafka) or `PENDING → CANCELLED` (payment failed / user cancels)
- `CONFIRMED → SHIPPED` (seller ships — internal only) or `CONFIRMED → CANCELLED` (exceptional)
- `SHIPPED → DELIVERED` (delivery confirmed — internal only)
- `DELIVERED` and `CANCELLED` are terminal — no transitions out; `validateTransition` throws `InvalidOrderStateException → 409`

Note: there is **no** `PAYMENT_FAILED` status. Payment failures transition to `CANCELLED`.

### Data model

```
orders
  id               UUID PK (gen_random_uuid)
  user_id          UUID NOT NULL
  cart_id          UUID
  seller_id        UUID NOT NULL                ← V2 migration: for seller scoped queries
  total_amount     NUMERIC(12,2)
  status           VARCHAR(50) NOT NULL         ← V6: converted from PostgreSQL enum to varchar
  shipping_address JSONB                        ← ShippingAddress value object
  created_at       TIMESTAMPTZ
  updated_at       TIMESTAMPTZ

order_items
  id           UUID PK
  order_id     UUID FK CASCADE
  product_id   BIGINT
  product_name VARCHAR                          ← snapshotted at order time
  quantity     INT
  unit_price   NUMERIC(12,2)                   ← snapshotted at order time
  image_url    VARCHAR (nullable)               ← shown in order detail UI

order_status_history (append-only)
  id           BIGSERIAL PK
  order_id     UUID FK
  old_status   VARCHAR(50)                      ← V6: was order_status enum
  new_status   VARCHAR(50) NOT NULL
  reason       VARCHAR
  changed_by   VARCHAR                          ← "user", "payment-service", "seller"
  changed_at   TIMESTAMPTZ

notifications
  id           UUID PK
  user_id      UUID NOT NULL                    ← recipient
  order_id     UUID (nullable)                  ← set for order events; null for review events
  product_id   BIGINT (nullable)                ← V4: set for review events; null for order events
  type         VARCHAR
  title        VARCHAR NOT NULL
  body         TEXT
  is_read      BOOLEAN DEFAULT false
  status       VARCHAR
  created_at   TIMESTAMPTZ
```

### Why VARCHAR(50) instead of PostgreSQL enum for status

PostgreSQL custom enum types (e.g., `order_status`) require the JDBC parameter to be cast explicitly: `UPDATE orders SET status = ?::order_status`. Hibernate 6 sends `@Enumerated(EnumType.STRING)` values as standard `character varying` JDBC parameters. PostgreSQL has no built-in `=` operator for `(order_status, character varying)`, so filtered queries fail with `operator does not exist: order_status = character varying`.

The fix (V6 migration) converts status columns to `VARCHAR(50)` using `ALTER COLUMN ... TYPE VARCHAR(50) USING status::text`. Hibernate then binds as string → Postgres matches string → queries work correctly. The `@Enumerated(EnumType.STRING)` annotation is kept; only the DB column type changes.

### Flyway migrations

| Version | What |
|---|---|
| V1 | `baseline_schema.sql` — orders, order_items, order_status_history with `order_status` enum |
| V2 | `add_seller_context.sql` — `seller_id` on orders; indexes for seller queries |
| V3 | `add_notification_in_app.sql` — `notifications` table |
| V4 | `add_product_id_to_notifications.sql` — `product_id BIGINT` nullable on notifications |
| V5 | `add_order_status_varchar_cast.sql` — implicit cast attempt (kept for migration continuity) |
| V6 | `convert_order_status_to_varchar.sql` — converts status columns from enum to VARCHAR(50) |

---

## 5. Code Examples

### Pessimistic lock on state transition

```java
// OrderRepository.java
@Lock(LockModeType.PESSIMISTIC_WRITE)
@Query("SELECT o FROM Order o WHERE o.id = :id")
Optional<Order> findByIdWithLock(@Param("id") UUID id);
// Generates: SELECT * FROM orders WHERE id=? FOR UPDATE
```

```java
// OrderServiceImpl.java
@Transactional
public OrderResponse updateOrderStatus(UUID orderId, OrderStatus newStatus, String reason, String changedBy) {
    Order order = orderRepository.findByIdWithLock(orderId)
        .orElseThrow(() -> new OrderNotFoundException(orderId));

    stateMachine.validateTransition(order.getStatus(), newStatus);

    OrderStatus oldStatus = order.getStatus();
    order.setStatus(newStatus);
    orderRepository.save(order);

    orderStatusHistoryRepository.save(
        OrderStatusHistory.of(orderId, oldStatus, newStatus, reason, changedBy));
    return toResponse(order);
}
```

### Notification: orderId vs productId split

```java
// NotificationService.java
public void notifySeller(UUID sellerId, UUID orderId, String title, String body) {
    persist(sellerId, orderId, null, title, body);           // orderId set, productId null
}

public void notifyBuyer(UUID buyerId, UUID orderId, String title, String body) {
    persist(buyerId, orderId, null, title, body);
}

public void notifySellerReview(UUID sellerId, Long productId, String title, String body) {
    persist(sellerId, null, productId, title, body);         // productId set, orderId null
}

private void persist(UUID userId, UUID orderId, Long productId, String title, String body) {
    repo.save(Notification.builder()
        .userId(userId).orderId(orderId).productId(productId)
        .type(NotificationType.IN_APP).title(title).body(body)
        .isRead(false).status("SENT").createdAt(OffsetDateTime.now()).build());
}
```

### Purchase verification — gating review creation

```java
// OrderItemRepository.java
@Query("""
    SELECT oi FROM OrderItem oi JOIN oi.order o
    WHERE oi.id = :itemId AND o.userId = :userId
      AND o.status = 'DELIVERED' AND oi.productId = :productId
    """)
Optional<OrderItem> findVerifiedDeliveredItem(UUID itemId, UUID userId, Long productId);
// product-service calls GET /purchase-verification?productId=&orderItemId=
// with X-User-Id header. Returns { verified: true/false }.
// Three conditions must ALL hold: correct item ID, correct user, order DELIVERED.
```

### Internal review notification endpoint (nginx-blocked externally)

```java
// OrderController.java
@PostMapping("/notifications/internal/review")
// nginx: location = /api/v1/orders/notifications/internal/review { return 403; }
// Only reachable from within the Docker network (product-service fire-and-forget call)
public ApiResponse<Void> createReviewNotification(@RequestBody ReviewNotificationRequest req) {
    notificationService.notifySellerReview(req.sellerId(), req.productId(), req.title(), req.body());
    return ApiResponse.ok((Void) null);
}
```

### Parallel stock reservation with compensation

```java
// OrderServiceImpl.java
List<Long> reservedProductIds = new ArrayList<>();

List<CompletableFuture<ProductServiceClient.StockResponse>> futures = items.stream()
    .map(item -> CompletableFuture.supplyAsync(() -> {
        ProductServiceClient.StockResponse resp = productServiceClient.reserveStock(
                item.getProductId(), item.getQuantity(), "order-" + userId);
        synchronized (reservedProductIds) {
            reservedProductIds.add(item.getProductId()); // track which succeeded
        }
        return resp;
    }))
    .collect(Collectors.toList());

try {
    CompletableFuture.allOf(futures.toArray(new CompletableFuture[0])).join();
} catch (Exception e) {
    // Compensate: release only the items that were successfully reserved
    for (Long productId : reservedProductIds) {
        items.stream().filter(i -> i.getProductId().equals(productId)).findFirst()
             .ifPresent(i -> productServiceClient.releaseStock(
                     productId, i.getQuantity(), "order-" + userId));
    }
    throw ...; // re-throw original exception
}
```

### Transactional outbox — write + publish separation

```java
// OrderServiceImpl.java — createOrder() step 4: write outbox IN SAME TRANSACTION as order
outboxEventRepository.save(OutboxEvent.builder()
        .orderId(saved.getId())
        .payload(objectMapper.writeValueAsString(event))
        .headers(objectMapper.writeValueAsString(Map.of("X-Correlation-ID", correlationId)))
        .createdAt(OffsetDateTime.now())
        .build());
// If this TX rolls back, the outbox row is also rolled back — no phantom Kafka event.
// OutboxPublisher polls every 100ms and publishes independently.

// OutboxPublisher.java — runs every 100ms, separate TX
@Scheduled(fixedDelay = 100)
@Transactional
public void publishPending() {
    List<OutboxEvent> batch = outboxRepo.findUnpublishedForUpdate();
    // findUnpublishedForUpdate: SELECT ... WHERE published_at IS NULL FOR UPDATE SKIP LOCKED
    // SKIP LOCKED ensures multiple instances don't publish the same row
    for (OutboxEvent ev : batch) {
        ProducerRecord<String, Object> record = new ProducerRecord<>(
                "orders.created", ev.getOrderId().toString(), event);
        kafkaTemplate.send(record).get(5, TimeUnit.SECONDS); // synchronous send
        ev.setPublishedAt(OffsetDateTime.now());
        outboxRepo.save(ev);
    }
}

// Reaper (every 60s): re-queues PENDING orders > 2min old with no unpublished outbox row
// (handles the case where the OutboxPublisher published but crashed before marking published_at)
```

---

## 6. Trade-offs

### Pessimistic vs. optimistic locking for order state transitions

| | Pessimistic (`FOR UPDATE`) | Optimistic (`@Version`) |
|---|---|---|
| Concurrent transitions | Serialized — second waiter blocks until first commits | Race — both may read old state; one commit wins, other retries |
| Correctness guarantee | Always — only one writer in the critical section | With retries — but retry after CONFIRMED→SHIPPED could still attempt same transition |
| Read performance | Blocked during write | Reads never blocked |
| **Our choice** | ✅ Order transitions are rare but catastrophic if duplicated | Fine for inventory (read-heavy, same outcome on retry) |

### HTTP fire-and-forget vs. Kafka for review notifications

| | HTTP fire-and-forget (our choice) | Kafka |
|---|---|---|
| Infrastructure | Zero — uses existing RestTemplate | Requires topic + consumer group + offset management |
| Reliability | Best-effort — WARN on failure, no retry | At-least-once guaranteed |
| Latency | ~50ms inline, logged on failure | Async, ~100–500ms to consumer |
| Code complexity | One try/catch | Producer + consumer + idempotency |
| **Our choice** | ✅ Single internal call; failure is tolerable | Necessary if notification delivery is contractually required |

Review notifications are quality-of-life for sellers. A missed notification doesn't corrupt any data. The HTTP call completes in under 50ms when successful and is immediately abandoned on failure. Adding Kafka for this would double the operational surface for a non-critical path.

### Choreography saga vs. orchestration

| | Choreography (our approach) | Orchestration (saga coordinator) |
|---|---|---|
| Coupling | Services react to events independently | Central orchestrator knows the full flow |
| Failure visibility | Hard — must trace events across topics | Easy — orchestrator tracks saga state |
| New service addition | Add consumer + events | Update orchestrator |
| **Our choice** | ✅ Simple 3-service saga | Better for 5+ step sagas with complex rollback |

### VARCHAR vs. PostgreSQL enum for status columns

| | PostgreSQL enum | VARCHAR(50) |
|---|---|---|
| Type safety | Enforced at DB level — invalid values rejected | Enforced only at application level |
| Hibernate 6 compatibility | Broken — `operator does not exist` on filtered queries | Works — Hibernate sends as String, Postgres matches String |
| Alter flexibility | `ALTER TYPE ... ADD VALUE` requires table lock | Simple `ALTER COLUMN` with explicit type list |
| **Our choice** | ✅ Initial design | ✅ V6 migration — compatibility over DB-level enforcement |

The `@Enumerated(EnumType.STRING)` annotation on the Java entity still enforces the value set at the application layer. The DB-level enum enforcement was lost but the application constraint is sufficient.

---

## 7. When to Use / Avoid

### Use this pattern when:
- **State machine correctness is critical**: orders, payments, shipments — any domain where "double-processing" causes real harm needs pessimistic locking.
- **Small-to-medium saga** (2–4 services): choreography via Kafka is clean and decoupled.
- **Parallel independent operations**: stock reservations for multiple items are independent — `CompletableFuture.runAsync` makes them genuinely parallel.
- **Non-critical side effects via fire-and-forget**: notifications, analytics, audit events that must not block the primary transaction.

### Avoid when:
- **High concurrent transitions on the same order**: the `FOR UPDATE` lock serializes all concurrent requests. If thousands of systems race to update a single order, use an event queue.
- **Long-running saga with many services**: choreography becomes hard to debug at 5+ services. Use Temporal or AWS Step Functions.
- **Notification delivery is contractually guaranteed**: fire-and-forget HTTP loses messages on failure; use Kafka with a consumer group.

---

## 8. Interview Insights

### Q: Why use `SELECT FOR UPDATE` for order state transitions instead of optimistic locking?

**A:** Order state transitions are low-frequency but high-stakes. Consider the race: two threads both read status=CONFIRMED, both validate that CONFIRMED → SHIPPED is allowed, and both commit. Now you have two SHIPPED history entries. With `SELECT FOR UPDATE`, the second thread waits until the first commits. It then reads status=SHIPPED and the state machine rejects SHIPPED → SHIPPED. Pessimistic locking is the conservative, obviously-correct choice for critical state machines.

### Q: Explain the parallel stock reservation with compensation. What does "compensation" mean?

**A:** Compensation is the distributed system equivalent of a rollback. When we create an order with 3 items, we fire 3 parallel HTTP calls to product-service to reserve stock. If the third call fails, the first two have already decremented `stock_reserved`. Without compensation, those two items stay "reserved" forever. The compensation logic releases them: for each successfully reserved item, we call `releaseStock`. This is the "undo" of a saga step — not a DB rollback, but a compensating transaction.

### Q: How does the choreography saga work end-to-end?

**A:** Order-service creates the order in PENDING state and publishes `orders.created` to Kafka. Payment-service consumes that event, attempts payment, and publishes `payments.completed` or `payments.failed`. Order-service consumes those events and transitions the order. No direct call from payment-service to order-service — they communicate only through events. The Kafka consumer group ensures each event is processed by exactly one instance of order-service.

### Q: Why does the `Notification` entity have both `orderId` and `productId`, and why are they mutually exclusive?

**A:** The two fields serve different navigation targets in the frontend. Order-related notifications (new order, payment confirmed) link to an order detail page — so `orderId` is set and `productId` is null. Review notifications link to the product page so the seller can see their feedback — so `productId` is set and `orderId` is null. A single `targetId` + `targetType` field would also work, but separate nullable columns are simpler and self-documenting. The frontend checks which field is non-null to decide where to navigate on click.

### Q: You mentioned the `order_status` PostgreSQL enum caused a bug with Hibernate 6. What happened exactly?

**A:** PostgreSQL custom enum types require explicit casts in SQL: `... WHERE status = ?::order_status`. Hibernate 6 changed how it binds `@Enumerated(EnumType.STRING)` values: it now sends them as typed JDBC parameters, not raw `character varying`. PostgreSQL's `=` operator has overloads for `(order_status, order_status)` and `(character varying, character varying)`, but not `(order_status, character varying)`. So any filtered query — `WHERE status = ?` — threw `operator does not exist`. The fix was to convert the column to `VARCHAR(50)` via a Flyway migration, keeping `@Enumerated(EnumType.STRING)` on the Java entity for application-layer enforcement.

### Q: How does the internal review notification endpoint stay secure?

**A:** nginx has an exact-match location block: `location = /api/v1/orders/notifications/internal/review { return 403; }`. All external traffic hitting this path gets a 403 before it reaches order-service. Requests from within the Docker network (product-service calling order-service by service name) bypass nginx entirely — they hit order-service directly at port 8082. This pattern (expose internally, block externally at the gateway) is standard in microservice architectures where service-to-service trust is enforced by network topology, not application-level auth.

### Q: Why use a transactional outbox instead of publishing directly to Kafka from createOrder?

**A:** Direct Kafka publish in the same transaction doesn't work — Kafka isn't part of the same 2PC boundary as Postgres. If `createOrder()` committed the Postgres row but then crashed before publishing to Kafka, the order would exist in PENDING forever with no payment event ever triggered. The transactional outbox solves this: the `OutboxEvent` row is written in the same `@Transactional` as the order insert, so they commit together. A background poller then publishes from the DB to Kafka with retries. The only failure modes are (a) the poller crashes after sending but before marking `published_at` — in which case the event is re-sent (at-least-once, Kafka handles this idempotently) or (b) the order is stuck PENDING, which the reaper job detects after 2 minutes and re-queues.

### Q: What happens if the Kafka consumer crashes between consuming an event and committing the offset?

**A:** Kafka's at-least-once delivery means the event will be redelivered. The consumer will process `payments.completed` again for an order already in CONFIRMED state. `stateMachine.validateTransition(CONFIRMED, CONFIRMED)` throws `InvalidOrderStateException` — but the consumer catches all exceptions and logs them, so the offset is still committed and the message is not re-delivered indefinitely. The seller notification call is idempotent at the notification level (a duplicate notification is annoying but not harmful). Without this exception handling, a redelivered event would cause the consumer to crash-loop and the message would eventually end up in the DLQ.
