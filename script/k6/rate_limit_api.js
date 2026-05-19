// Phase 3 §7.D — Nginx api_limit rate limit (10r/s burst=5 nodelay).
//
// Fires 50 requests as fast as possible through nginx (port 80) to /api/v1/products.
// The first ~15 (10 r/s rate + 5 burst) should pass with 200; the rest should be 429.
// Aggregator asserts ≥ 35 of 50 are 429.

import http from 'k6/http';
import { Counter } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost';

const status200 = new Counter('status_200');
const status429 = new Counter('status_429');
const statusOther = new Counter('status_other');

export const options = {
  scenarios: {
    burst: {
      executor: 'shared-iterations',
      vus: 50,
      iterations: 50,
      maxDuration: '10s',
      startTime: '0s',
    },
  },
  // Thresholds intentionally lenient — aggregator decides PASS/FAIL from counters.
  thresholds: {},
};

export default function () {
  const res = http.get(`${BASE}/api/v1/products?size=5`,
    { responseCallback: http.expectedStatuses(200, 429) });

  if (res.status === 200)      status200.add(1);
  else if (res.status === 429) status429.add(1);
  else                          statusOther.add(1);
}
