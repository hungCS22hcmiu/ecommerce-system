# frontend

React 19 SPA for the distributed e-commerce platform. Built with TypeScript, Vite, TanStack Query, Zustand, and Tailwind CSS. Served via Nginx in Docker on port **3001**; all API calls go through the Nginx gateway on port **80**.

## Stack

| Tool | Version | Purpose |
|---|---|---|
| React 19 + TypeScript | latest | UI with compile-time API contract safety |
| Vite 8 | latest | Fast HMR dev server; multi-stage Docker build |
| TanStack Query 5 | latest | Server state, caching, polling, optimistic updates |
| Zustand 5 | latest | `accessToken` in memory (XSS-safe); cart item count badge |
| Axios | 1.x | Queue-based 401→refresh→retry interceptor |
| React Router 6 | latest | Protected route wrappers via `<Outlet>` |
| Tailwind CSS 4 | latest | Utility-first styling |
| shadcn/ui | latest | Button, Badge, Skeleton, Toast, Sheet — source-copied into repo |

## Running

**Dev server (hot reload)**
```bash
cd frontend
npm install
npm run dev       # http://localhost:3001, proxies /api → localhost:80
```

**With Docker (full stack)**
```bash
# From repo root
docker compose up --build -d frontend
# App available at http://localhost:3001
```

## Project Structure

```
src/
├── lib/
│   ├── axios.ts          # Axios instance + queue-based 401 interceptor
│   ├── queryClient.ts    # TanStack Query global config
│   ├── toast.ts          # Global toast helper
│   └── utils.ts          # cn(), formatCurrency, formatDate, extractApiError
├── store/
│   ├── authStore.ts      # Zustand: accessToken (memory), userId, email, role
│   ├── cartStore.ts      # Zustand: itemCount for Navbar badge
│   └── themeStore.ts     # Zustand: dark/light theme (persisted to localStorage)
├── types/                # api.ts, auth.ts, product.ts, cart.ts, order.ts, payment.ts,
│                         # seller.ts, category.ts, notification.ts
├── components/
│   ├── ui/               # Button, Input, Badge, Skeleton, Toast, ThemeToggle, StarRating
│   ├── layout/           # Navbar (role-aware, notification bell)
│   └── shared/           # EmptyState, Pagination, ReviewDialog, NotificationBell, AISearchBadge
├── features/
│   ├── auth/             # LoginForm, RegisterForm, useAuth, authApi, ProtectedRoute
│   ├── products/         # ProductCard, ProductGrid, SearchBar, useProducts, productApi,
│   │                     # useProductAISearch (TanStack Query, staleTime 60s)
│   ├── cart/             # CartDrawer, CartItem, useCart, useCartMutations, cartApi
│   ├── orders/           # OrderTimeline, StatusBadge, useOrders, orderApi,
│   │                     # sellerOrderApi, useSellerOrders
│   ├── payment/          # usePaymentStatus (self-stopping poll), paymentApi
│   ├── profile/          # useProfile, profileApi
│   ├── reviews/          # useReviews, reviewApi (create, update, delete, my-review)
│   ├── notifications/    # useNotifications, notificationApi
│   ├── seller/           # SellerRoute, ProductForm, ProductStatusBadge, DeleteConfirmDialog,
│   │                     # CategoryCombobox, sellerApi, useSellerProducts, useCategories
│   └── sellers/          # sellerProfileApi, useSellerProfile (public seller shop)
└── pages/                # One file per route (see table below)
```

## Pages

| Route | Page | Auth | Notes |
|---|---|---|---|
| `/` | HomePage | No | Featured products |
| `/login` | LoginPage | No (redirect if authed) | |
| `/register` | RegisterPage | No | |
| `/verify-email` | VerifyEmailPage | No | |
| `/products` | ProductListPage | No | Keyword + AI search, category filter |
| `/products/:id` | ProductDetailPage | No | Multi-image gallery, reviews, ratings |
| `/categories` | CategoryBrowsePage | No | Category grid |
| `/categories/:slug` | CategoryProductsPage | No | Products by category |
| `/sellers/:id` | SellerShopPage | No | Public seller profile + products |
| `/cart` | CartPage | Yes | Item images, seller email grouping |
| `/checkout` | CheckoutPage | Yes | Address selector, order summary |
| `/orders/:id/confirmation` | OrderConfirmationPage | Yes | Kafka saga visible — payment polling |
| `/orders` | OrderHistoryPage | Yes | Paginated |
| `/orders/:id` | OrderDetailPage | Yes | Status timeline, product thumbnails |
| `/profile` | ProfilePage | Yes | |
| `/seller/products` | SellerDashboardPage | Seller | List/sort/filter; Highest Rated |
| `/seller/products/new` | SellerCreateProductPage | Seller | |
| `/seller/products/:id/edit` | SellerEditProductPage | Seller | |
| `/seller/orders` | SellerOrdersPage | Seller | Filter by status |
| `/seller/orders/:id` | SellerOrderDetailPage | Seller | |

## Key Patterns

**Queue-based JWT refresh** (`lib/axios.ts`): On 401, first request sets `isRefreshing = true` and calls `POST /auth/refresh`. All concurrent 401s are pushed onto `failedQueue: { resolve, reject }[]`. On refresh success, all queued requests replay with the new token. Without this, 5 simultaneous components on a stale page each trigger a refresh and 4 fail because the refresh token is already rotated.

**Payment polling** (`features/payment/usePaymentStatus.ts`): `refetchInterval` returns `false` when the status is terminal (`CONFIRMED`, `PAYMENT_FAILED`, `CANCELLED`). No `clearInterval`, no `useEffect` cleanup, no memory leaks. The Kafka saga is transparent to the user.

**Optimistic cart updates** (`features/cart/useCartMutations.ts`): `onMutate` snapshots the cache and applies the change locally. `onError` restores the snapshot. `onSettled` always re-syncs with the server. Navbar badge increments instantly.

**Notification navigation** (`components/shared/NotificationBell.tsx`): Clicking a notification calls `navigate`. If `productId` is set → `/products/:id` (review notifications). If `orderId` is set → `/orders/:id` (order status notifications).

**Multi-item review dialog** (`components/shared/ReviewDialog.tsx`): All order items shown simultaneously in a scrollable panel. Per-item state array (`rating`, `comment`). Existing reviews pre-populated via `useMyReviewByOrderItem`. Single Submit button loops through rated items and calls create or update.

**AI search** (`features/products/useProductAISearch.ts`): Enabled when `q.length >= 2`, `staleTime: 60s`. Falls back gracefully if ai-service is unavailable. `AISearchBadge` shown on ProductListPage when active.

## Testing

TypeScript compile check runs as part of Docker build (`tsc -b && vite build`). No separate test suite for the frontend — golden path and edge cases are validated manually.
