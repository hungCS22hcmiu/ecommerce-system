#!/usr/bin/env bash
# Phase 4 orchestrator — Testing Debt.
# Runs the new + extended unit/integration tests, captures pass/fail + coverage
# numbers, appends a "Phase 4 — Testing Debt" section to test_result.md.
#
# Bash 3.2 compatible.

set -uo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

RESULTS_DIR="script/k6/results"
LOG_DIR="${RESULTS_DIR}/logs"
mkdir -p "$RESULTS_DIR" "$LOG_DIR"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; RESET='\033[0m'
section() { echo ""; echo -e "${BOLD}── $* ──${RESET}"; }
info()    { echo -e "  ${YELLOW}→${RESET} $*"; }

ST_USER_PKG="MISSING"; ST_CB_UNIT="MISSING"; ST_IDEMPOTENCY="MISSING"
ST_REPO_QUERY="MISSING"; ST_EMBED="MISSING"
COV_USER_PKG=""; COV_CB=""

# ─── 1. user-service pkg tests (blacklist, verification, reset) with coverage ─
section "1. user-service/pkg tests (blacklist, verification, reset)"
(cd user-service && go test -race -coverprofile="$ROOT_DIR/$RESULTS_DIR/coverage_user_pkg.out" \
    ./pkg/blacklist/... ./pkg/verification/... ./pkg/reset/... ) \
    2>&1 | tee "$LOG_DIR/phase4_user_pkg.log"
PKG_STATUS=${PIPESTATUS[0]}
if [[ "$PKG_STATUS" == "0" ]]; then ST_USER_PKG=PASS; else ST_USER_PKG=FAIL; fi

if [[ -f "$RESULTS_DIR/coverage_user_pkg.out" ]]; then
  COV_USER_PKG=$(cd user-service && go tool cover -func="$ROOT_DIR/$RESULTS_DIR/coverage_user_pkg.out" \
    | awk '/^total:/ {print $3}')
  info "user-service/pkg coverage = $COV_USER_PKG"
fi

# ─── 2. cart-service circuit-breaker unit test with coverage ─────────────────
section "2. cart-service CircuitBreaker unit test"
(cd cart-service && go test -race -coverprofile="$ROOT_DIR/$RESULTS_DIR/coverage_cart_cb.out" \
    -run "TestCircuitBreaker" ./internal/client/...) \
    2>&1 | tee "$LOG_DIR/phase4_cart_cb.log"
CB_STATUS=${PIPESTATUS[0]}
if [[ "$CB_STATUS" == "0" ]]; then ST_CB_UNIT=PASS; else ST_CB_UNIT=FAIL; fi

if [[ -f "$RESULTS_DIR/coverage_cart_cb.out" ]]; then
  COV_CB=$(cd cart-service && go tool cover -func="$ROOT_DIR/$RESULTS_DIR/coverage_cart_cb.out" \
    | awk '/circuit_breaker.go/ && $3 ~ /%/' | awk '{sum+=$3; n++} END {if (n>0) printf "%.1f%%\n", sum/n; else print "n/a"}')
  info "circuit_breaker.go coverage = $COV_CB"
fi

# ─── 3. payment-service idempotency at N=20 (integration tag) ────────────────
if [[ "${SKIP_IDEMPOTENCY:-0}" != "1" ]]; then
  section "3. payment-service TestConcurrentIdempotency (N=20)"
  (cd payment-service && go test -tags=integration -v -race -count=1 \
      -run TestConcurrentIdempotency ./internal/integration/...) \
      2>&1 | tee "$LOG_DIR/phase4_idempotency.log"
  IDEMP_STATUS=${PIPESTATUS[0]}
  if [[ "$IDEMP_STATUS" == "0" ]]; then ST_IDEMPOTENCY=PASS; else ST_IDEMPOTENCY=FAIL; fi
else
  ST_IDEMPOTENCY=SKIPPED
fi

# ─── 4. product-service ProductRepositoryQueryIT ─────────────────────────────
if [[ "${SKIP_REPO_QUERY:-0}" != "1" ]]; then
  section "4. product-service ProductRepositoryQueryIT"
  (cd product-service && ./mvnw test -Dtest=ProductRepositoryQueryIT -q) \
      2>&1 | tee "$LOG_DIR/phase4_repo_query.log"
  RQ_STATUS=${PIPESTATUS[0]}
  if [[ "$RQ_STATUS" == "0" ]]; then ST_REPO_QUERY=PASS; else ST_REPO_QUERY=FAIL; fi
else
  ST_REPO_QUERY=SKIPPED
fi

# ─── 5. product-service EmbeddingClientTest ──────────────────────────────────
if [[ "${SKIP_EMBED:-0}" != "1" ]]; then
  section "5. product-service EmbeddingClientTest"
  (cd product-service && ./mvnw test -Dtest=EmbeddingClientTest -q) \
      2>&1 | tee "$LOG_DIR/phase4_embed.log"
  EM_STATUS=${PIPESTATUS[0]}
  if [[ "$EM_STATUS" == "0" ]]; then ST_EMBED=PASS; else ST_EMBED=FAIL; fi
else
  ST_EMBED=SKIPPED
fi

# ─── 6. Aggregate ────────────────────────────────────────────────────────────
section "6. aggregate_phase4.py"
python3 script/test/aggregate_phase4.py \
  --results-dir "$RESULTS_DIR" \
  --log-dir "$LOG_DIR" \
  --output docs/testing/test_result.md \
  --status "user_pkg=${ST_USER_PKG}" \
  --status "cb_unit=${ST_CB_UNIT}" \
  --status "idempotency=${ST_IDEMPOTENCY}" \
  --status "repo_query=${ST_REPO_QUERY}" \
  --status "embed=${ST_EMBED}" \
  --coverage "user_pkg=${COV_USER_PKG}" \
  --coverage "circuit_breaker=${COV_CB}"

# ─── Summary ─────────────────────────────────────────────────────────────────
section "Summary"
FAILED=0
summarize() {
  case "$2" in
    PASS)    echo -e "  ${GREEN}✓${RESET} $1";;
    SKIPPED) echo -e "  ${YELLOW}—${RESET} $1 (skipped)";;
    *)       echo -e "  ${RED}✗${RESET} $1"; FAILED=$((FAILED+1));;
  esac
}
summarize "user-service/pkg (blacklist+verification+reset)" "$ST_USER_PKG"
summarize "cart-service CircuitBreaker unit"                "$ST_CB_UNIT"
summarize "payment-service idempotency N=20"                "$ST_IDEMPOTENCY"
summarize "product-service ProductRepositoryQueryIT"        "$ST_REPO_QUERY"
summarize "product-service EmbeddingClientTest"             "$ST_EMBED"
echo ""
if [[ "$FAILED" -gt 0 ]]; then
  echo -e "${RED}Phase 4 result: $FAILED scenario(s) failed${RESET}"
  exit 1
else
  echo -e "${GREEN}Phase 4 result: all scenarios PASS${RESET}"
fi
