// Phase 2.A.2 — POST /api/v1/cart/items at 500 RPS for 60s.
// Target: testing_target.md §1 — P95 < 40ms, throughput ≥ 500 RPS.
// 409 = Redis WATCH/MULTI/EXEC conflict (cart uses optimistic concurrency); marked expected.

import http from 'k6/http';
import { check } from 'k6';
import { login } from './lib/auth.js';

const AUTH_URL = __ENV.AUTH_URL || 'http://localhost:8001';
const CART_URL = __ENV.CART_URL || 'http://localhost:8002';
const PRODUCT_ID = parseInt(__ENV.PRODUCT_ID || '1', 10);

export const options = {
  scenarios: {
    cart: {
      executor: 'constant-arrival-rate',
      rate: parseInt(__ENV.RATE || '500', 10),
      timeUnit: '1s',
      duration: __ENV.DURATION || '60s',
      preAllocatedVUs: parseInt(__ENV.PRE_VUS || '80', 10),
      maxVUs: parseInt(__ENV.MAX_VUS || '400', 10),
    },
  },
  thresholds: {
    'http_req_duration{expected_response:true}': ['p(95)<40'],
    http_req_failed: ['rate<0.05'],
    checks: ['rate>0.95'],
  },
};

export function setup() {
  return login(AUTH_URL, 'customer@example.com', 'Customer@123');
}

export default function ({ token }) {
  const res = http.post(
    `${CART_URL}/api/v1/cart/items`,
    JSON.stringify({ product_id: PRODUCT_ID, quantity: 1 }),
    {
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      responseCallback: http.expectedStatuses(200, 201, 409),
    },
  );
  check(res, { 'cart add 2xx or 409': (r) => r.status === 200 || r.status === 201 || r.status === 409 });
}
