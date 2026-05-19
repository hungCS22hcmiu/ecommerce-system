// Phase 2.A.3 — GET /api/v1/products/search at 150 RPS for 60s.
// Target: testing_target.md §1 — P95 < 150ms, throughput ≥ 150 RPS.
// Product-service caches keyword search 3 minutes — a warmup pass primes the cache so
// the measured run reflects steady-state hit-rate, not cold-cache cost.

import http from 'k6/http';
import { check } from 'k6';

const PRODUCT_URL = __ENV.PRODUCT_URL || 'http://localhost:8081';

const QUERIES = ['shoes', 'shirt', 'phone', 'laptop', 'watch', 'bag', 'book', 'lamp'];

export const options = {
  scenarios: {
    browse: {
      executor: 'constant-arrival-rate',
      rate: parseInt(__ENV.RATE || '150', 10),
      timeUnit: '1s',
      duration: __ENV.DURATION || '60s',
      preAllocatedVUs: parseInt(__ENV.PRE_VUS || '30', 10),
      maxVUs: parseInt(__ENV.MAX_VUS || '200', 10),
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<150'],
    http_req_failed:   ['rate<0.01'],
    checks:            ['rate>0.99'],
  },
};

export function setup() {
  for (const q of QUERIES) {
    http.get(`${PRODUCT_URL}/api/v1/products/search?q=${encodeURIComponent(q)}&size=20`);
  }
}

export default function () {
  const q = QUERIES[Math.floor(Math.random() * QUERIES.length)];
  const res = http.get(`${PRODUCT_URL}/api/v1/products/search?q=${encodeURIComponent(q)}&size=20`);
  check(res, { 'search 200': (r) => r.status === 200 });
}
