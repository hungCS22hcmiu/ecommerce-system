#!/usr/bin/env bash
# Phase 2 orchestrator — Load & Throughput.
# Runs every scenario with sidecar monitors backgrounded, then aggregates.
# Bash 3.2 compatible.
#
# Usage:
#   bash script/test/phase2_run.sh                           # full run
#   SKIP_AI_LAG=1 bash script/test/phase2_run.sh             # skip the Java IT
#   SKIP_KAFKA_THROUGHPUT=1 bash script/test/phase2_run.sh   # skip kafka producer test

set -uo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

RESULTS_DIR="script/k6/results"
MONITORS_DIR="${RESULTS_DIR}/monitors"
LOG_DIR="${RESULTS_DIR}/logs"
mkdir -p "$RESULTS_DIR" "$MONITORS_DIR" "$LOG_DIR"

PHASE2_OVR="script/test/docker-compose.phase2.override.yml"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; RESET='\033[0m'
section() { echo ""; echo -e "${BOLD}── $* ──${RESET}"; }
info()    { echo -e "  ${YELLOW}→${RESET} $*"; }

# Per-scenario PASS/FAIL/SKIPPED
ST_AUTH="MISSING"; ST_CART="MISSING"; ST_BROWSE="MISSING"; ST_ORDER="MISSING"
ST_KAFKA="MISSING"; ST_CHECKOUT="MISSING"; ST_AI="MISSING"; ST_COLDSTART="MISSING"; ST_LAG="MISSING"

health_check() {
  local fail=0
  for entry in "user-service|http://localhost:8001/health/live" \
               "product-service|http://localhost:8081/health/live" \
               "cart-service|http://localhost:8002/health/live" \
               "order-service|http://localhost:8082/health/live" \
               "payment-service|http://localhost:8003/health/ready"; do
    name="${entry%%|*}"; url="${entry##*|}"
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "$url" 2>/dev/null || echo "000")
    if [[ "$code" == "200" ]]; then echo -e "  ${GREEN}✓${RESET} $name"; else echo -e "  ${RED}✗${RESET} $name ($code)"; fail=1; fi
  done
  return $fail
}

start_monitor() {
  # start_monitor NAME ARGS...   →  echoes pid
  local name="$1"; shift
  local out="$LOG_DIR/monitor_${name}.log"
  "$@" >"$out" 2>&1 &
  echo $!
}

stop_pid() {
  local pid="${1:-}"
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
}

apply_phase2_override() {
  cp "$PHASE2_OVR" docker-compose.override.yml
  info "applying phase2 override (ai-service mem limit + healthcheck)"
  docker compose up -d --no-deps ai-service >/dev/null
  sleep 2
}

revert_override() {
  if [[ -f docker-compose.override.yml ]]; then
    rm -f docker-compose.override.yml
    info "reverting compose override"
    docker compose up -d --no-deps ai-service >/dev/null
  fi
}
trap revert_override EXIT

run_k6() {
  # run_k6 NAME SCRIPT [extra-k6-args]
  local name="$1" script="$2"; shift 2
  if k6 run --summary-export="$RESULTS_DIR/${name}.json" "$@" "script/k6/${script}" \
       2>&1 | tee "$LOG_DIR/${name}.log"; then
    echo PASS
  else
    echo FAIL
  fi
}

# ─── 0. Health gate ──────────────────────────────────────────────────────────
section "0. Health gate"
if ! health_check; then echo -e "${RED}Stack unhealthy — aborting${RESET}"; exit 1; fi

# ─── 1. auth_login (100 RPS) ──────────────────────────────────────────────────
section "1. auth_login.js — 100 RPS"
ST_AUTH=$(run_k6 auth_login auth_login.js)

# ─── 2. cart_ops (500 RPS) — with pg + redis monitors ────────────────────────
section "2. cart_ops.js — 500 RPS  (+ pg + redis monitors)"
PG_PID=$(start_monitor pg bash script/test/monitor_pg_connections.sh "$MONITORS_DIR/pg_cart.csv")
REDIS_PID=$(start_monitor redis bash script/test/monitor_redis.sh 70 "$MONITORS_DIR/redis_cart.log")
ST_CART=$(run_k6 cart_ops cart_ops.js)
stop_pid "$PG_PID"; stop_pid "$REDIS_PID"
cp "$MONITORS_DIR/pg_cart.csv" "$MONITORS_DIR/pg.csv" 2>/dev/null || true
cp "$MONITORS_DIR/redis_cart.log" "$MONITORS_DIR/redis.log" 2>/dev/null || true

# ─── 3. product_browse (150 RPS) — pg monitor ────────────────────────────────
section "3. product_browse.js — 150 RPS  (+ pg monitor)"
PG_PID=$(start_monitor pg bash script/test/monitor_pg_connections.sh "$MONITORS_DIR/pg_browse.csv")
ST_BROWSE=$(run_k6 product_browse product_browse.js)
stop_pid "$PG_PID"

# ─── 4. order_create (50 RPS) — pg + kafka monitors ──────────────────────────
section "4. order_create.js — 50 RPS  (+ pg + kafka monitors)"
PG_PID=$(start_monitor pg bash script/test/monitor_pg_connections.sh "$MONITORS_DIR/pg_order.csv")
KAFKA_PID=$(start_monitor kafka bash script/test/monitor_kafka_lag.sh "$MONITORS_DIR/kafka_lag_order.csv")
ST_ORDER=$(run_k6 order_create order_create.js)
stop_pid "$PG_PID"; stop_pid "$KAFKA_PID"

# ─── 5. payment_kafka_throughput ─────────────────────────────────────────────
if [[ "${SKIP_KAFKA_THROUGHPUT:-0}" != "1" ]]; then
  section "5. payment_kafka_throughput.sh — 10k messages  (+ kafka monitor)"
  KAFKA_PID=$(start_monitor kafka bash script/test/monitor_kafka_lag.sh "$MONITORS_DIR/kafka_lag.csv")
  if bash script/test/payment_kafka_throughput.sh "${KAFKA_N:-10000}" "$RESULTS_DIR" \
       2>&1 | tee "$LOG_DIR/kafka_throughput.log"; then
    ST_KAFKA=PASS
  else
    ST_KAFKA=FAIL
  fi
  stop_pid "$KAFKA_PID"
else
  ST_KAFKA=SKIPPED
fi

# ─── 6. apply phase2 override; AI tests ──────────────────────────────────────
apply_phase2_override

# 6.a cold start
section "6. ai_cold_start.sh"
if bash script/test/test_ai_cold_start.sh "$RESULTS_DIR/ai_cold_start.json" \
     2>&1 | tee "$LOG_DIR/ai_cold_start.log"; then
  ST_COLDSTART=PASS
else
  ST_COLDSTART=FAIL
fi

# 6.b ai_search load + ai_mem monitor
section "7. ai_search.js — 20 RPS  (+ ai_mem monitor)"
AI_MEM_PID=$(start_monitor ai_mem bash script/test/monitor_ai_mem.sh "$MONITORS_DIR/ai_mem.csv")
ST_AI=$(run_k6 ai_search ai_search.js)
stop_pid "$AI_MEM_PID"

# 6.c searchability lag (Java IT)
if [[ "${SKIP_AI_LAG:-0}" != "1" ]]; then
  section "8. AISearchabilityLagIT  (Maven test)"
  if (cd product-service && ./mvnw test -Dtest=AISearchabilityLagIT -q) \
       2>&1 | tee "$LOG_DIR/ai_lag.log"; then
    ST_LAG=PASS
  else
    ST_LAG=FAIL
  fi
else
  ST_LAG=SKIPPED
fi

revert_override

# ─── 9. checkout_50vu ────────────────────────────────────────────────────────
section "9. checkout_50vu.js"
ST_CHECKOUT=$(run_k6 checkout_50vu checkout_50vu.js)

# ─── 10. Hikari leak scan ────────────────────────────────────────────────────
section "10. Hikari leak scan"
bash script/test/monitor_hikari.sh 30m "$MONITORS_DIR/hikari.log" 2>&1 | tee -a "$LOG_DIR/hikari.log"

# ─── 11. Aggregate ───────────────────────────────────────────────────────────
section "11. aggregate_phase2.py"
python3 script/test/aggregate_phase2.py \
  --results-dir "$RESULTS_DIR" \
  --monitors-dir "$MONITORS_DIR" \
  --output docs/testing/test_result.md

# ─── 12. Summary ─────────────────────────────────────────────────────────────
section "Summary"
FAILED=0
summarize() {
  case "$2" in
    PASS)    echo -e "  ${GREEN}✓${RESET} $1";;
    SKIPPED) echo -e "  ${YELLOW}—${RESET} $1 (skipped)";;
    *)       echo -e "  ${RED}✗${RESET} $1"; FAILED=$((FAILED+1));;
  esac
}
summarize auth_login     "$ST_AUTH"
summarize cart_ops       "$ST_CART"
summarize product_browse "$ST_BROWSE"
summarize order_create   "$ST_ORDER"
summarize kafka_throughput "$ST_KAFKA"
summarize ai_cold_start  "$ST_COLDSTART"
summarize ai_search      "$ST_AI"
summarize searchability_lag "$ST_LAG"
summarize checkout_50vu  "$ST_CHECKOUT"
echo ""
if [[ "$FAILED" -gt 0 ]]; then
  echo -e "${RED}Phase 2 result: $FAILED scenario(s) failed (expected — surfaces Phase 1 IMP-1/2)${RESET}"
  exit 1
else
  echo -e "${GREEN}Phase 2 result: all scenarios PASS${RESET}"
fi
