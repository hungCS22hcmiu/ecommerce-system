# Plan: Order-to-Delivered — Seller Management & Multi-Seller Cart

## Context

Orders currently have no seller concept. `PUT /orders/:id/ship` is nginx-blocked and has no seller UI — orders can never reach SHIPPED, so customers can never trigger the deliver → review flow. This plan adds:

1. **Same-seller order enforcement**: an order contains items from one seller only
2. **Seller order management**: sellers see incoming orders, mark them as shipped
3. **Customer delivery confirmation**: customer marks order Delivered, then reviews
4. **Notifications**: lightweight in-app alerts for key events

---

## Current State

| Area | State |
|---|---|
| `orders` / `order_items` tables | No `seller_id` column |
| `ProductServiceClient.ProductDetail` | Only `name`, `price` — no `sellerId` |
| `PUT /orders/:id/ship` | nginx returns 403 unconditionally |
| `shipOrder` in controller | Calls `updateOrderStatus` with no seller validation (any user can ship) |
| Cart / CheckoutPage | Sends ALL cart items in one order regardless of sellers |
| Seller UI | Products only — no orders page |
| Notifications table | Exists but typed `EMAIL`/`SMS` only, no `is_read` flag |

---

## Architecture Decisions

### 1. Same-Seller Order Enforcement

**Chosen: Frontend-groups + backend-validates.**

- CartPage groups items by `sellerId` (already fetching product detail for stock display).
- Each seller group shows its own "Checkout [N items] with this seller" button.
- CheckoutPage receives the selected seller's items via React Router state.
- `createOrder` in order-service fetches `sellerId` per product, rejects if mixed sellers.

**Why not auto-split on backend?** Breaking the API contract (POST returns one order) adds complexity and the UX of "you placed 3 orders" is confusing. Frontend grouping is transparent and simpler.

**Trade-off**: Customers cannot buy from two sellers in one transaction. They must check out separately per seller. This is intentional — a single order corresponds to one shipment from one seller.

### 2. Seller ID in Orders

Add `seller_id UUID NOT NULL` to both `orders` and `order_items` tables via Flyway V2 migration in order-service. `orders.seller_id` is derived during `createOrder` (all items from same seller, so take the first). `order_items.seller_id` is denormalized for easier querying.

### 3. Ship Endpoint Security — Current Bug

**Critical**: `PUT /orders/:id/ship` currently calls `updateOrderStatus` with no validation that the caller is the order's seller. Any authenticated user who knows an order ID can ship it.

**Fix**: Introduce dedicated `shipOrder(UUID orderId, UUID sellerId)` in OrderService that:
1. Acquires pessimistic lock on the order
2. Validates `order.getSellerId().equals(sellerId)` — throws 403 if not
3. Calls `updateOrderStatus` with SHIPPED

**Nginx**: Remove the `location ~ ^/api/v1/orders/[^/]+/ship$ { return 403; }` block. Validation moves to order-service.

### 4. Cancel vs Ship Race Condition — Already Solved

The existing `findByIdWithLock()` (pessimistic `SELECT … FOR UPDATE`) already handles this:

```
Seller calls ship()      Buyer calls cancel()     Outcome
──────────────────────   ────────────────────────  ──────────────────
Acquires lock            Waits for lock            Ship wins → SHIPPED
                                                   Cancel fails (SHIPPED→CANCELLED invalid)
OR:
Waits for lock           Acquires lock             Cancel wins → CANCELLED
                                                   Ship fails (CANCELLED→SHIPPED invalid)
```

Both outcomes are correct. No additional code is needed. The state machine already rejects invalid transitions.

**Additional invariant to add**: only the buyer (`order.userId`) can cancel; only the seller (`order.sellerId`) can ship. This is validated per-method, not just by state machine.

### 5. Notifications — Lightweight In-App Polling

**Decision: No new service. Extend existing order-service notifications table.**

A new notification microservice is architecturally clean but adds significant overhead: new Docker container, new DB connection, Kafka consumer, nginx routing, SSE/WebSocket for real-time. For an internship system, that's premature.

**Instead**:
- V2 migration adds `IN_APP` to the `notification_type` enum and adds `is_read BOOLEAN DEFAULT FALSE` to the existing `notifications` table.
- Order-service creates notification records within the same DB transaction as status changes (atomic, no partial state).
- Frontend polls `GET /api/v1/orders/notifications?unread=true` every 30 seconds for badge count.
- Review notification to seller: product-service creates a record in `product_reviews`-adjacent logic by calling a new internal endpoint `POST /api/v1/orders/notifications/seller` (from product-service → order-service).

**Why not SSE?** SSE requires persistent server connections, nginx buffering config changes, and reconnect logic. Polling at 30s is imperceptible to users and eliminates all of this complexity.

**Why not WebSocket?** Same concerns, plus requires a stateful server — conflicts with horizontal scaling.

**Future path to real-time**: When needed, extract to a notification-service consuming Kafka events and serving SSE/WebSocket. The Kafka events are already published by order-service, so the migration is additive.

---

## Implementation Phases

---

### Phase 1: Order-Seller Foundation (Backend Only)

**Goal**: Orders know their seller. Ship endpoint is secure and open. Seller can list their orders.

#### 1a. Flyway V2 Migration (`order-service/src/main/resources/db/migration/V2__add_seller_context.sql`)

```sql
ALTER TABLE orders      ADD COLUMN seller_id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE order_items ADD COLUMN seller_id UUID NOT NULL DEFAULT gen_random_uuid();
-- Remove defaults after backfill:
ALTER TABLE orders      ALTER COLUMN seller_id DROP DEFAULT;
ALTER TABLE order_items ALTER COLUMN seller_id DROP DEFAULT;

CREATE INDEX idx_orders_seller_id    ON orders(seller_id);
CREATE INDEX idx_orders_seller_status ON orders(seller_id, status);
```

*Note*: The `DEFAULT gen_random_uuid()` satisfies NOT NULL for existing rows; in dev the DB is wiped on migration so this is harmless. In production, a real backfill from product-service would be needed first.

#### 1b. ProductServiceClient — Add `sellerId`

**File**: `order-service/src/main/java/com/ecommerce/order_service/client/ProductServiceClient.java`

Change `ProductDetail`:
```java
public static class ProductDetail {
    private String name;
    private BigDecimal price;
    private String sellerId;   // UUID as string
}
```

The product-service `GET /products/:id` response already includes `sellerId` (it's in `ProductResponse`). Spring's `RestTemplate` just wasn't mapping it before.

#### 1c. Order Model

**File**: `order-service/src/main/java/com/ecommerce/order_service/model/Order.java`

Add:
```java
@Column(name = "seller_id", nullable = false)
private UUID sellerId;
```

**File**: `order-service/src/main/java/com/ecommerce/order_service/model/OrderItem.java`

Add:
```java
@Column(name = "seller_id", nullable = false)
private UUID sellerId;
```

#### 1d. OrderServiceImpl — Update `createOrder`

After fetching `ProductDetail` for each item, collect sellerIds and validate:

```java
// After building items loop:
Set<UUID> sellerIds = order.getItems().stream()
    .map(OrderItem::getSellerId)
    .collect(Collectors.toSet());
if (sellerIds.size() != 1) {
    // Release all reserved stock
    releaseAllStock(items, userId);
    throw new IllegalArgumentException(
        "All items in an order must be from the same seller");
}
UUID sellerId = sellerIds.iterator().next();
order.setSellerId(sellerId);
```

Inside the item-building loop, set `item.setSellerId(UUID.fromString(product.getSellerId()))`.

#### 1e. New `shipOrder` Method (Security Fix)

**File**: `order-service/src/main/java/com/ecommerce/order_service/service/impl/OrderServiceImpl.java`

```java
@Transactional
public OrderResponse shipOrder(UUID orderId, UUID sellerId) {
    Order order = orderRepository.findByIdWithLock(orderId)
        .orElseThrow(() -> new OrderNotFoundException(orderId));
    if (!order.getSellerId().equals(sellerId)) {
        throw new OrderAccessDeniedException(orderId,
            "Only the seller can ship this order");
    }
    return updateOrderStatus(orderId, OrderStatus.SHIPPED,
        "Shipped by seller", sellerId.toString());
}
```

Update `OrderController.shipOrder` to call `orderService.shipOrder(id, actorId)`.
Update `OrderService` interface with `OrderResponse shipOrder(UUID orderId, UUID sellerId)`.

#### 1f. Seller Order List Endpoint

**File**: `OrderController.java` — new endpoint:
```java
@GetMapping("/seller")
public ApiResponse<List<OrderSummaryResponse>> listSellerOrders(
        @RequestHeader("X-User-Id") UUID sellerId,
        @RequestParam(required = false) OrderStatus status,
        @PageableDefault(size = 20, sort = "createdAt") Pageable pageable) {
    return ApiResponse.ok(orderService.listSellerOrders(sellerId, status, pageable));
}
```

**File**: `OrderRepository.java`:
```java
Page<Order> findBySellerIdOrderByCreatedAtDesc(UUID sellerId, Pageable pageable);
Page<Order> findBySellerIdAndStatusOrderByCreatedAtDesc(UUID sellerId, OrderStatus status, Pageable pageable);
```

**File**: `OrderServiceImpl.java` — implement `listSellerOrders`:
```java
public Page<OrderSummaryResponse> listSellerOrders(UUID sellerId, OrderStatus status, Pageable pageable) {
    Page<Order> page = (status != null)
        ? orderRepository.findBySellerIdAndStatusOrderByCreatedAtDesc(sellerId, status, pageable)
        : orderRepository.findBySellerIdOrderByCreatedAtDesc(sellerId, pageable);
    return page.map(OrderSummaryResponse::from);
}
```

`OrderSummaryResponse` should include `sellerId` and buyer's `userId` (for seller to see who placed the order).

#### 1g. Seller-Side Order Detail Access

Current `getOrder` validates `order.userId == userId` — seller would get 403. Add overload or parameter:
```java
public OrderResponse getOrderAsSeller(UUID orderId, UUID sellerId) {
    Order order = orderRepository.findById(orderId)
        .orElseThrow(() -> new OrderNotFoundException(orderId));
    if (!order.getSellerId().equals(sellerId)) {
        throw new OrderAccessDeniedException(orderId);
    }
    return OrderResponse.from(order);
}
```

New endpoint: `GET /orders/seller/{id}` or use a query param `?role=seller`.
Simpler: `GET /orders/seller/{id}` (separate path, no ambiguity).

#### 1h. Nginx: Remove Ship Block

**File**: `nginx/nginx.conf`

Remove:
```nginx
location ~ ^/api/v1/orders/[^/]+/ship$ {
    return 403;
}
```

Security is now enforced in order-service (seller validation).

---

### Phase 2: Cart Grouping & Per-Seller Checkout (Frontend)

**Goal**: Users see cart items grouped by seller; each group has its own checkout button.

#### 2a. CartPage — Seller Grouping

**File**: `frontend/src/pages/CartPage.tsx`

CartPage already fetches product details via `useQueries` for stock display. These responses contain `sellerId`. Group items by `sellerId`:

```tsx
const sellerGroups = useMemo(() => {
  const groups: Map<string, { sellerId: string; items: CartItem[] }> = new Map()
  items.forEach((item, i) => {
    const product = productQueries[i]?.data?.data
    if (!product) return
    const sid = product.sellerId
    if (!groups.has(sid)) groups.set(sid, { sellerId: sid, items: [] })
    groups.get(sid)!.items.push(item)
  })
  return Array.from(groups.values())
}, [items, productQueries])
```

Render per-group summary + "Checkout [N] items" button that navigates to `/checkout` with `state: { items: group.items }`.

#### 2b. CheckoutPage — Use Provided Items

**File**: `frontend/src/pages/CheckoutPage.tsx`

```tsx
const location = useLocation()
const selectedItems: CartItem[] = location.state?.items ?? cart?.items ?? []
```

Use `selectedItems` instead of `items` from cart for the order. Display a warning if `selectedItems` is empty.

#### 2c. Order Type Update

**File**: `frontend/src/types/order.ts`

```ts
interface Order {
  // existing fields...
  sellerId: string   // add this
}
interface OrderSummaryResponse {
  // existing fields...
  sellerId: string
  buyerUserId: string  // visible to seller
}
```

---

### Phase 3: Seller Order Management UI (Frontend)

**Goal**: Sellers can see, filter, and act on their orders.

#### 3a. New `src/features/orders/sellerOrderApi.ts`

```ts
export const sellerOrderApi = {
  list: (status?: OrderStatus, page = 0) =>
    api.get<ApiResponse<OrderSummary[]>>('/orders/seller', { params: { status, page, size: 20 } }).then(r => r.data),
  get: (id: string) =>
    api.get<ApiResponse<Order>>(`/orders/seller/${id}`).then(r => r.data),
  ship: (id: string) =>
    api.put<ApiResponse<Order>>(`/orders/${id}/ship`).then(r => r.data),
}
```

#### 3b. New `src/features/orders/useSellerOrders.ts`

```ts
export function useSellerOrders(status?: OrderStatus, page = 0) { ... }
export function useSellerOrder(id: string) { ... }
export function useShipOrder() {
  return useMutation({
    mutationFn: (id: string) => sellerOrderApi.ship(id),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: ['seller-orders'] })
      qc.invalidateQueries({ queryKey: ['seller-order', id] })
    },
  })
}
```

#### 3c. New `src/pages/SellerOrdersPage.tsx`

Table with columns: Order ID, Buyer (masked userId or "Customer"), Total, Status, Date, Actions.
Status filter tabs: All | Confirmed | Shipped | Delivered | Cancelled.
"Ship Order" button on CONFIRMED rows → calls `useShipOrder`.

#### 3d. New `src/pages/SellerOrderDetailPage.tsx`

Shows:
- Order ID, status badge, timestamps
- Items list with product name, quantity, unit price
- Shipping address
- Status history timeline (reuses `GET /orders/:id/history` — need seller access, add separate endpoint or reuse with seller role)
- "Mark as Shipped" button (visible when CONFIRMED)

#### 3e. Routes & Navigation

**File**: `src/App.tsx`
```tsx
<Route path="/seller/orders" element={<ProtectedRoute role="seller"><SellerOrdersPage /></ProtectedRoute>} />
<Route path="/seller/orders/:id" element={<ProtectedRoute role="seller"><SellerOrderDetailPage /></ProtectedRoute>} />
```

**File**: Seller navigation component — add "Orders" link alongside "Products".

---

### Phase 4: Notifications (Lightweight Polling)

**Goal**: Sellers see new order count; customers see "your order shipped" alerts.

#### 4a. V3 Migration in Order-Service

```sql
ALTER TYPE notification_type ADD VALUE 'IN_APP';
ALTER TABLE notifications ADD COLUMN is_read BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE notifications ADD COLUMN title VARCHAR(255);
```

(Drop `channel`, `subject`, `body` unused columns or leave them nullable.)

#### 4b. NotificationService in Order-Service

```java
@Service
public class NotificationService {
    @Transactional
    public void notifySeller(UUID sellerId, UUID orderId, String title, String body) {
        notificationRepository.save(Notification.builder()
            .userId(sellerId).orderId(orderId)
            .type(NotificationType.IN_APP)
            .title(title).body(body).isRead(false).build());
    }
    @Transactional
    public void notifyBuyer(UUID buyerId, UUID orderId, String title, String body) { ... }
}
```

Call from:
- `createOrder` → `notifySeller("New order placed", ...)`
- `shipOrder` → `notifyBuyer("Your order has been shipped!", ...)`
- `cancelOrder` by buyer → `notifySeller("Order cancelled by customer", ...)`
- `updateOrderStatus(CONFIRMED)` in PaymentEventConsumer → `notifySeller("Payment confirmed — order ready to ship", ...)`

#### 4c. Notification Endpoint

```java
@GetMapping("/notifications")
public ApiResponse<NotificationSummary> getNotifications(
    @RequestHeader("X-User-Id") UUID userId,
    @RequestParam(defaultValue = "false") boolean unreadOnly) { ... }

@PutMapping("/notifications/mark-read")
public ApiResponse<Void> markAllRead(@RequestHeader("X-User-Id") UUID userId) { ... }
```

Nginx: add `location /api/v1/orders/notifications { proxy_pass http://order_service; }` if needed (the existing `/api/v1/orders` block already routes all sub-paths).

#### 4d. Review Notification to Seller

From product-service `ReviewServiceImpl.createReview`, after saving:
```java
// Async, fire-and-forget — never throws
try {
    orderServiceClient.notifySellerOfReview(product.getSellerId(),
        product.getId(), review.getId());
} catch (Exception e) {
    log.warn("Failed to send review notification", e);
}
```

Add `notifySellerOfReview` to `OrderServiceClient` in product-service:
```java
POST /api/v1/orders/notifications/seller-review
{ sellerId, productId, reviewId, message }
```

This is a fire-and-forget internal call — product-service already calls order-service for purchase verification, so this follows the same pattern.

#### 4e. Frontend Notification Bell

Small badge on the nav bar. Polls every 30s when user is logged in:
```tsx
const { data } = useQuery({
  queryKey: ['notifications', 'unread-count'],
  queryFn: () => orderApi.getNotifications({ unreadOnly: true }),
  refetchInterval: 30_000,
  enabled: isLoggedIn,
})
```

Clicking the bell opens a dropdown panel with recent notifications.

---

## Race Conditions & Security Summary

| Scenario | Mechanism | Outcome |
|---|---|---|
| Seller ships while buyer cancels simultaneously | `findByIdWithLock()` (row-level lock) | One wins, other gets 409 CONFLICT |
| Malicious user ships someone else's order | `order.sellerId == actorId` check in `shipOrder` | 403 FORBIDDEN |
| Malicious user cancels someone else's order | `order.userId == actorId` check in `cancelOrder` | 403 FORBIDDEN (existing) |
| Mixed-seller cart checkout | Backend validates all items same seller | 400 BAD_REQUEST |
| Seller tries to buy own product | `SellerID == userID` in cart-service AddItem | 403 FORBIDDEN (existing) |
| Double-review same purchase | UNIQUE(order_item_id) constraint | 409 ALREADY_REVIEWED (existing) |

---

## Critical Files

| File | Action |
|---|---|
| `order-service/src/main/resources/db/migration/V2__add_seller_context.sql` | NEW |
| `order-service/.../client/ProductServiceClient.java` | ADD `sellerId` to `ProductDetail` |
| `order-service/.../model/Order.java` | ADD `sellerId UUID` |
| `order-service/.../model/OrderItem.java` | ADD `sellerId UUID` |
| `order-service/.../service/impl/OrderServiceImpl.java` | UPDATE `createOrder`; ADD `shipOrder`, `listSellerOrders`, `getOrderAsSeller` |
| `order-service/.../service/OrderService.java` | ADD interface methods |
| `order-service/.../repository/OrderRepository.java` | ADD `findBySellerId*` queries |
| `order-service/.../controller/OrderController.java` | UPDATE `shipOrder`; ADD `/seller`, `/seller/{id}` endpoints |
| `nginx/nginx.conf` | REMOVE ship 403 block |
| `frontend/src/types/order.ts` | ADD `sellerId` |
| `frontend/src/pages/CartPage.tsx` | ADD seller grouping + per-seller checkout |
| `frontend/src/pages/CheckoutPage.tsx` | READ items from router state |
| `frontend/src/features/orders/sellerOrderApi.ts` | NEW |
| `frontend/src/features/orders/useSellerOrders.ts` | NEW |
| `frontend/src/pages/SellerOrdersPage.tsx` | NEW |
| `frontend/src/pages/SellerOrderDetailPage.tsx` | NEW |
| `frontend/src/App.tsx` | ADD seller order routes |
| `order-service/src/main/resources/db/migration/V3__add_notification_in_app.sql` | NEW (Phase 4) |
| `order-service/.../service/NotificationService.java` | NEW (Phase 4) |
| `product-service/.../client/OrderServiceClient.java` | ADD `notifySellerOfReview` (Phase 4) |
| `frontend/src/components/shared/NotificationBell.tsx` | NEW (Phase 4) |

---

## Verification

**Phase 1:**
1. `POST /api/v1/orders` with items from two different sellers → expect 400
2. `POST /api/v1/orders` with items from same seller → order created with `sellerId`
3. `PUT /orders/:id/ship` with wrong `X-User-Id` (not the seller) → expect 403
4. Cancel order as buyer while shipping simultaneously (two concurrent requests) → one 200 one 409

**Phase 2:**
1. Add items from Seller A and Seller B to cart → cart shows two groups
2. Click "Checkout items from Seller A" → checkout page shows only Seller A's items
3. Place order → only Seller A's items in order

**Phase 3:**
1. Log in as seller → `/seller/orders` shows orders where `sellerId` matches
2. Click "Mark as Shipped" on a CONFIRMED order → status changes, button disappears
3. Log in as buyer → order detail shows SHIPPED status

**Phase 4:**
1. Place order → seller gets in-app notification badge
2. Seller ships → buyer gets notification "Your order has been shipped"
3. Buyer cancels → seller gets notification "Order cancelled"
4. Buyer posts review → seller gets notification

**Build command after all phases:**
```bash
docker compose build order-service product-service frontend cart-service && \
docker compose up -d order-service product-service frontend cart-service nginx
```
