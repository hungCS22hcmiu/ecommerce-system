// Phase 2.B — GET /api/v1/products/ai-search at 20 RPS for 60s.
// Target: testing_target.md §4 — total P95 < 250ms, throughput ≥ 20 RPS.
// Per-layer P95 (embed/vector/rerank) is captured by aggregator from product-service logs
// (AISearchServiceImpl now logs `ai.search.layer embed_ms=... vector_ms=... rerank_ms=...`).

import http from 'k6/http';
import { check } from 'k6';

const PRODUCT_URL = __ENV.PRODUCT_URL || 'http://localhost:8081';

// Rotated query corpus — varied enough to defeat the @Cacheable result cache.
const QUERIES = [
  'comfortable running shoes',
  'gaming laptop with rgb',
  'wireless noise-cancelling headphones',
  'modern minimalist watch',
  'leather messenger bag for work',
  'mechanical keyboard cherry mx',
  'bedside reading lamp warm light',
  'smartphone with great camera',
  'kids cotton t-shirt',
  'water-resistant hiking jacket',
];

export const options = {
  scenarios: {
    ai: {
      executor: 'constant-arrival-rate',
      rate: parseInt(__ENV.RATE || '20', 10),
      timeUnit: '1s',
      duration: __ENV.DURATION || '60s',
      preAllocatedVUs: parseInt(__ENV.PRE_VUS || '20', 10),
      maxVUs: parseInt(__ENV.MAX_VUS || '100', 10),
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<250'],
    http_req_failed:   ['rate<0.02'],
    checks:            ['rate>0.98'],
  },
};

export default function () {
  const q = QUERIES[__VU + __ITER % QUERIES.length] || QUERIES[0];
  const res = http.get(
    `${PRODUCT_URL}/api/v1/products/ai-search?q=${encodeURIComponent(q)}&limit=10`,
  );
  check(res, {
    'ai-search 200': (r) => r.status === 200,
    'has results array': (r) => r.status === 200 && Array.isArray(r.json('data.results')),
  });
}
