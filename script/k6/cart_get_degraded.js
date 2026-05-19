// Phase 3 §7.A — Degraded GET /cart while product-service is paused.
// Target: testing_target.md §7.A — GET /cart P95 < 20ms (served from Redis only).
//
// Caller must export $TOKEN before running (the script does not log in itself,
// because chaos_cb_cart.sh pre-seeds the cart while product-service is still up).

import http from 'k6/http';
import { check } from 'k6';

const CART_URL = __ENV.CART_URL || 'http://localhost:8002';
const TOKEN    = __ENV.TOKEN;

if (!TOKEN) {
  throw new Error('TOKEN env var is required (set by chaos_cb_cart.sh after login)');
}

export const options = {
  scenarios: {
    degraded: {
      executor: 'constant-arrival-rate',
      rate: parseInt(__ENV.RATE || '10', 10),
      timeUnit: '1s',
      duration: __ENV.DURATION || '30s',
      preAllocatedVUs: 5,
      maxVUs: 30,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<20'],
    http_req_failed:   ['rate<0.01'],
    checks:            ['rate>0.99'],
  },
};

export default function () {
  const r = http.get(`${CART_URL}/api/v1/cart`,
    { headers: { Authorization: `Bearer ${TOKEN}` } });
  check(r, { 'GET /cart 200': (x) => x.status === 200 });
}
