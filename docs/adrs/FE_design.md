# Frontend Design Plan — Phase 5 (Weeks 17–20)

## Aesthetic Direction

**Theme: Dark Editorial / Luxury Utility**

A dark, high-contrast interface with editorial typography and amber-gold accents. Think high-end developer tooling meets boutique e-commerce — the kind of UI that signals the backend is serious. Not a generic white SaaS dashboard. Not a purple gradient startup template.

The system already has depth (Kafka sagas, idempotency keys, distributed locks). The frontend should *feel* like it belongs to that system.

**Guiding principles:**
- Dark primary surface (`#0a0a0b`) with layered elevation using subtle borders, not shadows
- One strong accent: amber-gold (`#f59e0b` / `#d97706`) — used sparingly on CTAs, active states, status indicators
- Typography: `DM Serif Display` for headings (editorial weight) + `JetBrains Mono` for order IDs / prices / status labels (data feels like data) + `Inter` for body copy
- Motion: staggered entrance animations on page load only; hover states on cards; skeleton pulses. Nothing gratuitous.
- Status colors: gray=PENDING, blue=CONFIRMED, amber=SHIPPED, emerald=DELIVERED, red=CANCELLED/PAYMENT_FAILED

**What this achieves in interviews:** When you demo the project, the UI looks like it came from a senior engineer — not a bootcamp. The dark theme with amber accents photographs well in screen shares.

---

## Tech Stack (from six-month plan)

| Tool | Version | Role |
|---|---|---|
| React 18 + TypeScript | latest | Component model + type safety |
| Vite 5 | latest | Dev server (HMR) + build |
| TanStack Query 5 | latest | Server state, caching, polling |
| Zustand 4 | latest | Auth token (memory) + cart badge count |
| Axios 1 | latest | HTTP client + 401 interceptor |
| React Router 6.4+ | latest | Protected routes via `<Outlet>` |
| Tailwind CSS 3 | latest | Utility-first styling |
| shadcn/ui | latest | Accessible component primitives |

**shadcn/ui components to install:** Button, Input, Badge, Skeleton, Dialog, Table, Sheet, Toast, Card, Separator, Avatar, DropdownMenu, RadioGroup, ScrollArea, Tooltip

---

## Project Structure

```
frontend/
├── public/
├── src/
│   ├── lib/
│   │   ├── axios.ts           # Axios instance + queue-based 401 interceptor
│   │   ├── queryClient.ts     # TanStack Query config (staleTime defaults, retry)
│   │   └── utils.ts           # cn(), formatCurrency, formatDate, truncateId
│   ├── store/
│   │   ├── authStore.ts       # accessToken (memory), refreshToken ref, userId, email
│   │   └── cartStore.ts       # itemCount (for Navbar badge only)
│   ├── types/
│   │   ├── api.ts             # ApiResponse<T>, PaginationMeta, ApiError
│   │   ├── auth.ts            # LoginRequest, RegisterRequest, AuthResponse, UserProfile
│   │   ├── product.ts         # Product, ProductListParams, StockLevel
│   │   ├── cart.ts            # Cart, CartItem, AddItemRequest
│   │   ├── order.ts           # Order, OrderSummary, CreateOrderRequest, OrderStatus, OrderStatusHistory
│   │   └── payment.ts         # Payment, PaymentStatus
│   ├── components/
│   │   ├── ui/                # shadcn/ui generated components (do not hand-edit)
│   │   ├── layout/
│   │   │   ├── Navbar.tsx     # Logo, nav links, cart badge, user menu
│   │   │   ├── Footer.tsx     # Minimal: links + copyright
│   │   │   └── PageLayout.tsx # Consistent max-width wrapper + padding
│   │   └── shared/
│   │       ├── LoadingSpinner.tsx
│   │       ├── ErrorMessage.tsx   # Renders envelope error.message
│   │       ├── EmptyState.tsx     # Icon + message + optional CTA
│   │       └── Pagination.tsx     # Reads PaginationMeta, renders page controls
│   ├── features/
│   │   ├── auth/
│   │   │   ├── authApi.ts
│   │   │   ├── useAuth.ts         # login, register, logout, refresh mutations
│   │   │   ├── LoginForm.tsx
│   │   │   ├── RegisterForm.tsx
│   │   │   └── ProtectedRoute.tsx # Outlet wrapper; silent refresh on mount
│   │   ├── products/
│   │   │   ├── productApi.ts
│   │   │   ├── useProducts.ts     # list, search, single
│   │   │   ├── ProductCard.tsx
│   │   │   ├── ProductGrid.tsx    # Renders 8 Skeleton cards while loading
│   │   │   └── SearchBar.tsx      # Debounced input (300ms)
│   │   ├── cart/
│   │   │   ├── cartApi.ts
│   │   │   ├── useCart.ts         # GET cart query
│   │   │   ├── useCartMutations.ts # addItem, updateQty, removeItem — all optimistic
│   │   │   ├── CartDrawer.tsx     # shadcn Sheet (slide-over)
│   │   │   └── CartItem.tsx
│   │   ├── orders/
│   │   │   ├── orderApi.ts
│   │   │   ├── useOrders.ts       # list + single + history + create + cancel
│   │   │   ├── OrderList.tsx      # shadcn Table
│   │   │   ├── OrderTimeline.tsx  # Vertical timeline from status history
│   │   │   ├── CheckoutForm.tsx   # Address selector + place order button
│   │   │   └── StatusBadge.tsx    # Color-coded by OrderStatus
│   │   └── payment/
│   │       ├── paymentApi.ts
│   │       ├── usePaymentStatus.ts  # refetchInterval polling hook
│   │       └── PaymentStatusPoller.tsx  # Spinner → success/error state
│   └── pages/
│       ├── HomePage.tsx
│       ├── LoginPage.tsx
│       ├── RegisterPage.tsx
│       ├── ProductListPage.tsx
│       ├── ProductDetailPage.tsx
│       ├── CartPage.tsx
│       ├── CheckoutPage.tsx
│       ├── OrderConfirmationPage.tsx
│       ├── OrderHistoryPage.tsx
│       ├── OrderDetailPage.tsx
│       └── ProfilePage.tsx
├── index.html
├── vite.config.ts             # /api proxy → http://localhost:80
├── tailwind.config.ts         # custom colors, fonts
├── tsconfig.json
├── .env.local                 # VITE_API_BASE_URL=http://localhost/api/v1
├── Dockerfile                 # multi-stage: build → nginx serve
└── README.md
```

---

## Design System

### Colors (tailwind.config.ts additions)

```ts
colors: {
  surface: {
    base:    '#0a0a0b',   // page background
    raised:  '#111113',   // card background
    overlay: '#18181b',   // modal / drawer
    border:  '#27272a',   // subtle dividers
  },
  accent: {
    DEFAULT: '#f59e0b',   // amber — primary CTA, active nav
    dim:     '#d97706',   // hover state
    muted:   '#92400e',   // disabled / tertiary
  },
  status: {
    pending:   '#71717a',  // zinc-500
    confirmed: '#3b82f6',  // blue-500
    shipped:   '#f59e0b',  // amber (reuse accent)
    delivered: '#10b981',  // emerald-500
    failed:    '#ef4444',  // red-500
    cancelled: '#6b7280',  // gray-500
  }
}
```

### Typography (index.html `<head>` + tailwind config)

```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=DM+Serif+Display&family=JetBrains+Mono:wght@400;500&family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
```

```ts
fontFamily: {
  display: ['"DM Serif Display"', 'serif'],   // page titles, product names
  mono:    ['"JetBrains Mono"', 'monospace'],  // IDs, prices, status labels
  sans:    ['Inter', 'sans-serif'],            // body, forms, nav
}
```

### Elevation model (no box-shadow — border-based)

```
Level 0: surface-base     bg-surface-base
Level 1: surface-raised   bg-surface-raised border border-surface-border
Level 2: surface-overlay  bg-surface-overlay border border-surface-border
```

---

## Page Designs

### Navbar

```
[Logo "SHOP"]  [Products] [Orders]        [Cart 🛒 (3)] [User ▾]
              ─────────────────────────────────────────────────
```

- Dark `bg-surface-base` with `border-b border-surface-border` — not sticky, just static top bar
- Logo in `font-display` text-white; nav links in `font-sans text-sm text-zinc-400 hover:text-white`
- Cart icon shows `cartStore.itemCount` as an amber Badge
- User dropdown (shadcn DropdownMenu): Profile · Orders · Logout
- Mobile: hamburger collapses to a Sheet

---

### HomePage (`/`)

Layout: hero → featured products grid

```
┌─────────────────────────────────────────────────────────┐
│  [hero: full-width dark panel]                          │
│  "Everything you need,                                  │ ← font-display, 72px
│   shipped to your door."                               │
│  [Browse Products →]                    [amber button]  │
├─────────────────────────────────────────────────────────┤
│  Featured Products                      ← font-sans     │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐                  │
│  │ card │ │ card │ │ card │ │ card │   ← 4-col grid    │
│  └──────┘ └──────┘ └──────┘ └──────┘                  │
└─────────────────────────────────────────────────────────┘
```

- Hero panel: `bg-surface-raised` with a subtle radial amber glow at bottom-right (`radial-gradient` in CSS)
- Entrance animation: hero text fades up on mount (`@keyframes fadeUp` with `animation-delay` stagger)
- ProductGrid below with `staleTime: 30_000`; first 8 products

---

### ProductListPage (`/products`)

```
┌────────────────────────────────────────────────────────┐
│  [SearchBar — full width, debounced 300ms]             │
├────────────────────────────────────────────────────────┤
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐                 │
│  │ card │ │ card │ │ card │ │ card │  ← 4-col         │
│  │ card │ │ card │ │ card │ │ card │                  │
│  └──────┘ └──────┘ └──────┘ └──────┘                 │
│  [← 1  2  3  4 →]   ← Pagination                     │
└────────────────────────────────────────────────────────┘
```

- `useSearchParams` drives `page` and `q`; URL is shareable
- `enabled: query.length >= 2` for search queries
- Skeleton: 8 `<Skeleton>` cards `bg-surface-raised animate-pulse`
- `EmptyState` when no results: "No products found for '{query}'"

**ProductCard anatomy:**
```
┌──────────────────────┐
│  [product image area] │  ← bg-surface-raised aspect-square placeholder
│  ─────────────────── │
│  Product Name         │  ← font-display truncate
│  $99.99               │  ← font-mono text-accent
│  [In Stock ✓]         │  ← Badge: emerald / amber / red
│  [Add to Cart]        │  ← Button: full width, amber on hover
└──────────────────────┘
```

---

### ProductDetailPage (`/products/:id`)

```
┌──────────────────────────────────────────────────────┐
│  ┌──────────────────┐  │  Product Name (font-display) │
│  │                  │  │  $129.99 (font-mono, large)  │
│  │   image area     │  │  ─────────────────────────── │
│  │   (placeholder)  │  │  Description (font-sans)     │
│  │                  │  │                              │
│  └──────────────────┘  │  Stock: [14 available]       │
│                         │                              │
│                         │  Qty: [─] [1] [+]           │
│                         │  [Add to Cart]  ← amber btn │
└──────────────────────────────────────────────────────┘
```

- 2-column layout above md; stacks on mobile
- Stock level from `GET /inventory/:id` — color-coded with status colors

---

### CartPage (`/cart`) + CartDrawer

**CartDrawer** (shadcn Sheet, slides from right):
- Opens when cart icon is clicked from any page
- Lists CartItems with remove / qty controls
- "Proceed to Checkout" button at bottom → navigates to `/checkout`
- Optimistic mutations: badge and list update before network response

**CartPage** (dedicated full-page view):
```
┌─────────────────────────────┬─────────────────────┐
│  Your Cart (3 items)        │  Order Summary       │
│  ─────────────────────────  │  Subtotal   $297.00  │
│  [item row] [qty] [$price]  │  Shipping   Free     │
│  [item row] [qty] [$price]  │  ─────────────────── │
│  [item row] [qty] [$price]  │  Total      $297.00  │
│                             │  [Checkout →]        │
└─────────────────────────────┴─────────────────────┘
```

- EmptyState when cart is empty: "Your cart is empty" + Browse Products link

---

### CheckoutPage (`/checkout`)

```
┌─────────────────────────────┬─────────────────────┐
│  Shipping Address            │  Cart Summary       │
│  ─────────────────────────── │  [items list]       │
│  ◉ 123 Main St (Default)    │                     │
│  ○ 456 Oak Ave              │  Total  $297.00     │
│  [+ Add new address]        │                     │
│                             │  [Place Order →]    │
└─────────────────────────────┴─────────────────────┘
```

- Address list from `GET /users/profile` as RadioGroup (shadcn)
- "Place Order" calls `POST /orders`; on success → navigate to `/orders/:id/confirmation`
- Button shows spinner during mutation; disabled to prevent double-submit

---

### OrderConfirmationPage (`/orders/:id/confirmation`)

**This page showcases the Kafka saga visually.**

```
┌──────────────────────────────────────────────────────┐
│  Order #a3f2...  Placed                              │
│  ──────────────────────────────────────────────────  │
│                                                      │
│  Payment Status                                      │
│  ┌──────────────────────────────────────────────┐   │
│  │  ⟳  Processing payment...                   │   │  ← spinner, PENDING
│  │                                              │   │
│  │  [amber pulsing ring animation]              │   │
│  └──────────────────────────────────────────────┘   │
│                                                      │
│  ✓  Payment confirmed            → shows on success  │
│  ✗  Payment failed               → shows on failure  │
│                                                      │
│  Redirecting to order detail in 3s...               │
└──────────────────────────────────────────────────────┘
```

- `usePaymentStatus(orderId)` polls `GET /payments/order/:id` every 2s
- `refetchInterval` stops when status is CONFIRMED / PAYMENT_FAILED / CANCELLED
- Pulsing amber ring: `@keyframes ping` (Tailwind `animate-ping`) around a status icon
- Auto-redirect after 3s on terminal status using `setTimeout` + `navigate`
- **This is the demo moment**: visitor watches PENDING flip to CONFIRMED in real time

---

### OrderHistoryPage (`/orders`)

```
┌────────────────────────────────────────────────────────┐
│  Order History                                         │
│  ──────────────────────────────────────────────────── │
│  Order ID       Date         Total    Status           │
│  a3f2b1...      May 08       $297     [CONFIRMED]      │
│  c9d4e2...      May 07       $45      [DELIVERED]      │
│  f1a2b3...      May 06       $120     [CANCELLED]      │
│                                                        │
│  [← 1  2  3 →]                                        │
└────────────────────────────────────────────────────────┘
```

- shadcn Table; order IDs truncated to 8 chars in `font-mono`
- Status badges use the `status.*` color map defined above
- Clicking a row → `/orders/:id`
- Pagination driven by `useSearchParams`

---

### OrderDetailPage (`/orders/:id`)

```
┌───────────────────────────────┬──────────────────────┐
│  Order #a3f2b1c4              │  Status Timeline     │
│  Placed May 08, 2026          │                      │
│  ─────────────────────────    │  ● PENDING           │
│  Items                        │  │ May 08 10:02      │
│  Widget A × 2   $49.98        │  ● CONFIRMED         │
│  Gadget B × 1   $99.00        │  │ May 08 10:02      │
│  ─────────────────────────    │  ○ (next step)       │
│  Total  $148.98               │                      │
│                               │  [Cancel Order]      │
│  Shipping to:                 │  (if PENDING/CONF.)  │
│  123 Main St                  │                      │
└───────────────────────────────┴──────────────────────┘
```

- `OrderTimeline` renders from `GET /orders/:id/history`
- Timeline dots color-coded by status; current status is the last filled dot
- Cancel button only shown if status is PENDING or CONFIRMED; calls `PUT /orders/:id/cancel`

**OrderTimeline component anatomy:**
```
● ─── PENDING       (zinc dot)
│     May 08, 10:02:34
│
● ─── CONFIRMED     (blue dot)
│     May 08, 10:02:41
│
○ ─── SHIPPED       (empty dot — not yet reached)
```
- Vertical line connecting dots: `border-l border-surface-border ml-2`
- Status icon per state: clock / checkmark / truck / home / x

---

### ProfilePage (`/profile`)

```
┌──────────────────────────────────────────────────────┐
│  ┌─────┐  John Doe                                   │
│  │  JD │  john@example.com                           │
│  └─────┘                                             │
│  ───────────────────────────────────────────────     │
│  Personal Info               Addresses               │
│  ─────────────────           ──────────────────      │
│  First Name  [John     ]     ● 123 Main (Default)    │
│  Last Name   [Doe      ]     ○ 456 Oak Ave           │
│  [Save Changes]              [+ Add Address]         │
└──────────────────────────────────────────────────────┘
```

- Avatar initials from `firstName[0] + lastName[0]` — `bg-accent text-surface-base font-mono`
- Address list with Edit (opens Dialog) / Delete / Set Default actions
- Toast notification on successful save

---

### Login / Register Pages

Centered card layout, full-height dark background:

```
                ┌─────────────────────┐
                │  SHOP               │  ← font-display logo
                │  ─────────────────  │
                │  Email              │
                │  [________________] │
                │  Password           │
                │  [________________] │
                │                     │
                │  [Sign In]          │  ← full-width amber button
                │                     │
                │  No account?        │
                │  Create one →       │
                └─────────────────────┘
```

- Card: `bg-surface-raised border border-surface-border rounded-lg p-8`
- Input focus ring: `focus:ring-1 focus:ring-accent`
- Error inline below each field; global error Toast on network failure
- Redirect to `?from=` param after login

---

## Key Technical Patterns

### 1. Queue-Based 401 Interceptor (`src/lib/axios.ts`)

```ts
// Pseudo-code — implement fully in Week 17
let isRefreshing = false;
let failedQueue: { resolve: (token: string) => void; reject: (err: unknown) => void }[] = [];

axiosInstance.interceptors.response.use(
  (res) => res,
  async (error) => {
    const original = error.config;
    if (error.response?.status === 401 && !original._retry) {
      if (isRefreshing) {
        // Queue this request until refresh completes
        return new Promise((resolve, reject) => {
          failedQueue.push({ resolve, reject });
        }).then((token) => {
          original.headers['Authorization'] = `Bearer ${token}`;
          return axiosInstance(original);
        });
      }
      original._retry = true;
      isRefreshing = true;
      try {
        const { data } = await axiosInstance.post('/auth/refresh', { ... });
        const newToken = data.data.access_token;
        authStore.getState().setToken(newToken);
        failedQueue.forEach(({ resolve }) => resolve(newToken));
        failedQueue = [];
        return axiosInstance(original);
      } catch (refreshError) {
        failedQueue.forEach(({ reject }) => reject(refreshError));
        authStore.getState().clearToken();
        window.location.href = '/login';
      } finally {
        isRefreshing = false;
      }
    }
    return Promise.reject(error);
  }
);
```

### 2. Optimistic Cart Mutations (`useCartMutations.ts`)

```ts
// addItem — pattern for updateQuantity and removeItem too
addItem: useMutation({
  mutationFn: cartApi.addItem,
  onMutate: async (newItem) => {
    await queryClient.cancelQueries({ queryKey: ['cart'] });
    const snapshot = queryClient.getQueryData<ApiResponse<Cart>>(['cart']);
    queryClient.setQueryData(['cart'], (old) => optimisticAdd(old, newItem));
    cartStore.getState().increment();  // Navbar badge
    return { snapshot };
  },
  onError: (_err, _vars, ctx) => {
    queryClient.setQueryData(['cart'], ctx?.snapshot);
    cartStore.getState().decrement();
  },
  onSettled: () => queryClient.invalidateQueries({ queryKey: ['cart'] }),
}),
```

### 3. Payment Status Polling (`usePaymentStatus.ts`)

```ts
const TERMINAL_STATUSES = ['CONFIRMED', 'PAYMENT_FAILED', 'CANCELLED'];

export function usePaymentStatus(orderId: string) {
  return useQuery({
    queryKey: ['payment', 'order', orderId],
    queryFn: () => paymentApi.getByOrderId(orderId),
    refetchInterval: (query) => {
      const status = query.state.data?.data?.status;
      return TERMINAL_STATUSES.includes(status ?? '') ? false : 2000;
    },
    refetchIntervalInBackground: true,
    enabled: !!orderId,
  });
}
```

### 4. ProtectedRoute (`ProtectedRoute.tsx`)

```ts
export function ProtectedRoute() {
  const token = useAuthStore((s) => s.accessToken);
  const location = useLocation();
  const [checking, setChecking] = useState(!token);

  useEffect(() => {
    if (!token) {
      // Attempt silent refresh — POST /auth/refresh using stored refreshToken
      silentRefresh()
        .catch(() => {/* will redirect below */})
        .finally(() => setChecking(false));
    }
  }, []);

  if (checking) return <LoadingSpinner />;
  if (!token) return <Navigate to={`/login?from=${location.pathname}`} replace />;
  return <Outlet />;
}
```

---

## Implementation Schedule

### Week 17 — Scaffold + Auth
**Goal:** Working login, register, protected routes, JWT interceptor with queue

1. `npm create vite@latest frontend -- --template react-ts`
2. Install deps: TanStack Query, Zustand, Axios, React Router, Tailwind, shadcn/ui
3. Configure Tailwind (`tailwind.config.ts`) with custom colors + fonts
4. Add Google Fonts link to `index.html`
5. Wire Vite proxy: `/api → http://localhost:80`
6. Define all types in `src/types/` (do this first — TypeScript guides everything)
7. Build `src/lib/axios.ts` with queue-based interceptor
8. Build `authStore.ts`, `authApi.ts`
9. Build `LoginForm`, `RegisterForm`, `ProtectedRoute`, router
10. Build `Navbar` (static, no cart badge yet)

**Done when:** Register → login → `/products` loads; token expiry → silent refresh → stays on page; two simultaneous 401s → only one refresh fires.

---

### Week 18 — Product Catalog
**Goal:** Full browsable catalog with search, pagination, detail page

1. `productApi.ts` — list, search, getById
2. `useProducts` hooks — staleTime aligned with backend cache TTLs
3. `ProductCard`, `ProductGrid` (with Skeleton loading)
4. `SearchBar` (debounced 300ms)
5. `Pagination` component reading `PaginationMeta`
6. `ProductListPage` — URL-driven state via `useSearchParams`
7. `ProductDetailPage` — image placeholder, qty selector, Add to Cart (wired in Week 19)
8. `HomePage` — hero section + first 8 products

**Done when:** Browse → search → detail → back (cache hit, no network request); page 2 URL shareable; skeleton visible before data; out-of-stock disables Add to Cart.

---

### Week 19 — Cart + Checkout + Order + Payment Polling
**Goal:** Complete purchase flow visible in browser; Kafka saga transparent

Days 1–2 (Cart):
1. `cartApi.ts`, `useCart`, `useCartMutations`
2. `CartDrawer` (shadcn Sheet) + `CartItem`
3. `CartPage` (full page view)
4. Wire Add to Cart on ProductCard and ProductDetailPage
5. `cartStore` — itemCount badge in Navbar

Days 3–4 (Checkout + Order):
1. `orderApi.ts`, `useCreateOrder`
2. `CheckoutPage` — address selector from profile, Place Order button
3. On success: invalidate `['cart']`, navigate to confirmation page

Days 5–6 (Payment Polling):
1. `paymentApi.ts`, `usePaymentStatus`
2. `PaymentStatusPoller` — spinner → success/error state with amber ping animation
3. `OrderConfirmationPage` — order summary + poller + 3s auto-redirect

**Done when:** Add to Cart → Navbar badge increments immediately; checkout → place order → watch payment flip PENDING→CONFIRMED on confirmation page without refresh; polling stops after terminal status.

---

### Week 20 — Order History + Profile + Polish + Docker
**Goal:** Every feature works; error states handled; Docker image built; zero TypeScript errors

Days 1–2 (Orders):
1. `useOrders`, `useOrder`, `useCancelOrder`
2. `OrderList` (shadcn Table), `StatusBadge`
3. `OrderTimeline` (vertical timeline from status history)
4. `OrderHistoryPage`, `OrderDetailPage`
5. Cancel flow with mutation + invalidation

Days 3–4 (Profile):
1. `profileApi.ts`
2. `ProfileForm` — update name, Toast on success
3. `AddressManager` — list + Dialog for add/edit + delete + set default
4. Wire default address pre-selection on CheckoutPage

Days 5–6 (Polish + Docker):
1. Audit: every `useQuery` has error state with `<ErrorMessage>`
2. Audit: every page has skeleton on first load, never blank
3. Audit: empty states on cart, order history, no search results
4. Global Axios catch-all for network errors → Toast
5. `frontend/Dockerfile` multi-stage: `npm run build` → `nginx:alpine` serve
6. Add `frontend` service to `docker-compose.yml`
7. Update root Nginx: serve Vite build at `/`, proxy `/api` to services
8. `npm run build` — zero TypeScript errors
9. `frontend/README.md`

**Done when:** Full demo: register → browse → add to cart → checkout → payment polls to CONFIRMED → order history → order detail with timeline → cancel → cancelled state shown. `docker compose up` serves frontend at `http://localhost`.

---

## Verification Checklist

```bash
# Dev mode
cd frontend && npm run dev
# App runs at http://localhost:5173, API proxied to port 80

# TypeScript check (no errors)
npm run build

# Docker build + serve
docker compose up --build frontend
# App served at http://localhost (Nginx serves build + proxies /api)

# JWT queue interceptor
# 1. Log in, copy access token
# 2. Expire it (wait 15min or patch authStore in devtools)
# 3. Navigate to a protected page with 3 components that fetch simultaneously
# 4. Network tab: exactly ONE POST /auth/refresh fires; all three requests succeed

# Optimistic cart
# 1. Add item → Navbar badge increments before response arrives
# 2. Disable network (DevTools) → Add item → badge reverts on failure

# Payment polling
# 1. Place order → watch confirmation page
# 2. Network tab: GET /payments/order/:id fires every 2s
# 3. Once CONFIRMED: polling stops, no more requests

# E2E smoke
bash script/e2e-test.sh    # backend still 14/14
bash script/e2e-payment.sh # backend still 12/12
```

---

## Notes for Interview

When asked about the frontend:

**"Why Zustand for auth token and not localStorage?"**
XSS threat model. `localStorage` is readable by any JS on the page. Zustand keeps the access token in memory — a compromised script can't exfiltrate it. Trade-off: token disappears on page refresh, requiring a silent refresh on every page load (which I handle in `ProtectedRoute`).

**"How does your 401 interceptor handle concurrent requests?"**
Without a queue, five components mounting simultaneously on a stale-token page each fire a 401 → each tries to refresh → four fail because the refresh token rotates after first use. My interceptor sets `isRefreshing = true` on the first 401, queues all subsequent requests as promise callbacks, and replays them all once the single refresh succeeds. Same pattern as the backend's idempotency — one winner, rest wait.

**"Walk me through 'Place Order' to 'Confirmed' from the browser's view."**
`POST /orders` returns 201 with `orderId` while the order is still PENDING. Frontend navigates to the confirmation page, which mounts `usePaymentStatus` — a TanStack Query polling every 2s against `GET /payments/order/:id`. Behind the scenes, Kafka is delivering `orders.created` to payment-service, which processes the charge and publishes `payments.completed`, which order-service consumes to transition the order. The next poll returns CONFIRMED, `refetchInterval` returns `false`, polling stops, UI shows success. The entire Kafka saga is visible in the browser as a 2–5 second status flip.
