// Phase 2.A.4 — POST /api/v1/orders at 50 RPS for 60s.
// Target: testing_target.md §1 — P95 < 400ms, throughput ≥ 50 RPS, error rate < 0.1%.
//
// Each iteration creates one order against a fresh high-stock product seeded in setup().
// Expected to FAIL the threshold given Phase 1 finding IMP-2 (optimistic-lock retry exhaustion).

import http from 'k6/http';
import { check } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import { login, seedHighStockProduct } from './lib/auth.js';

const AUTH_URL    = __ENV.AUTH_URL    || 'http://localhost:8001';
const PRODUCT_URL = __ENV.PRODUCT_URL || 'http://localhost:8081';
const ORDER_URL   = __ENV.ORDER_URL   || 'http://localhost:8082';
const SEED_STOCK  = parseInt(__ENV.SEED_STOCK || '5000', 10);

export const options = {
  scenarios: {
    orders: {
      executor: 'constant-arrival-rate',
      rate: parseInt(__ENV.RATE || '50', 10),
      timeUnit: '1s',
      duration: __ENV.DURATION || '60s',
      preAllocatedVUs: parseInt(__ENV.PRE_VUS || '50', 10),
      maxVUs: parseInt(__ENV.MAX_VUS || '300', 10),
    },
  },
  thresholds: {
    'http_req_duration{expected_response:true}': ['p(95)<400'],
    http_req_failed: ['rate<0.001'],
    checks: ['rate>0.999'],
  },
};

export function setup() {
  const seller   = login(AUTH_URL, 'seller@example.com',   'Seller@123');
  const customer = login(AUTH_URL, 'customer@example.com', 'Customer@123');
  const productId = seedHighStockProduct(PRODUCT_URL, seller.userId, SEED_STOCK, 'phase2-orders');
  return { userId: customer.userId, productId };
}

export default function ({ userId, productId }) {
  const payload = JSON.stringify({
    cartId: uuidv4(),
    items: [{ productId, quantity: 1 }],
    shippingAddress: { street: '1 Phase2 St', city: 'HCM', state: 'HCM', country: 'VN', zipCode: '70000' },
  });
  const res = http.post(`${ORDER_URL}/api/v1/orders`, payload, {
    headers: { 'Content-Type': 'application/json', 'X-User-Id': userId },
  });
  check(res, { 'order 201': (r) => r.status === 201 });
}
