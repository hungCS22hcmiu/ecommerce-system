// Phase 1 — Payment-failure saga TTC + compensation TTC
// Run with payment-service started under docker-compose.phase1-fail.override.yml
// so GATEWAY_SUCCESS_RATE=0.0 (every payment is declined).
//
// Measures two metrics per iteration:
//   saga_ttc_ms          — POST /orders → payment.status == FAILED            (target P95 < 1500ms)
//   compensation_ttc_ms  — POST /orders → order.status == CANCELLED           (target P95 < 2000ms)
//                           (order-service sets CANCELLED right before firing releaseStock,
//                            so this is the closest observable proxy for "stock restored")
//
// Output: --summary-export=script/k6/results/saga_fail.json

import http from 'k6/http';
import { check, fail } from 'k6';
import { Trend } from 'k6/metrics';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const AUTH_URL    = __ENV.AUTH_URL    || 'http://localhost:8001';
const PRODUCT_URL = __ENV.PRODUCT_URL || 'http://localhost:8081';
const ORDER_URL   = __ENV.ORDER_URL   || 'http://localhost:8082';
const PAYMENT_URL = __ENV.PAYMENT_URL || 'http://localhost:8003';
const SEED_STOCK = parseInt(__ENV.SEED_STOCK || '500', 10);
const POLL_MS = 50;
const POLL_TIMEOUT_MS = 8000;

const sagaTTC = new Trend('saga_ttc_ms', true);
const compTTC = new Trend('compensation_ttc_ms', true);

export const options = {
  scenarios: {
    fail: {
      executor: 'shared-iterations',
      vus: parseInt(__ENV.VUS || '10', 10),
      iterations: parseInt(__ENV.ITERATIONS || '200', 10),
      maxDuration: '5m',
    },
  },
  thresholds: {
    saga_ttc_ms: ['p(95)<1500'],
    compensation_ttc_ms: ['p(95)<2000'],
    checks: ['rate==1.0'],
  },
};

function login(email, password) {
  const res = http.post(
    `${AUTH_URL}/api/v1/auth/login`,
    JSON.stringify({ email, password }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  if (res.status !== 200) throw new Error(`Login ${email}: ${res.status} ${res.body}`);
  const body = res.json();
  return { token: body.data.access_token, userId: body.data.user.id };
}

export function setup() {
  const seller = login('seller@example.com', 'Seller@123');
  const customer = login('customer@example.com', 'Customer@123');

  const productPayload = JSON.stringify({
    name: `saga-fail-${uuidv4()}`,
    description: 'Phase 1 saga_fail test product (auto-seeded, high stock)',
    price: 9.99,
    categoryId: 1,
    stockQuantity: SEED_STOCK,
  });
  const createRes = http.post(`${PRODUCT_URL}/api/v1/products`, productPayload, {
    headers: { 'Content-Type': 'application/json', 'X-Seller-Id': seller.userId },
  });
  if (createRes.status !== 201) {
    throw new Error(`Product create failed: ${createRes.status} ${createRes.body}`);
  }
  return {
    token: customer.token,
    userId: customer.userId,
    productId: createRes.json('data.id'),
  };
}

function busyWait(ms) {
  const target = Date.now() + ms;
  while (Date.now() < target) { /* spin */ }
}

export default function ({ token, userId, productId }) {
  const orderHeaders = { 'Content-Type': 'application/json', 'X-User-Id': userId };
  const orderPayload = JSON.stringify({
    cartId: uuidv4(),
    items: [{ productId: productId, quantity: 1 }],
    shippingAddress: {
      street: '1 Saga Fail St',
      city: 'Ho Chi Minh City',
      state: 'HCM',
      country: 'Vietnam',
      zipCode: '70000',
    },
  });

  const t0 = Date.now();
  const orderRes = http.post(`${ORDER_URL}/api/v1/orders`, orderPayload, { headers: orderHeaders });
  if (!check(orderRes, { 'order created 201': (r) => r.status === 201 })) {
    fail(`order create failed: ${orderRes.status} ${orderRes.body}`);
  }
  const orderId = orderRes.json('data.id');

  const pollHeaders = { 'Authorization': `Bearer ${token}`, 'X-User-Id': userId };

  let sagaTtcMs = -1;
  let compTtcMs = -1;
  let paymentStatus = '';
  let orderStatus = '';

  while (Date.now() - t0 < POLL_TIMEOUT_MS && (sagaTtcMs < 0 || compTtcMs < 0)) {
    if (sagaTtcMs < 0) {
      const pr = http.get(`${PAYMENT_URL}/api/v1/payments/order/${orderId}`, { headers: pollHeaders });
      if (pr.status === 200) {
        paymentStatus = pr.json('data.status');
        if (paymentStatus === 'FAILED' || paymentStatus === 'COMPLETED') {
          sagaTtcMs = Date.now() - t0;
        }
      }
    }
    if (compTtcMs < 0) {
      const or = http.get(`${ORDER_URL}/api/v1/orders/${orderId}`, { headers: pollHeaders });
      if (or.status === 200) {
        orderStatus = or.json('data.status');
        if (orderStatus === 'CANCELLED' || orderStatus === 'CONFIRMED') {
          compTtcMs = Date.now() - t0;
        }
      }
    }
    busyWait(POLL_MS);
  }

  if (sagaTtcMs >= 0) sagaTTC.add(sagaTtcMs);
  if (compTtcMs >= 0) compTTC.add(compTtcMs);

  check(null, {
    'payment terminal FAILED': () => paymentStatus === 'FAILED',
    'order terminal CANCELLED': () => orderStatus === 'CANCELLED',
  });
}
