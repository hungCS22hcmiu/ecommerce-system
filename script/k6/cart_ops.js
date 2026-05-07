// k6 load test — cart mutations (50 VUs, shared user token)
// Run: k6 run script/k6/cart_ops.js
// Requires: stack running with seeded users (script/sample_users.sql)
//
// Expected behaviour: 409 responses are normal under concurrent load — the
// cart uses Redis WATCH/MULTI/EXEC (optimistic locking). The failure threshold
// only counts 5xx errors, not 4xx conflicts.
import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE = __ENV.BASE_URL || 'http://localhost';

export const options = {
  stages: [
    { duration: '30s', target: 50 },   // ramp up
    { duration: '90s', target: 50 },   // hold
    { duration: '30s', target: 0  },   // ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<1000'],
    // 409 (Redis WATCH conflict) is expected; only 5xx counts as a failure
    'http_req_failed{expected_response:false}': ['rate<0.05'],
  },
};

export function setup() {
  const res = http.post(
    `${BASE}/api/v1/auth/login`,
    JSON.stringify({ email: 'customer@example.com', password: 'Customer@123' }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  if (res.status !== 200) {
    throw new Error(`Login failed: ${res.status} ${res.body}`);
  }
  const token = res.json('data.access_token');
  if (!token) throw new Error('No access_token in login response');
  return { token };
}

export default function ({ token }) {
  const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`,
  };

  // Add item to cart
  const addRes = http.post(
    `${BASE}/api/v1/cart/items`,
    JSON.stringify({ product_id: 1, quantity: 1 }),
    { headers },
  );
  // 200/201 = success, 409 = Redis WATCH conflict (concurrent update, expected)
  check(addRes, { 'add item ok': (r) => r.status === 200 || r.status === 201 || r.status === 409 });

  // Read the cart back
  const getRes = http.get(`${BASE}/api/v1/cart`, { headers });
  check(getRes, { 'get cart 200': (r) => r.status === 200 });

  sleep(1);
}
