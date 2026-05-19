// Phase 2.C — 50 VU composite checkout (testing_target.md §3.A).
// Chain: login (once in setup) → product search → add-to-cart → create-order.
// Target: error rate < 0.1%, full-chain P95 < 1000ms.
//
// Expected to FAIL given Phase 1 findings (saga TTC + order 409 stock at sustained load).

import http from 'k6/http';
import { check } from 'k6';
import { Trend } from 'k6/metrics';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import { login, seedHighStockProduct } from './lib/auth.js';

const AUTH_URL    = __ENV.AUTH_URL    || 'http://localhost:8001';
const PRODUCT_URL = __ENV.PRODUCT_URL || 'http://localhost:8081';
const CART_URL    = __ENV.CART_URL    || 'http://localhost:8002';
const ORDER_URL   = __ENV.ORDER_URL   || 'http://localhost:8082';
const SEED_STOCK  = parseInt(__ENV.SEED_STOCK || '100000', 10);

const chainDuration = new Trend('checkout_chain_ms', true);

export const options = {
  scenarios: {
    checkout: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 50 },
        { duration: __ENV.HOLD || '3m', target: 50 },
        { duration: '15s', target: 0 },
      ],
      gracefulRampDown: '20s',
    },
  },
  thresholds: {
    checkout_chain_ms: ['p(95)<1000'],
    http_req_failed:   ['rate<0.001'],
    checks:            ['rate>0.999'],
  },
};

export function setup() {
  const seller   = login(AUTH_URL, 'seller@example.com',   'Seller@123');
  const customer = login(AUTH_URL, 'customer@example.com', 'Customer@123');
  const productId = seedHighStockProduct(PRODUCT_URL, seller.userId, SEED_STOCK, 'phase2-checkout');
  return { token: customer.token, userId: customer.userId, productId };
}

export default function ({ token, userId, productId }) {
  const t0 = Date.now();

  const browseRes = http.get(`${PRODUCT_URL}/api/v1/products/search?q=phase2&size=10`);
  check(browseRes, { 'browse 200': (r) => r.status === 200 });

  const cartRes = http.post(
    `${CART_URL}/api/v1/cart/items`,
    JSON.stringify({ product_id: productId, quantity: 1 }),
    {
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      responseCallback: http.expectedStatuses(200, 201, 409),
    },
  );
  check(cartRes, { 'cart add ok': (r) => r.status === 200 || r.status === 201 || r.status === 409 });

  const orderRes = http.post(
    `${ORDER_URL}/api/v1/orders`,
    JSON.stringify({
      cartId: uuidv4(),
      items: [{ productId, quantity: 1 }],
      shippingAddress: { street: '1 Composite St', city: 'HCM', state: 'HCM', country: 'VN', zipCode: '70000' },
    }),
    { headers: { 'Content-Type': 'application/json', 'X-User-Id': userId } },
  );
  check(orderRes, { 'order 201': (r) => r.status === 201 });

  chainDuration.add(Date.now() - t0);
}
