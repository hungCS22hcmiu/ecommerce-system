// Phase 3 §7.D — Nginx auth_limit rate limit (5r/min burst=3 nodelay).
//
// Fires 9 sequential POST /api/v1/auth/login through nginx (port 80).
// With burst=3, the first 8 should pass auth_limit (5 + 3) and the 9th should be 429.
// Aggregator asserts ≥ 1 of 9 is 429.

import http from 'k6/http';
import { Counter } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost';

const status200 = new Counter('status_200');
const status401 = new Counter('status_401');
const status429 = new Counter('status_429');
const statusOther = new Counter('status_other');

export const options = {
  scenarios: {
    burst: {
      executor: 'shared-iterations',
      vus: 1,            // sequential — same TCP/IP source key
      iterations: 9,
      maxDuration: '10s',
    },
  },
  thresholds: {},
};

export default function () {
  const res = http.post(
    `${BASE}/api/v1/auth/login`,
    JSON.stringify({ email: 'customer@example.com', password: 'Customer@123' }),
    {
      headers: { 'Content-Type': 'application/json' },
      responseCallback: http.expectedStatuses(200, 401, 429),
    },
  );
  if (res.status === 200)      status200.add(1);
  else if (res.status === 401) status401.add(1);
  else if (res.status === 429) status429.add(1);
  else                          statusOther.add(1);
}
