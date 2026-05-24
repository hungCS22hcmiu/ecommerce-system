# order-service

Java 21 / Spring Boot 3.5 microservice for order management. Handles the full order lifecycle from creation through delivery, manages in-app notifications for buyers and sellers, and publishes Kafka events to trigger the payment saga.

- **Port:** 8082
- **Database:** PostgreSQL (`ecommerce_orders`)
- **Concurrency:** Pessimistic locking (`SELECT ... FOR UPDATE`) on all state transitions

## Quick Start

```bash
# From repo root — start infrastructure (postgres + kafka required)
docker compose up -d postgres zookeeper kafka

# Run locally
cd order-service
./mvnw spring-boot:run

# Or via Docker
docker compose build order-service
docker compose up -d order-service
```

Health check: `GET http://localhost:8082/health/live`

---

## Order State Machine

```
                        ┌──────────┐
                        │ PENDING  │  Created, awaiting payment
                        └────┬─────┘
               ┌─────────────┴──────────────┐
               ▼                            ▼
         CONFIRMED                      CANCELLED
    (payments.completed)     (payments.failed OR user cancel)
               │
               ▼
           SHIPPED   (seller action — nginx blocks external)
               │
               ▼
          DELIVERED  (seller action — nginx blocks external)
```

`CANCELLED` and `DELIVERED` are terminal — no transitions out. There is **no** `PAYMENT_FAILED` status; payment failures transition to `CANCELLED`. All transitions use `SELECT ... FOR UPDATE` (pessimistic lock) to prevent concurrent updates. Invalid transitions return 409.

---

## API Reference

### Orders — `/api/v1/orders`

Auth for all routes is `X-User-Id` header (UUID), injected by Nginx after validating the Bearer JWT.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/` | Create order — reserves stock, writes outbox, notifies seller |
| `GET` | `/` | List buyer's own orders |
| `GET` | `/:id` | Get order detail (buyer ownership check) |
| `PUT` | `/:id/cancel` | Cancel order (PENDING or CONFIRMED); releases stock; notifies seller |
| `PUT` | `/:id/ship` | Mark SHIPPED (seller, nginx blocks external); notifies buyer |
| `PUT` | `/:id/deliver` | Mark DELIVERED (nginx blocks external) |
| `GET` | `/:id/history` | Get status change history |
| `GET` | `/seller` | List orders as seller (filterable by `?status=`) |
| `GET` | `/seller/:id` | Get single order detail as seller (ownership check) |
| `GET` | `/purchase-verification` | Verify buyer purchased a product at DELIVERED status (called by product-service) |

**List query params:** `?status=<STATUS>`, standard `page`/`size`.

**Create order body:**
```json
{
  "cartId": "uuid",
  "sellerId": "uuid",
  "totalAmount": 99.99,
  "shippingAddress": {
    "street": "123 Main St",
    "city": "Springfield",
    "state": "IL",
    "zipCode": "62701",
    "country": "US"
  },
  "items": [
    { "productId": 1, "productName": "Widget", "quantity": 2, "unitPrice": 49.99 }
  ]
}
```

### Notifications — `/api/v1/orders/notifications`

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Returns `{unreadCount, items[top 20 newest]}` for the current user |
| `PUT` | `/mark-read` | Mark **all** notifications read for the current user (bulk) |
| `POST` | `/internal/review` | **Blocked externally (nginx 403)** — called by product-service after a review is saved |

**Notification shape:**
```json
{
  "id": "uuid",
  "orderId": "uuid or null",
  "productId": 123456,
  "title": "New review on Widget Pro",
  "body": "5/5 stars — great product!",
  "isRead": false,
  "createdAt": "2026-05-18T10:00:00Z"
}
```

`orderId` is set for order-related notifications (buyer/seller); `productId` is set for review notifications (seller). The frontend uses whichever field is non-null to determine where to navigate on click.

---

## Kafka Integration

order-service is both a **producer** and a **consumer**.

**Produces** (via transactional outbox):
- Topic: `orders.created`
- Payload: `{ orderId, userId, totalAmount, items[], idempotencyKey }`
- `createOrder()` writes an `OutboxEvent` row **atomically** with the order in the same transaction. `OutboxPublisher` polls every 100ms with `FOR UPDATE SKIP LOCKED`, publishes to Kafka, then marks `published_at`. A reaper job (`@Scheduled(fixedDelay=60_000)`) re-queues PENDING orders older than 2 minutes that have no unpublished outbox row.

**Consumes**:
- `payments.completed` → transitions order to `CONFIRMED`, notifies buyer
- `payments.failed` → transitions order to `PAYMENT_FAILED`, releases stock reservation (async, `@Retryable` 3×), notifies buyer

---

## Notifications

Five notification events are created:

| Event | Recipient | `orderId` | `productId` |
|---|---|---|---|
| Order placed | Seller | ✓ | null |
| Order cancelled by customer | Seller | ✓ | null |
| Payment confirmed (ready to ship) | Seller | ✓ | null |
| Order shipped | Buyer | ✓ | null |
| New review | Seller | null | ✓ |

Note: payment failure triggers **no** notification from order-service — payment-service handles its own buyer-facing communication.

Review notifications arrive via `POST /notifications/internal/review` from product-service (fire-and-forget HTTP call). This endpoint is blocked at nginx (`return 403`) — it is only reachable within the Docker network.

---

## Data Model

```
orders
  ├─ id (UUID), user_id (UUID), cart_id (UUID), seller_id (UUID)
  ├─ total_amount DECIMAL(10,2)
  ├─ status VARCHAR(50)  ← was PostgreSQL enum; converted to varchar in V6 migration
  ├─ shipping_address JSONB
  └─ created_at / updated_at TIMESTAMPTZ

order_items
  ├─ id (UUID), order_id → orders
  ├─ product_id BIGINT, product_name VARCHAR
  ├─ quantity INT, unit_price DECIMAL(10,2)
  └─ image_url VARCHAR (nullable — shown in order detail UI)

order_status_history
  ├─ id BIGSERIAL, order_id UUID
  ├─ old_status / new_status VARCHAR(50)
  ├─ reason, changed_by VARCHAR
  └─ changed_at TIMESTAMPTZ

notifications
  ├─ id (UUID), user_id (UUID)
  ├─ order_id (UUID, nullable), product_id (BIGINT, nullable)
  ├─ type, title, body, is_read BOOLEAN, status
  └─ created_at TIMESTAMPTZ

orders_outbox  (transactional outbox — append-only)
  ├─ id (UUID), aggregate_id (UUID — orderId), event_type VARCHAR
  ├─ payload JSONB, headers JSONB (correlation-id stored here)
  ├─ created_at TIMESTAMPTZ, published_at TIMESTAMPTZ (NULL until sent)
  └─ PARTIAL INDEX on (created_at) WHERE published_at IS NULL
```

**Why VARCHAR(50) for status?** PostgreSQL custom enum types (`order_status`) require explicit casts when Hibernate 6 sends parameters as JDBC typed objects. Converting to VARCHAR eliminates `operator does not exist: order_status = character varying` errors on filtered queries — see V6 migration.

---

## Flyway Migrations

| Version | File | What it does |
|---|---|---|
| V1 | `baseline_schema.sql` | `orders`, `order_items`, `order_status_history` with `order_status` enum |
| V2 | `add_seller_context.sql` | `seller_id` on orders; indexes for seller queries |
| V3 | `add_notification_in_app.sql` | `notifications` table |
| V4 | `add_product_id_to_notifications.sql` | `product_id BIGINT` nullable column on notifications |
| V5 | `add_order_status_varchar_cast.sql` | Implicit cast attempt (kept for migration continuity) |
| V6 | `convert_order_status_to_varchar.sql` | Converts status columns to VARCHAR(50) |
| V7 | `add_orders_outbox.sql` | `orders_outbox` table + partial index on unpublished rows (`published_at IS NULL`) |

---

## Configuration

| Env var | Default | Description |
|---|---|---|
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | DB username |
| `DB_PASSWORD` | `postgres` | DB password |
| `DB_NAME` | `ecommerce_orders` | PostgreSQL database |
| `KAFKA_BROKERS` | `kafka:29092` | Kafka broker address |
| `PRODUCT_SERVICE_URL` | `http://product-service:8081` | For stock reservation/release |
| `JWT_PUBLIC_KEY_PATH` | `./keys/public.pem` | RS256 public key |

---

## Testing

```bash
cd order-service
./mvnw test
```

Integration tests use Testcontainers (real PostgreSQL) — no local setup needed.

## Key Files

```
src/main/java/com/ecommerce/order_service/
├── controller/OrderController.java           # REST endpoints; auth via X-User-Id header (set by Nginx)
├── filter/CorrelationFilter.java             # OncePerRequestFilter: X-Correlation-ID → MDC
├── service/
│   ├── OrderService.java                     # interface (createOrder, cancelOrder, shipOrder, verifyPurchase, ...)
│   ├── OrderStateMachine.java                # VALID_TRANSITIONS map; validateTransition / canTransition
│   ├── NotificationService.java              # notifySeller/notifyBuyer/notifySellerReview; getSummary; markAllRead
│   └── impl/OrderServiceImpl.java            # pessimistic locking, outbox write, same-seller validation, @Async stock release
├── kafka/
│   ├── OrderEventProducer.java               # direct publishOrderCreated (used by tests; NOT called in createOrder)
│   ├── OutboxPublisher.java                  # @Scheduled 100ms: FOR UPDATE SKIP LOCKED → publish → mark published_at
│   │                                         # @Scheduled 60s: reaper re-queues stuck PENDING orders
│   └── PaymentEventConsumer.java             # payments.completed → CONFIRMED + notify seller
│                                             # payments.failed → CANCELLED + @Async releaseStockForOrder
├── model/
│   ├── Order.java                            # @Enumerated(STRING) on status VARCHAR
│   ├── OrderItem.java
│   ├── OrderStatus.java                      # PENDING, CONFIRMED, CANCELLED, SHIPPED, DELIVERED
│   ├── OrderStatusHistory.java
│   ├── OutboxEvent.java                      # orderId, payload JSONB, headers JSONB, published_at
│   ├── Notification.java                     # orderId XOR productId (mutually exclusive)
│   └── NotificationType.java
├── dto/
│   ├── ReviewNotificationRequest.java        # {sellerId, productId, title, body}
│   ├── NotificationResponse.java             # {id, orderId, productId, title, body, isRead, createdAt}
│   ├── NotificationSummaryResponse.java      # {unreadCount, items[]}
│   └── PurchaseVerificationResponse.java     # {verified: boolean}
└── repository/
    ├── OrderRepository.java                  # findByIdWithLock; seller queries; findStuckPendingOrderIds
    ├── OrderItemRepository.java              # findVerifiedDeliveredItem (purchase verification)
    ├── OutboxEventRepository.java            # findUnpublishedForUpdate (FOR UPDATE SKIP LOCKED)
    └── NotificationRepository.java           # countUnread; findTop20; markAllReadForUser
```
