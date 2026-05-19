# Phase 2 — Load & Throughput: Implementation Plan

> Companion to [`testing_plan.md`](./testing_plan.md) §Phase 2. Defines the concrete artifacts that will produce Phase 2 evidence in [`test_result.md`](./test_result.md).

## Context

Phase 2 verifies four target sections from `testing_target.md`: **§1 Service performance**, **§3.A 50 VU composite**, **§4 AI semantic search**, **§5 Infrastructure**, **§6 Connection pooling**.

Today the repo has 3 starter k6 scripts (`cart_ops.js`, `order_create.js`, `product_browse.js`) using ramp-up profiles, plus `perf-baseline.sh` (single-threaded). No AI search load test, no infra monitors, no per-layer AI timing, no orchestrator.

Phase 1 surfaced 3 perf gaps that will **reappear and worsen** here: saga TTC blown at 10 VU (IMP-1), order 409 INSUFFICIENT_STOCK at sustained ~5 r/s (IMP-2), correlation-id loss order→payment (IMP-4). Phase 2 will quantify each §1 target gap against current code and feed concrete numbers into the improvement plan.

**Decisions confirmed:**
- Run Phase 2 against current code; do not pre-fix Phase 1 findings.
- AI per-layer timing: **log-based** (`System.nanoTime` + INFO log line) — aggregator greps logs. ~15 LOC change in `AISearchServiceImpl`.
- Use `constant-arrival-rate` k6 executor for rate-based targets (RPS) — back-pressure surfaces as queue growth.

---

## Targets → Artifacts

### §1 Service performance (Phase 2.A)

| Endpoint | Threshold | Artifact (NEW / REWRITE) |
|---|---|---|
| POST /auth/login P95<300ms @ 100 RPS | `script/k6/auth_login.js` (NEW) — bypasses nginx auth_limit by hitting `localhost:8001` |
| POST /cart/items P95<40ms @ 500 RPS | `script/k6/cart_ops.js` (REWRITE) — switch from ramp profile to `constant-arrival-rate`, threshold `p(95)<40` |
| GET /products/search P95<150ms @ 150 RPS | `script/k6/product_browse.js` (REWRITE) — keyword search only, cache warmup pre-run |
| POST /orders P95<400ms @ 50 RPS | `script/k6/order_create.js` (REWRITE) — seed high-stock product in setup() (Phase 1 pattern), `constant-arrival-rate` 50/s |
| Kafka consumer 200 msg/s, P95<100ms | `script/test/payment_kafka_throughput.sh` (NEW) — produces 10k `orders.created` via `docker exec kafka kafka-console-producer`, parses payment-service logs for handler latency |

### §3.A 50 VU composite checkout

| Threshold | Artifact |
|---|---|
| 50 VUs, full checkout (login→browse→cart→order), err<0.1%, P95<1s | `script/k6/checkout_50vu.js` (NEW) — `ramping-vus` to 50, default() runs the full chain, threshold on `checks rate>0.999` + `http_req_duration p(95)<1000` |

### §4 AI semantic search (Phase 2.B)

| Layer | Threshold | Artifact / instrumentation |
|---|---|---|
| Total /ai-search P95 | <250ms | `script/k6/ai_search.js` (NEW) — rotating query corpus, `constant-arrival-rate` 20/s |
| Embedding | <100ms | **NEW instrumentation** in `AISearchServiceImpl.java`: wrap `embeddingClient.embed()` with nanoTime + log `INFO ai.search.layer embed_ms=...` |
| Vector search | <50ms | same: wrap `productRepository.findIdsBySemanticSimilarity*` with `vector_ms=...` |
| Re-ranking | <30ms | same: wrap the rerank loop (lines 63–75) with `rerank_ms=...` |
| Throughput ≥20 RPS | covered by k6 rate metric | same script |
| Cold start <15s | `script/test/test_ai_cold_start.sh` (NEW) — `docker compose restart ai-service`, poll `/health/ready` from inside the network every 250ms |
| Memory ≤1.5GB | `script/test/monitor_ai_mem.sh` (NEW) — `docker stats ai-service` sampled 1Hz during AI load |
| Searchability lag P95 <1.0s | `product-service/src/test/java/.../integration/AISearchabilityLagIT.java` (NEW) — Testcontainers PG+Redis, POST product with unique uuid keyword, poll `/ai-search?q=<uuid-tail>` until first hit, × 30 iterations |

### §5 Infrastructure + §6 Connection pooling (Phase 2.D sidecar monitors)

Run alongside each Phase 2.A scenario.

| Monitor | Target | Artifact |
|---|---|---|
| Postgres conns per DB and global | per-DB ≤25 (Go) / ≤20 (Java), total ≤150 | `script/test/monitor_pg_connections.sh` (NEW) — `docker exec postgres psql -c "SELECT datname, count(*) FROM pg_stat_activity GROUP BY datname"` every 1s into CSV |
| HikariCP leak detection | zero leak warnings | `script/test/monitor_hikari.sh` (NEW) — grep `ConnectionLeakDetectionThreshold` warnings |
| Redis latency baseline <0.2ms, P99 acquisition <1ms | sampled during cart_ops | `script/test/monitor_redis.sh` (NEW) — `docker exec redis redis-cli --latency -i 1 -t 60` |
| Kafka lag <50 during peak | during order_create + kafka_throughput | `script/test/monitor_kafka_lag.sh` (NEW) — `kafka-consumer-groups --describe --group payment-service` every 5s |

### Orchestrator + reporting

| Artifact | Role |
|---|---|
| `script/test/phase2_run.sh` (NEW) | Health-gate → run each scenario with sidecar monitors backgrounded → aggregate |
| `script/test/aggregate_phase2.py` (NEW) | Parse k6 JSON, monitor CSVs, AI log greps → append Phase 2 section to `docs/testing/test_result.md` |
| `docker-compose.phase2.override.yml` (NEW) | Adds memory limit 1.5G to ai-service + enables its healthcheck. Applied for Phase 2.B only |

---

## Concrete Artifact Specs

### A. Common k6 shape (constant-arrival-rate)

```javascript
const AUTH_URL    = __ENV.AUTH_URL    || 'http://localhost:8001';
const PRODUCT_URL = __ENV.PRODUCT_URL || 'http://localhost:8081';
const ORDER_URL   = __ENV.ORDER_URL   || 'http://localhost:8082';
const CART_URL    = __ENV.CART_URL    || 'http://localhost:8002';

export const options = {
  scenarios: {
    main: {
      executor: 'constant-arrival-rate',
      rate: parseInt(__ENV.RATE || '100', 10),
      timeUnit: '1s',
      duration: __ENV.DURATION || '60s',
      preAllocatedVUs: parseInt(__ENV.PRE_VUS || '50', 10),
      maxVUs: parseInt(__ENV.MAX_VUS || '200', 10),
    },
  },
  thresholds: { /* per-script */ },
};
```

### B. Login/seed helper (`script/k6/lib/auth.js`)

Extract the `login()` and seller-product-seed pattern from Phase 1's `saga_happy.js` so all Phase 2 scripts share it. Phase 1 scripts won't be retrofitted in this plan.

### C. AI per-layer logging — the one production code change

`product-service/src/main/java/.../service/serviceImpl/AISearchServiceImpl.java`:

```java
long t0 = System.nanoTime();
float[] vector = embeddingClient.embed(query, query, limit);
long tEmbed = System.nanoTime();

List<Object[]> rows = sellerId != null ? ... : ...;
long tVector = System.nanoTime();

// existing rerank block ...
long tRerank = System.nanoTime();

log.info("ai.search.layer query='{}' embed_ms={} vector_ms={} rerank_ms={} results={}",
    query,
    (tEmbed - t0) / 1_000_000,
    (tVector - tEmbed) / 1_000_000,
    (tRerank - tVector) / 1_000_000,
    page.size());
```

Aggregator greps `ai.search.layer ` lines in `docker compose logs product-service`, regex-extracts ms values, computes P95 per layer.

### D. AISearchabilityLagIT

`product-service/src/test/java/.../integration/AISearchabilityLagIT.java`:

```java
@Test void posting_a_product_makes_it_findable_within_1s_p95() {
  List<Long> lags = new ArrayList<>();
  for (int i = 0; i < 30; i++) {
    String tag = "lagtest-" + UUID.randomUUID().toString().substring(0, 8);
    long t0 = System.currentTimeMillis();
    restTemplate.postForEntity("/api/v1/products", payload(tag), ...);
    await().atMost(5, SECONDS).pollInterval(50, MILLIS).untilAsserted(() -> {
      var resp = restTemplate.getForEntity("/api/v1/products/ai-search?q=" + tag, ...);
      assertThat(resp.getBody().getData().results()).isNotEmpty();
    });
    lags.add(System.currentTimeMillis() - t0);
  }
  assertThat(percentile(lags, 95)).isLessThan(1000);
}
```

Reuses pgvector + Redis testcontainers from `AISearchIntegrationTest.java`. Awaitility is already in pom.xml.

### E. payment_kafka_throughput.sh skeleton

```bash
NUM=${NUM:-10000}
python3 script/test/gen_kafka_payloads.py $NUM > /tmp/payloads.jsonl
t0=$(date +%s%N)
docker exec -i ecommerce-kafka kafka-console-producer \
  --bootstrap-server kafka:29092 --topic orders.created \
  --producer-property compression.type=snappy < /tmp/payloads.jsonl
# wait for lag = 0
while true; do
  lag=$(docker compose exec kafka kafka-consumer-groups --bootstrap-server kafka:29092 \
    --describe --group payment-service | awk 'NR>1 {sum+=$6} END {print sum+0}')
  [[ "$lag" == "0" ]] && break
  sleep 1
done
t1=$(date +%s%N)
echo "throughput=$(( NUM * 1000000000 / (t1 - t0) )) msg/s"
```

Per-message P95 latency: grep payment-service JSON logs for `"kafka.worker: processed"` lines, compute consumed_at - produced_at from Kafka record timestamps.

### F. Compose override for AI tests

`docker-compose.phase2.override.yml`:

```yaml
services:
  ai-service:
    deploy:
      resources:
        limits:
          memory: 1.5G
    healthcheck:
      test: ["CMD-SHELL", "python -c \"import urllib.request,sys;sys.exit(0 if urllib.request.urlopen('http://localhost:9000/health/ready').status==200 else 1)\""]
      interval: 5s
      timeout: 3s
      retries: 5
      start_period: 30s
```

Applied only for Phase 2.B; reverted afterward.

### G. phase2_run.sh flow

```
0. Health gate (direct-port checks)
1. 2.A.1 auth_login           — bypass nginx, 100 RPS × 60s
2. 2.A.2 cart_ops             — 500 RPS × 60s, with pg_conn + redis monitors backgrounded
3. 2.A.3 product_browse       — 150 RPS × 60s, with pg_conn monitor
4. 2.A.4 order_create         — 50 RPS × 60s, with pg_conn + kafka_lag monitors
5. 2.A.5 payment_kafka_throughput — 10k events, capture throughput + lag
6. apply phase2 override, restart ai-service
7. 2.B.1 ai_cold_start         — restart ai-service, time /health/ready
8. 2.B.2 ai_search load        — 20 RPS × 60s with ai_mem monitor + product-service log capture
9. 2.B.3 ai_searchability_lag  — mvn -Dtest=AISearchabilityLagIT verify
10. revert override
11. 2.C  checkout_50vu         — composite chain
12. aggregate → docs/testing/test_result.md (append, not replace)
13. summary + exit code
```

Each scenario writes `script/k6/results/<name>.json`; each monitor writes `script/k6/results/monitors/<name>.csv`.

### H. aggregate_phase2.py

Mirrors `aggregate_phase1.py`. Two new helpers:
- `parse_ai_layer_logs(log_path)` — greps `ai.search.layer` lines, regex-extracts ms values, returns `{embed_p95, vector_p95, rerank_p95}`.
- `parse_pg_csv(csv_path)` — reads sidecar CSV, returns `{max_per_db, max_global, breach_count}`.

---

## Critical Files

**Production code change (1 file):**
- `product-service/src/main/java/com/ecommerce/product_service/service/serviceImpl/AISearchServiceImpl.java`

**Compose change (1 new file):**
- `docker-compose.phase2.override.yml`

**New k6 scripts:**
- `script/k6/lib/auth.js`
- `script/k6/auth_login.js`
- `script/k6/ai_search.js`
- `script/k6/checkout_50vu.js`

**Rewritten k6 scripts (constant-arrival-rate):**
- `script/k6/cart_ops.js`
- `script/k6/product_browse.js`
- `script/k6/order_create.js`

**New bash scripts:**
- `script/test/payment_kafka_throughput.sh`
- `script/test/test_ai_cold_start.sh`
- `script/test/monitor_pg_connections.sh`
- `script/test/monitor_redis.sh`
- `script/test/monitor_kafka_lag.sh`
- `script/test/monitor_ai_mem.sh`
- `script/test/monitor_hikari.sh`
- `script/test/phase2_run.sh`

**New Python:**
- `script/test/aggregate_phase2.py`
- `script/test/gen_kafka_payloads.py`

**New Java integration test:**
- `product-service/src/test/java/.../integration/AISearchabilityLagIT.java`

**Reused (no changes):**
- Login/seed pattern from `script/k6/saga_happy.js`
- Health-check shape from `script/test/phase1_run.sh`
- pgvector Testcontainers scaffold from `AISearchIntegrationTest.java`
- `docker compose exec kafka kafka-console-producer` (used in `loadtest-orders.sh`)

---

## Verification

```bash
docker compose up -d                            # full stack
docker compose build product-service            # rebuild with AI layer logging
docker compose up -d product-service
bash script/test/phase2_run.sh                  # ~10–15 min full run
cat docs/testing/test_result.md                 # Phase 2 section appended
ls script/k6/results/                           # 8+ k6 JSON summaries
ls script/k6/results/monitors/                  # CSV sidecar evidence
```

Per-target independent runs:

| Target | Command |
|---|---|
| §1 login    | `RATE=100 DURATION=60s k6 run --summary-export=/tmp/auth.json script/k6/auth_login.js` |
| §1 cart     | `RATE=500 DURATION=60s k6 run script/k6/cart_ops.js` |
| §1 products | `RATE=150 DURATION=60s k6 run script/k6/product_browse.js` |
| §1 orders   | `RATE=50  DURATION=60s k6 run script/k6/order_create.js` |
| §1 kafka    | `bash script/test/payment_kafka_throughput.sh` |
| §3.A 50 VU  | `k6 run script/k6/checkout_50vu.js` |
| §4 ai search| `RATE=20 k6 run script/k6/ai_search.js && python3 script/test/aggregate_phase2.py --only ai-layers` |
| §4 cold start| `bash script/test/test_ai_cold_start.sh` |
| §4 lag      | `(cd product-service && mvn -Dtest=AISearchabilityLagIT verify)` |
| §5/§6 PG    | `bash script/test/monitor_pg_connections.sh & PID=$!; k6 run script/k6/order_create.js; kill $PID; cat script/k6/results/monitors/pg.csv` |

**Acceptance:** Phase 2 is executed once `test_result.md` contains observed values for every row in §1, §3.A, §4, §5, §6. Each FAIL or AT-RISK row drops into `improvement performance plan.md` with file/line root-cause pointers.

**Expected failures (so we're not surprised):**
- §1 orders @ 50 RPS — Phase 1 already showed ~5 r/s ceiling (IMP-2).
- §3.A 50 VU composite — IMP-1 saga TTC compounds at 50 VUs.
- §4 cold start on first run — model load + container start on M1; subsequent runs are warmer.
- §5 Kafka lag during throughput test — will spike well above 50.

These still count as executed — they fill the result table with concrete numbers.

---

## Out of Scope

- Fixing Phase 1 findings (IMP-1..5).
- Adding actuator/Prometheus — log-based timing is sufficient.
- Rate-limit tests (§7.D) — those are Phase 3.
- CI integration.
