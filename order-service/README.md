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
               ┌─────────────┼──────────────┐
               ▼             ▼              ▼
         CONFIRMED      PAYMENT_FAILED   CANCELLED
         (Kafka saga)   (Kafka saga)    (manual cancel)
               │
               ▼
           SHIPPED   (seller action — internal only)
               │
               ▼
          DELIVERED  (seller action — internal only)
```

State transitions use `SELECT ... FOR UPDATE` to prevent concurrent updates from putting an order in two states simultaneously. Invalid transitions (e.g., DELIVERED → PENDING) return 409.

---

## API Reference

### Orders — `/api/v1/orders`

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/` | Bearer JWT | Create order from cart |
| `GET` | `/` | Bearer JWT | List orders (buyer: own orders; seller: pass `?sellerId=`) |
| `GET` | `/:id` | Bearer JWT | Get order detail (buyer or owning seller) |
| `PUT` | `/:id/cancel` | Bearer JWT | Cancel order (PENDING or CONFIRMED only) |
| `PUT` | `/:id/ship` | Internal only (nginx 403) | Mark as SHIPPED |
| `PUT` | `/:id/deliver` | Internal only (nginx 403) | Mark as DELIVERED |
| `GET` | `/:id/history` | Bearer JWT | Get status change history |

**List query params:** `?sellerId=<uuid>` (seller view), `?status=<STATUS>`, standard `page`/`size`.

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

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/` | Bearer JWT | List notifications for the current user (newest first) |
| `PUT` | `/:id/read` | Bearer JWT | Mark a notification as read |
| `POST` | `/internal/review` | **Blocked externally** | Called by product-service after a review is created |

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

**Produces** (on order creation):
- Topic: `orders.created`
- Payload: `{ orderId, userId, totalAmount, items[], idempotencyKey }`

**Consumes**:
- `payments.completed` → transitions order to `CONFIRMED`, notifies buyer
- `payments.failed` → transitions order to `PAYMENT_FAILED`, releases stock reservation, notifies buyer

---

## Notifications

Three types of notifications are created:

| Event | Recipient | `orderId` | `productId` |
|---|---|---|---|
| Order placed | Seller | ✓ | null |
| Payment confirmed | Buyer | ✓ | null |
| Payment failed | Buyer | ✓ | null |
| New review | Seller | null | ✓ |

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
├── controller/OrderController.java           # REST endpoints + internal notification endpoint
├── service/
│   ├── OrderService.java                     # interface
│   ├── NotificationService.java              # creates/queries notifications
│   └── impl/OrderServiceImpl.java            # state machine, pessimistic locking
├── kafka/
│   ├── OrderEventPublisher.java              # produces orders.created
│   └── PaymentEventConsumer.java             # consumes payments.completed/failed
├── model/
│   ├── Order.java                            # JPA entity (status: VARCHAR via @Enumerated STRING)
│   ├── OrderItem.java
│   ├── OrderStatus.java                      # enum
│   ├── OrderStatusHistory.java
│   ├── Notification.java                     # orderId + productId (one nullable per notification)
│   └── NotificationType.java
├── dto/
│   ├── ReviewNotificationRequest.java        # {sellerId, productId, title, body}
│   └── NotificationResponse.java            # {id, orderId, productId, title, body, isRead, createdAt}
└── repository/
    ├── OrderRepository.java                  # findByIdWithLock, seller + status queries
    └── NotificationRepository.java
```
