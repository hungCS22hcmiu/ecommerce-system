// k6 load test — order creation (20 VUs, Kafka saga entry point)
// Run: k6 run script/k6/order_create.js
// Requires: full stack including Kafka (docker compose up -d)
//
// This test measures synchronous order-creation latency only.
// It does NOT wait for the Kafka saga (payment processing) to complete.
// Use script/loadtest-orders.sh to verify saga correctness at lower concurrency.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const BASE = __ENV.BASE_URL || 'http://localhost';

export const options = {
  stages: [
    { duration: '30s', target: 20 },   // ramp up
    { duration: '2m',  target: 20 },   // hold
    { duration: '30s', target: 0  },   // ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<2000'],  // order creation involves DB + Kafka publish
    http_req_failed:   ['rate<0.05'],
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
  const body = res.json();
  const token = body.data.access_token;
  const userId = body.data.user.id;
  if (!token || !userId) throw new Error('Login response missing token or userId');
  return { token, userId };
}

export default function ({ token }) {
  const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`,
  };

  // Each VU creates a fresh order with a unique cartId (no shared cart state)
  const payload = JSON.stringify({
    cartId: uuidv4(),
    items: [{ productId: 1, quantity: 1 }],
    shippingAddress: {
      street: '123 Test Street',
      city: 'Springfield',
      state: 'IL',
      country: 'US',
      zipCode: '62701',
    },
  });

  const res = http.post(`${BASE}/api/v1/orders`, payload, { headers });
  check(res, { 'order created 201': (r) => r.status === 201 });

  sleep(2); // longer think time — order creation is heavier than reads
}
