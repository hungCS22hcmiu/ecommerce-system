// Phase 2.A.1 — POST /api/v1/auth/login at 100 RPS for 60s.
// Bypasses nginx auth_limit (5/min) by hitting user-service directly on :8001.
// Target: testing_target.md §1 — P95 < 300ms, throughput ≥ 100 RPS, error rate < 1%.

import http from 'k6/http';
import { check } from 'k6';

const AUTH_URL = __ENV.AUTH_URL || 'http://localhost:8001';
const EMAIL    = __ENV.EMAIL    || 'customer@example.com';
const PASSWORD = __ENV.PASSWORD || 'Customer@123';

export const options = {
  scenarios: {
    login: {
      executor: 'constant-arrival-rate',
      rate: parseInt(__ENV.RATE || '100', 10),
      timeUnit: '1s',
      duration: __ENV.DURATION || '60s',
      preAllocatedVUs: parseInt(__ENV.PRE_VUS || '30', 10),
      maxVUs: parseInt(__ENV.MAX_VUS || '200', 10),
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<300'],
    http_req_failed:   ['rate<0.01'],
    checks:            ['rate>0.99'],
  },
};

export default function () {
  const res = http.post(
    `${AUTH_URL}/api/v1/auth/login`,
    JSON.stringify({ email: EMAIL, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  check(res, {
    'login 200': (r) => r.status === 200,
    'has token': (r) => r.status === 200 && !!r.json('data.access_token'),
  });
}
