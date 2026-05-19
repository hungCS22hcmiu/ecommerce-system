#!/usr/bin/env bash
# Phase 3 orchestrator — Resilience & Chaos.
# Runs every scenario with per-step traps to restore stack state on failure.
# Bash 3.2-compatible (no associative arrays).

set -uo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

RESULTS_DIR="script/k6/results"
LOG_DIR="${RESULTS_DIR}/logs"
mkdir -p "$RESULTS_DIR" "$LOG_DIR"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; RESET='\033[0m'
section() { echo ""; echo -e "${BOLD}── $* ──${RESET}"; }
info()    { echo -e "  ${YELLOW}→${RESET} $*"; }

ST_CB="MISSING"; ST_DEG="MISSING"; ST_RACE="MISSING"
ST_API="MISSING"; ST_AUTH="MISSING"; ST_KILL="MISSING"

health_check() {
  local fail=0
  for entry in "user-service|http://localhost:8001/health/live" \
               "product-service|http://localhost:8081/health/live" \
               "cart-service|http://localhost:8002/health/live" \
               "order-service|http://localhost:8082/health/live" \
               "payment-service|http://localhost:8003/health/ready"; do
    name="${entry%%|*}"; url="${entry##*|}"
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "$url" 2>/dev/null || echo "000")
    if [[ "$code" == "200" ]]; then
      echo -e "  ${GREEN}✓${RESET} $name"
    else
      echo -e "  ${RED}✗${RESET} $name ($code) — $url"; fail=1
    fi
  done
  return $fail
}

# Global safety trap — restore stack on any exit path
global_restore() {
  docker compose unpause product-service >/dev/null 2>&1 || true
  docker compose start payment-service >/dev/null 2>&1 || true
}
trap global_restore EXIT

# ─── 0. Health gate ──────────────────────────────────────────────────────────
section "0. Health gate"
if ! health_check; then echo -e "${RED}Stack unhealthy — aborting${RESET}"; exit 1; fi

# ─── 1. §7.A Circuit breaker + degraded mode ─────────────────────────────────
if [[ "${SKIP_CB:-0}" != "1" ]]; then
  section "1. chaos_cb_cart.sh  (§7.A CB + degraded GET /cart)"
  if bash script/test/chaos_cb_cart.sh 2>&1 | tee "$LOG_DIR/chaos_cb_cart.log"; then
    ST_CB=PASS; ST_DEG=PASS
  else
    ST_CB=FAIL; ST_DEG=FAIL
  fi
  # The exit code from chaos_cb_cart.sh is always 0 — aggregator decides PASS/FAIL.
  ST_CB=$([[ -f "$RESULTS_DIR/chaos_cb_cart.json" ]] && echo "RAN" || echo "MISSING")
  ST_DEG=$([[ -f "$RESULTS_DIR/cart_get_degraded.json" ]] && echo "RAN" || echo "MISSING")
else
  info "skipped"
fi

# ─── 2. §7.D API rate limit ──────────────────────────────────────────────────
if [[ "${SKIP_RATELIMIT:-0}" != "1" ]]; then
  section "2. rate_limit_api.js  (§7.D — 50-burst general)"
  if k6 run --summary-export="$RESULTS_DIR/rate_limit_api.json" \
       script/k6/rate_limit_api.js 2>&1 | tee "$LOG_DIR/rate_limit_api.log"; then
    ST_API=RAN
  else
    ST_API=RAN  # k6 may exit non-zero on 429s; aggregator decides
  fi

  # nginx zones are leaky-bucket — wait > 1 min so auth_limit budget refills
  info "waiting 65s for auth_limit budget refill before auth test"
  sleep 65

  section "3. rate_limit_auth.js  (§7.D — 9 sequential logins)"
  if k6 run --summary-export="$RESULTS_DIR/rate_limit_auth.json" \
       script/k6/rate_limit_auth.js 2>&1 | tee "$LOG_DIR/rate_limit_auth.log"; then
    ST_AUTH=RAN
  else
    ST_AUTH=RAN
  fi
else
  info "skipped"
fi

# ─── 4. §3.C Order state-machine race ────────────────────────────────────────
if [[ "${SKIP_RACE:-0}" != "1" ]]; then
  section "4. chaos_order_race.sh  (§3.C deadlock-free transitions)"
  if bash script/test/chaos_order_race.sh 2>&1 | tee "$LOG_DIR/chaos_order_race.log"; then
    ST_RACE=RAN
  else
    ST_RACE=FAIL
  fi
else
  info "skipped"
fi

# ─── 5. Mid-saga payment-service kill recovery ───────────────────────────────
if [[ "${SKIP_KILL:-0}" != "1" ]]; then
  section "5. chaos_saga_kill.sh  (mid-saga recovery)"
  if bash script/test/chaos_saga_kill.sh 2>&1 | tee "$LOG_DIR/chaos_saga_kill.log"; then
    ST_KILL=RAN
  else
    ST_KILL=FAIL
  fi
else
  info "skipped"
fi

# ─── 6. Aggregate ────────────────────────────────────────────────────────────
section "6. aggregate_phase3.py"
python3 script/test/aggregate_phase3.py \
  --results-dir "$RESULTS_DIR" \
  --output docs/testing/test_result.md

# ─── Summary ─────────────────────────────────────────────────────────────────
section "Summary"
echo "  chaos_cb_cart        : $ST_CB"
echo "  cart_get_degraded    : $ST_DEG"
echo "  rate_limit_api       : $ST_API"
echo "  rate_limit_auth      : $ST_AUTH"
echo "  chaos_order_race     : $ST_RACE"
echo "  chaos_saga_kill      : $ST_KILL"
echo ""
echo -e "${GREEN}Phase 3 scenarios complete. See docs/testing/test_result.md for PASS/FAIL per target.${RESET}"
