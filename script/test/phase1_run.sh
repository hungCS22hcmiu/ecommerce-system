#!/usr/bin/env bash
# Phase 1 orchestrator — Functional & Saga Correctness.
# Runs every Phase 1 test in order, captures evidence into script/k6/results/,
# then writes a Markdown table into docs/testing/test_result.md.
#
# Usage:
#   bash script/test/phase1_run.sh                    # full run
#   SKIP_REPLAY=1 bash script/test/phase1_run.sh      # skip the Go testcontainers test (slow)
#
# Compatible with bash 3.2 (macOS default).

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

RESULTS_DIR="script/k6/results"
RESULT_MD="docs/testing/test_result.md"
LOG_DIR="${RESULTS_DIR}/logs"
HAPPY_OVR="script/test/docker-compose.phase1-happy.override.yml"
FAIL_OVR="script/test/docker-compose.phase1-fail.override.yml"

mkdir -p "$RESULTS_DIR" "$LOG_DIR"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; RESET='\033[0m'

# scenario status — plain variables (bash 3.2 compatible)
ST_SAGA_HAPPY="MISSING";     NOTE_SAGA_HAPPY=""
ST_SAGA_FAIL="MISSING";      NOTE_SAGA_FAIL=""
ST_RACE="MISSING";           NOTE_RACE=""
ST_SAGA_REPLAY="MISSING";    NOTE_SAGA_REPLAY=""
ST_CORRELATION_ID="MISSING"; NOTE_CORRELATION_ID=""

section() { echo ""; echo -e "${BOLD}── $* ──${RESET}"; }
info()    { echo -e "  ${YELLOW}→${RESET} $*"; }

# Health-check the services this Phase 1 actually hits.
health_check() {
  local fail=0
  local checks=(
    "user-service|http://localhost:8001/health/live"
    "product-service|http://localhost:8081/health/live"
    "order-service|http://localhost:8082/health/live"
    "payment-service|http://localhost:8003/health/ready"
    "nginx|http://localhost/health/live"
  )
  for entry in "${checks[@]}"; do
    local name="${entry%%|*}"
    local url="${entry##*|}"
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "$url" 2>/dev/null || echo "000")
    if [[ "$code" == "200" ]]; then
      echo -e "  ${GREEN}✓${RESET} $name ($code)"
    else
      echo -e "  ${RED}✗${RESET} $name ($code) — $url"
      fail=1
    fi
  done
  return $fail
}

apply_override() {
  local ovr="$1"
  cp "$ovr" docker-compose.override.yml
  info "applying override: $ovr (restarting payment-service)"
  docker compose up -d payment-service >/dev/null
  # Wait for HTTP health check to pass
  local healthy=0
  for _ in $(seq 1 30); do
    if curl -fsS http://localhost:8003/health/ready >/dev/null 2>&1; then
      healthy=1; break
    fi
    sleep 1
  done
  if [[ "$healthy" -eq 0 ]]; then
    echo -e "${RED}payment-service did not become healthy${RESET}"
    return 1
  fi
  # Wait for consumer group to reach STABLE state (partitions assigned)
  info "payment-service healthy; waiting for Kafka consumer group to stabilize..."
  local stable=0
  for _ in $(seq 1 40); do
    local state
    state=$(docker exec ecommerce-kafka kafka-consumer-groups \
      --bootstrap-server kafka:29092 \
      --group payment-service --describe --state 2>/dev/null \
      | awk 'NR>1 && /payment-service/ {print $5; exit}')
    if [[ "$state" == "Stable" ]]; then
      stable=1; break
    fi
    sleep 1
  done
  if [[ "$stable" -eq 0 ]]; then
    info "consumer group did not reach Stable within 40s — falling back to 15s sleep"
    sleep 15
  else
    info "consumer group Stable; waiting 3s buffer"
    sleep 3
  fi
  # Wait for any existing Kafka backlog to drain (≤ 10 messages) before running
  # load tests. Phase 2 throughput tests can leave thousands of messages in
  # orders.created, causing Phase 1 saga orders to queue behind them.
  info "waiting for Kafka lag to drain (target: total lag ≤ 10)..."
  local drain=0
  for _ in $(seq 1 120); do
    local total_lag
    total_lag=$(docker exec ecommerce-kafka kafka-consumer-groups \
      --bootstrap-server kafka:29092 \
      --group payment-service --describe 2>/dev/null \
      | awk 'NR>1 && /orders.created/ {sum += $6} END {print sum+0}')
    if [[ "${total_lag:-9999}" -le 10 ]]; then
      drain=1; break
    fi
    info "  lag=${total_lag}, waiting..."
    sleep 3
  done
  if [[ "$drain" -eq 0 ]]; then
    info "Kafka lag did not drain within 6 min — proceeding anyway"
  else
    info "Kafka lag drained; proceeding"
  fi
  return 0
}

revert_override() {
  if [[ -f docker-compose.override.yml ]]; then
    rm -f docker-compose.override.yml
    info "reverting override (restarting payment-service with defaults)"
    docker compose up -d payment-service >/dev/null
    for _ in $(seq 1 30); do
      if curl -fsS http://localhost:8003/health/ready >/dev/null 2>&1; then return 0; fi
      sleep 1
    done
  fi
}

trap revert_override EXIT

# -- 0. Health gate -----------------------------------------------------------
section "0. Health gate"
if ! health_check; then
  echo -e "${RED}Stack unhealthy — aborting${RESET}"
  exit 1
fi

# -- 1. saga_happy ------------------------------------------------------------
section "1. saga_happy.js  (GATEWAY_SUCCESS_RATE=1.0)"
if apply_override "$HAPPY_OVR"; then
  if k6 run --summary-export="$RESULTS_DIR/saga_happy.json" \
       script/k6/saga_happy.js 2>&1 | tee "$LOG_DIR/saga_happy.log"; then
    ST_SAGA_HAPPY="PASS"
  else
    ST_SAGA_HAPPY="FAIL"; NOTE_SAGA_HAPPY="k6 thresholds breached or run errored"
  fi
else
  ST_SAGA_HAPPY="FAIL"; NOTE_SAGA_HAPPY="override apply failed"
fi

# -- 2. saga_fail -------------------------------------------------------------
section "2. saga_fail.js  (GATEWAY_SUCCESS_RATE=0.0)"
if apply_override "$FAIL_OVR"; then
  if k6 run --summary-export="$RESULTS_DIR/saga_fail.json" \
       script/k6/saga_fail.js 2>&1 | tee "$LOG_DIR/saga_fail.log"; then
    ST_SAGA_FAIL="PASS"
  else
    ST_SAGA_FAIL="FAIL"; NOTE_SAGA_FAIL="k6 thresholds breached or run errored"
  fi
else
  ST_SAGA_FAIL="FAIL"; NOTE_SAGA_FAIL="override apply failed"
fi

# -- 3. race_inventory --------------------------------------------------------
revert_override
section "3. race_inventory.js  (default success rate, race at order layer)"
if k6 run --summary-export="$RESULTS_DIR/race_inventory.json" \
     script/k6/race_inventory.js 2>&1 | tee "$LOG_DIR/race_inventory.log"; then
  ST_RACE="PASS"
else
  ST_RACE="FAIL"; NOTE_RACE="expected 1×201 + 9×409 not observed"
fi

# -- 4. saga replay (existing Go integration test) ----------------------------
section "4. TestDuplicateDeliveryIdempotency  (Go testcontainers)"
if [[ "${SKIP_REPLAY:-0}" == "1" ]]; then
  ST_SAGA_REPLAY="SKIPPED"; NOTE_SAGA_REPLAY="skipped via SKIP_REPLAY=1"
  info "skipped"
else
  if (cd payment-service && \
      go test -tags=integration -v -race -count=1 \
        -run TestDuplicateDeliveryIdempotency \
        ./internal/integration/...) 2>&1 | tee "$LOG_DIR/saga_replay.log"; then
    ST_SAGA_REPLAY="PASS"
  else
    ST_SAGA_REPLAY="FAIL"; NOTE_SAGA_REPLAY="go integration test failed"
  fi
fi

# -- 5. correlation id --------------------------------------------------------
section "5. test_correlation_id.sh"
if bash script/test_correlation_id.sh 2>&1 | tee "$LOG_DIR/correlation_id.log"; then
  ST_CORRELATION_ID="PASS"
else
  ST_CORRELATION_ID="FAIL"; NOTE_CORRELATION_ID="X-Correlation-ID not found in one or more service logs"
fi

# -- 6. Aggregate into test_result.md ----------------------------------------
section "6. Writing $RESULT_MD"
python3 script/test/aggregate_phase1.py \
  --results-dir "$RESULTS_DIR" \
  --log-dir "$LOG_DIR" \
  --output "$RESULT_MD" \
  --status "saga_happy=${ST_SAGA_HAPPY}" \
  --status "saga_fail=${ST_SAGA_FAIL}" \
  --status "race_inventory=${ST_RACE}" \
  --status "saga_replay=${ST_SAGA_REPLAY}" \
  --status "correlation_id=${ST_CORRELATION_ID}" || {
    echo -e "${RED}Aggregator failed${RESET}"; exit 1; }

# -- Summary ------------------------------------------------------------------
section "Summary"
FAILED=0
summarize() {
  local name="$1" status="$2" note="$3"
  case "$status" in
    PASS)    echo -e "  ${GREEN}✓${RESET} $name";;
    SKIPPED) echo -e "  ${YELLOW}—${RESET} $name (skipped)";;
    *)       echo -e "  ${RED}✗${RESET} $name  $note"; FAILED=$((FAILED+1));;
  esac
}
summarize saga_happy     "$ST_SAGA_HAPPY"     "$NOTE_SAGA_HAPPY"
summarize saga_fail      "$ST_SAGA_FAIL"      "$NOTE_SAGA_FAIL"
summarize race_inventory "$ST_RACE"           "$NOTE_RACE"
summarize saga_replay    "$ST_SAGA_REPLAY"    "$NOTE_SAGA_REPLAY"
summarize correlation_id "$ST_CORRELATION_ID" "$NOTE_CORRELATION_ID"
echo ""
if [[ "$FAILED" -gt 0 ]]; then
  echo -e "${RED}Phase 1 result: $FAILED scenario(s) failed${RESET}"
  exit 1
else
  echo -e "${GREEN}Phase 1 result: all scenarios PASS${RESET}"
fi
