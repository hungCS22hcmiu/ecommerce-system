// k6 load test — product browsing (read-heavy, no auth)
// Run: k6 run script/k6/product_browse.js
// Override base URL: k6 run --env BASE_URL=http://your-host script/k6/product_browse.js
import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE = __ENV.BASE_URL || 'http://localhost';

export const options = {
  stages: [
    { duration: '30s', target: 100 },  // ramp up to 100 VUs
    { duration: '2m',  target: 100 },  // hold steady
    { duration: '30s', target: 0   },  // ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'],  // 95th percentile under 500ms
    http_req_failed:   ['rate<0.01'],  // error rate under 1%
  },
};

export default function () {
  const r = Math.random();

  if (r < 0.5) {
    // 50% — product list (exercises Redis productList cache)
    const res = http.get(`${BASE}/api/v1/products`);
    check(res, { 'list 200': (r) => r.status === 200 });

  } else if (r < 0.8) {
    // 30% — single product (exercises Redis product cache)
    const id = Math.ceil(Math.random() * 10);
    const res = http.get(`${BASE}/api/v1/products/${id}`);
    // 404 is valid if the product doesn't exist in the test dataset
    check(res, { 'detail 2xx/404': (r) => r.status === 200 || r.status === 404 });

  } else {
    // 20% — keyword search (exercises full-text + Redis productList cache)
    const queries = ['widget', 'laptop', 'phone', 'shoes'];
    const q = queries[Math.floor(Math.random() * queries.length)];
    const res = http.get(`${BASE}/api/v1/products?q=${q}`);
    check(res, { 'search 200': (r) => r.status === 200 });
  }

  sleep(1); // realistic think time between page views
}
