#!/usr/bin/env bash
# Phase 3 §7.A — Circuit breaker + degraded mode test.
#
# 1. Login as customer, seed cart via POST /cart/items while product-service is up.
# 2. Pause product-service.
# 3. Fire 10 POST /cart/items; expect CB to open after the first 5 exhausted retries
#    and subsequent calls to fast-fail with HTTP 503 (≥6 of 10 should be 503).
# 4. Run k6 GET /cart at 10 RPS × 30s while product-service still paused;
#    expect p(95) < 20ms (Redis-only path).
# 5. Always unpause product-service on exit.
#
# Outputs:
#   script/k6/results/chaos_cb_cart.json    {post_503_count, post_other_count}
#   script/k6/results/cart_get_degraded.json (k6 summary export)

set -uo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

RESULTS_DIR="script/k6/results"
mkdir -p "$RESULTS_DIR"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; RESET='\033[0m'
info() { echo -e "  ${YELLOW}→${RESET} $*"; }

cleanup() {
  info "cleanup: unpausing product-service"
  docker compose unpause product-service >/dev/null 2>&1 || true
}
trap cleanup EXIT

# 1. Login + pre-seed cart
info "logging in + pre-seeding cart"
TOKEN=$(curl -fsS -X POST http://localhost:8001/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"customer@example.com","password":"Customer@123"}' | jq -r .data.access_token)

if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
  echo -e "${RED}Login failed${RESET}"; exit 1
fi

# Seed: 1 item in the cart so GET /cart has data to return
curl -fsS -X POST http://localhost:8002/api/v1/cart/items \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"product_id":1,"quantity":1}' >/dev/null

# 2. Pause product-service
info "pausing product-service"
docker compose pause product-service >/dev/null

# 3. Fire 10 POST /cart/items; count 503s
info "firing 10 POST /cart/items..."
N_503=0; N_OTHER=0
STATUSES=""
for i in $(seq 1 10); do
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 30 \
    -X POST http://localhost:8002/api/v1/cart/items \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -d '{"product_id":1,"quantity":1}')
  STATUSES="${STATUSES}${code} "
  if [[ "$code" == "503" ]]; then N_503=$((N_503+1)); else N_OTHER=$((N_OTHER+1)); fi
done
info "POST statuses: $STATUSES"
info "503=$N_503, other=$N_OTHER"

# 4. Run k6 GET /cart degraded
info "running cart_get_degraded.js (30s)"
TOKEN="$TOKEN" k6 run \
  --summary-export="$RESULTS_DIR/cart_get_degraded.json" \
  script/k6/cart_get_degraded.js
K6_STATUS=$?
info "k6 exit code: $K6_STATUS"

# 5. Emit result JSON for aggregator
cat > "$RESULTS_DIR/chaos_cb_cart.json" <<EOF
{
  "post_503_count": $N_503,
  "post_other_count": $N_OTHER,
  "post_statuses": "$STATUSES",
  "k6_degraded_status": $K6_STATUS
}
EOF
info "wrote $RESULTS_DIR/chaos_cb_cart.json"

# Exit successfully — the aggregator decides PASS/FAIL based on counts
exit 0
