#!/usr/bin/env bash
# Phase 5 orchestrator — Frontend UX & Reliability.
#
# Runs Vitest unit suite (axios queue, cart optimistic, toast) + Playwright E2E
# (responsive + a11y) against the docker compose frontend on :3001.
#
# Bash 3.2 compatible.

set -uo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR/frontend"

RESULTS_DIR="$ROOT_DIR/script/k6/results"
LOG_DIR="$RESULTS_DIR/logs"
mkdir -p "$RESULTS_DIR" "$LOG_DIR"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; RESET='\033[0m'
section() { echo ""; echo -e "${BOLD}── $* ──${RESET}"; }
info()    { echo -e "  ${YELLOW}→${RESET} $*"; }

ST_VITEST="MISSING"; ST_RESP="MISSING"; ST_A11Y="MISSING"

# ─── 1. Vitest unit tests ───────────────────────────────────────────────────
section "1. Vitest  (toast.test, useCart.test, axios.test)"
if npx vitest run --reporter=json --outputFile="$RESULTS_DIR/vitest.json" \
     2>&1 | tee "$LOG_DIR/phase5_vitest.log"; then
  ST_VITEST=PASS
else
  ST_VITEST=FAIL
fi

# ─── 2. Playwright — browser install (idempotent) ───────────────────────────
if [[ "${SKIP_PLAYWRIGHT:-0}" != "1" ]]; then
  section "2. Playwright — install Chromium (one-time)"
  npx playwright install chromium 2>&1 | tee "$LOG_DIR/phase5_pw_install.log"

  # Pre-flight: docker-served frontend must be reachable.
  if ! curl -sf -o /dev/null --max-time 3 http://localhost:3001/ ; then
    info "frontend not reachable on :3001 — skipping E2E specs"
    ST_RESP=SKIPPED; ST_A11Y=SKIPPED
  else
    section "3. Playwright responsive.spec.ts"
    if npx playwright test tests/e2e/responsive.spec.ts --reporter=json \
         > "$LOG_DIR/phase5_responsive.json" 2> "$LOG_DIR/phase5_responsive.log"; then
      ST_RESP=PASS
    else
      ST_RESP=FAIL
    fi
    cp "$LOG_DIR/phase5_responsive.json" "$RESULTS_DIR/playwright_responsive.json" 2>/dev/null || true

    section "4. Playwright a11y.spec.ts"
    if npx playwright test tests/e2e/a11y.spec.ts --reporter=json \
         > "$LOG_DIR/phase5_a11y.json" 2> "$LOG_DIR/phase5_a11y.log"; then
      ST_A11Y=PASS
    else
      ST_A11Y=FAIL
    fi
    cp "$LOG_DIR/phase5_a11y.json" "$RESULTS_DIR/playwright_a11y.json" 2>/dev/null || true
  fi
else
  info "Playwright skipped via SKIP_PLAYWRIGHT=1"
  ST_RESP=SKIPPED; ST_A11Y=SKIPPED
fi

# ─── 5. Aggregate ───────────────────────────────────────────────────────────
section "5. aggregate_phase5.py"
cd "$ROOT_DIR"
python3 script/test/aggregate_phase5.py \
  --results-dir "$RESULTS_DIR" \
  --log-dir "$LOG_DIR" \
  --output docs/testing/test_result.md \
  --status "vitest=${ST_VITEST}" \
  --status "responsive=${ST_RESP}" \
  --status "a11y=${ST_A11Y}"

# ─── Summary ────────────────────────────────────────────────────────────────
section "Summary"
FAILED=0
summarize() {
  case "$2" in
    PASS)    echo -e "  ${GREEN}✓${RESET} $1";;
    SKIPPED) echo -e "  ${YELLOW}—${RESET} $1 (skipped)";;
    *)       echo -e "  ${RED}✗${RESET} $1"; FAILED=$((FAILED+1));;
  esac
}
summarize "Vitest unit (toast, useCart, axios)" "$ST_VITEST"
summarize "Playwright responsive @ 320px"        "$ST_RESP"
summarize "Playwright a11y (axe)"                "$ST_A11Y"
echo ""
if [[ "$FAILED" -gt 0 ]]; then
  echo -e "${RED}Phase 5 result: $FAILED scenario(s) failed${RESET}"; exit 1
else
  echo -e "${GREEN}Phase 5 result: all scenarios PASS${RESET}"
fi
