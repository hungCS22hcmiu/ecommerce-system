// k6 load test — cart mutations (50 VUs, shared user token)
// Run: k6 run script/k6/cart_ops.js
// Requires: stack running with seeded users (script/sample_users.sql)
//
// Expected behaviour: 409 responses are normal under concurrent load — the
// cart uses Redis WATCH/MULTI/EXEC (optimistic locking). They are marked as
// expected statuses so they don't inflate http_req_failed.
import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE     = __ENV.BASE_URL  || 'http://localhost';
const AUTH_URL = __ENV.AUTH_URL  || BASE;  // user-service; override when bypassing Nginx

export const options = {
  stages: [
    { duration: '30s', target: 50 },   // ramp up
    { duration: '90s', target: 50 },   // hold
    { duration: '30s', target: 0  },   // ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<1000'],
    http_req_failed:   ['rate<0.05'],  // 409 marked expected below; only real errors counted
  },
};

export function setup() {
  const res = http.post(
    `${AUTH_URL}/api/v1/auth/login`,
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

  // Add item to cart — 409 marked as expected so it doesn't inflate http_req_failed
  const addRes = http.post(
    `${BASE}/api/v1/cart/items`,
    JSON.stringify({ product_id: 1, quantity: 1 }),
    { headers, responseCallback: http.expectedStatuses(200, 201, 409) },
  );
  // 200/201 = success, 409 = Redis WATCH conflict (concurrent update, expected)
  check(addRes, { 'add item ok': (r) => r.status === 200 || r.status === 201 || r.status === 409 });

  // Read the cart back
  const getRes = http.get(`${BASE}/api/v1/cart`, { headers });
  check(getRes, { 'get cart 200': (r) => r.status === 200 });

  sleep(1);
}
