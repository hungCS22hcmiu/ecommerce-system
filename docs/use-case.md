# Use Cases & API Endpoints

## User Service (Go — :8001)

### Authentication

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | Public | Register new user account |
| POST | `/api/v1/auth/login` | Public | Authenticate and return JWT + refresh token |
| POST | `/api/v1/auth/refresh` | Public | Issue new JWT from refresh token |
| POST | `/api/v1/auth/logout` | Bearer | Revoke tokens, add JWT to Redis blacklist |

### User Profile

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/users/profile` | Bearer | Get current user profile + addresses |
| PUT | `/api/v1/users/profile` | Bearer | Update user profile |
| GET | `/api/v1/users/{id}` | Internal | Get user by ID (service-to-service only) |

### Authentication Flow

1. User submits email + password → bcrypt hash comparison (cost 12)
2. Check `is_locked` flag and `failed_login_attempts` (lockout after 5 consecutive failures)
3. On success → reset `failed_login_attempts`, generate:
   - **Access token:** JWT (RS256), 15-minute TTL, claims: `{userId, email, role}`
   - **Refresh token:** Opaque random string, 7-day TTL, stored hashed in `auth_tokens`
4. On failure → increment `failed_login_attempts`, lock account if threshold exceeded
5. Logout → add JWT `jti` to Redis blacklist with TTL matching token remaining lifetime

### Data Model

| Table | Key Columns |
|---|---|
| `users` | id (UUID), email (unique), password_hash, role, is_locked, failed_login_attempts |
| `user_profiles` | id, user_id (FK), first_name, last_name, phone, avatar_url |
| `user_addresses` | id, user_id (FK), street, city, state, zip, country, is_default |
| `auth_tokens` | id, user_id (FK), refresh_token_hash, expires_at, revoked |

---

## Product Service (Java/Spring Boot — :8081)

### Product CRUD

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

### Inventory Management

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/inventory/{productId}` | Internal | Check available stock (quantity - reserved) |
| POST | `/api/v1/inventory/{productId}/reserve` | Internal | Reserve stock for order (atomic, optimistic lock) |
| POST | `/api/v1/inventory/{productId}/release` | Internal | Release reserved stock (on cancellation) |
| POST | `/api/v1/inventory/{productId}/confirm` | Internal | Deduct reserved stock (order confirmed) |
| PUT | `/api/v1/inventory/{productId}` | Admin | Manually adjust stock levels |
| GET | `/api/v1/inventory/{productId}/movements` | Admin | View stock movement audit trail |

### Data Model

| Table | Key Columns |
|---|---|
| `products` | id, name, description, price (DECIMAL(10,2)), category_id, seller_id, status, stock_quantity, stock_reserved, version (optimistic lock) |
| `categories` | id, name, slug, parent_id (self-referencing hierarchy), sort_order |
| `product_images` | id, product_id (FK), url, alt_text, sort_order |
| `stock_movements` | id, product_id (FK), type (IN/OUT/RESERVE/RELEASE), quantity, reference_id, reason |

---

## Cart Service (Go — :8002)

### Cart Operations

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/carts` | Bearer | Create a new cart (or return existing active cart) |
| GET | `/api/v1/carts/me` | Bearer | Get current user's active cart with computed total |
| POST | `/api/v1/carts/me/items` | Bearer | Add item to cart (validates product exists + in stock) |
| PUT | `/api/v1/carts/me/items/{productId}` | Bearer | Update item quantity |
| DELETE | `/api/v1/carts/me/items/{productId}` | Bearer | Remove item from cart |
| DELETE | `/api/v1/carts/me` | Bearer | Clear entire cart |
| POST | `/api/v1/carts/me/checkout` | Bearer | Validate cart and initiate order creation |

### Storage Architecture

- **Primary store:** Redis (`cart:{userId}` key, 30-min TTL)
- **Persistence:** PostgreSQL via background goroutine (debounced every 5 min)
- **On add/update:** validates product price + stock via Product Service (circuit breaker protected)

### Data Model

| Table | Key Columns |
|---|---|
| `carts` | id (UUID), user_id, status (ACTIVE/CHECKED_OUT/ABANDONED), expires_at |
| `cart_items` | id, cart_id (FK), product_id, product_name (denormalized), quantity, unit_price |

---

## Order Service (Java/Spring Boot — :8082)

### Order Management

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/orders` | Bearer | Create order from validated cart |
| GET | `/api/v1/orders/{id}` | Bearer | Get order details (owner or admin) |
| GET | `/api/v1/orders` | Bearer | List current user's orders (paginated) |
| PUT | `/api/v1/orders/{id}/cancel` | Bearer | Cancel order (only if PENDING) |
| PUT | `/api/v1/orders/{id}/ship` | Seller/Admin | Mark order as shipped |
| PUT | `/api/v1/orders/{id}/deliver` | Admin | Mark order as delivered |
| GET | `/api/v1/orders/{id}/history` | Bearer | View order status change history |

### Order State Machine

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
| → PENDING | `POST /orders` | Reserve stock (sync), publish `orders.created` to Kafka |
| PENDING → CONFIRMED | `payments.completed` Kafka event | Confirm stock deduction, send confirmation email |
| PENDING → CANCELLED | `payments.failed` or `PUT /cancel` | Release reserved stock, send cancellation email |
| CONFIRMED → SHIPPED | `PUT /orders/{id}/ship` | Send shipment email with tracking |
| SHIPPED → DELIVERED | `PUT /orders/{id}/deliver` | Send delivery confirmation email |

### Kafka Events

**Published:**

| Topic | Event | Payload |
|---|---|---|
| `orders.created` | ORDER_CREATED | orderId, userId, items[], totalAmount |
| `orders.confirmed` | ORDER_CONFIRMED | orderId, userId |
| `orders.cancelled` | ORDER_CANCELLED | orderId, userId, reason, items[] |

**Consumed:**

| Topic | Action |
|---|---|
| `payments.completed` | PENDING→CONFIRMED, confirm stock, send email |
| `payments.failed` | PENDING→CANCELLED, release stock, send failure email |

### Data Model

| Table | Key Columns |
|---|---|
| `orders` | id, user_id, cart_id, total_amount, status, shipping_address (JSONB) |
| `order_items` | id, order_id (FK), product_id, product_name, quantity, unit_price |
| `order_status_history` | id, order_id (FK), old_status, new_status, reason, changed_by |
| `notifications` | id, order_id (FK), user_id, type (EMAIL/SMS), subject, body, status (SENT/FAILED) |

---

## Payment Service (Go — :8003)

### Payment Operations

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/payments` | Internal | Initiate payment (idempotency key required) |
| GET | `/api/v1/payments/{id}` | Bearer | Get payment status |
| GET | `/api/v1/payments/order/{orderId}` | Bearer | Get payment by order ID |
| POST | `/api/v1/payments/{id}/refund` | Admin | Initiate full refund |

### Payment Flow

1. Kafka delivers `orders.created` event → worker pool goroutine picks it up
2. Check idempotency key (DB `UNIQUE` constraint as safety net)
3. Create payment record (status=PENDING) — claims the idempotency key
4. Call mock payment gateway (5s timeout)
5. Update status → publish `payments.completed` or `payments.failed` to Kafka

### Mock Payment Gateway

- Configurable success rate (default: 90%)
- Random latency (50–200ms) to simulate network call
- Supports forced failure via special card numbers (for testing)

### Dead Letter Queue

- Failed events routed to `payments.dlq` after 3 retry attempts
- Retry backoff: 100ms → 200ms → 400ms (exponential)
- DLQ messages can be manually replayed via admin endpoint

### Data Model

| Table | Key Columns |
|---|---|
| `payments` | id (UUID), order_id, user_id, amount, currency, status, method, idempotency_key (unique), gateway_reference |
| `payment_history` | id, payment_id (FK), old_status, new_status, reason |

---

## Health Endpoints (All Services)

| Method | Endpoint | Description |
|---|---|---|
| GET | `/health/live` | Liveness probe — process is running |
| GET | `/health/ready` | Readiness probe — DB, Redis, Kafka reachable |

---

## API Conventions

- **Base URL:** `http://{host}/api/v1`
- **Format:** JSON with `Content-Type: application/json`
- **Auth:** Bearer JWT in `Authorization` header
- **Pagination:** `?page=0&size=20&sort=createdAt,desc`
- **Filtering:** `?category=electronics&minPrice=10&maxPrice=100`
- **Response envelope:** See [convention.md](convention.md) for standard response format
