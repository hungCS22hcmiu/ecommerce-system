#!/usr/bin/env bash
# Performance baseline: measures p50/avg/max response times for key operations through Nginx.
# Run after the full stack is up: docker compose up -d
#
# Usage:
#   bash script/perf-baseline.sh
#
# Override base URL (default: Nginx on port 80):
#   BASE_URL=http://localhost bash script/perf-baseline.sh

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost}"
PRODUCT_REQUESTS="${PRODUCT_REQUESTS:-10}"
ORDER_REQUESTS="${ORDER_REQUESTS:-5}"

BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RESET='\033[0m'

BODY_FILE=$(mktemp)
trap 'rm -f "$BODY_FILE"' EXIT

# ── Login to get JWT token ────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}=== Performance Baseline (through Nginx at $BASE_URL) ===${RESET}"
echo ""
echo "Logging in as customer@example.com..."

LOGIN_STATUS=$(curl -s -o "$BODY_FILE" -w "%{http_code}" -X POST "$BASE_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"customer@example.com","password":"Customer@123"}')

if [[ "$LOGIN_STATUS" != "200" ]]; then
  echo "Login failed (HTTP $LOGIN_STATUS). Is the stack running?"
  exit 1
fi

ACCESS_TOKEN=$(jq -r '.data.access_token' "$BODY_FILE")
USER_ID=$(jq -r '.data.user.id' "$BODY_FILE")

if [[ -z "$ACCESS_TOKEN" || "$ACCESS_TOKEN" == "null" ]]; then
  echo "Could not extract access_token. Aborting."
  exit 1
fi

echo "  Logged in. user_id=$USER_ID"

# ── Helper: run N timed requests, report min/avg/max in ms ───────────────────
# Usage: measure_get <label> <url> [extra curl args...]
measure_get() {
  local label="$1" url="$2"
  shift 2
  local times=() total=0 min=999999 max=0 n=$PRODUCT_REQUESTS

  for _ in $(seq 1 "$n"); do
    t=$(curl -s -o /dev/null -w "%{time_total}" "$url" "$@")
    ms=$(echo "$t * 1000" | bc | cut -d. -f1)
    times+=("$ms")
    total=$((total + ms))
    [[ "$ms" -lt "$min" ]] && min="$ms"
    [[ "$ms" -gt "$max" ]] && max="$ms"
  done

  local avg=$((total / n))
  # Sort to get p50
  local sorted
  sorted=$(printf '%s\n' "${times[@]}" | sort -n)
  local p50_idx=$(( (n + 1) / 2 ))
  local p50
  p50=$(echo "$sorted" | sed -n "${p50_idx}p")

  printf "  %-40s  n=%-3s  min=%4sms  p50=%4sms  avg=%4sms  max=%4sms\n" \
    "$label" "$n" "$min" "$p50" "$avg" "$max"
}

measure_order() {
  local label="$1"
  local n=$ORDER_REQUESTS
  local times=() total=0 min=999999 max=0

  # Pick product ID from first available
  PRODUCT_ID=""
  for cid in 1 2 3 4 5 6 7 8 9 10; do
    pstatus=$(curl -s -o "$BODY_FILE" -w "%{http_code}" "$BASE_URL/api/v1/products/$cid")
    if [[ "$pstatus" == "200" ]]; then
      pname=$(jq -r '.data.name // empty' "$BODY_FILE")
      if [[ -n "$pname" ]]; then
        PRODUCT_ID="$cid"
        break
      fi
    fi
  done

  if [[ -z "$PRODUCT_ID" ]]; then
    echo "  $label — skipped (no product with stock found; run seed data)"
    return
  fi

  for _ in $(seq 1 "$n"); do
    CART_UUID=$(python3 -c "import uuid; print(uuid.uuid4())" 2>/dev/null || uuidgen | tr '[:upper:]' '[:lower:]')
    PAYLOAD=$(jq -n \
      --arg cartId "$CART_UUID" \
      --argjson productId "$PRODUCT_ID" \
      '{cartId: $cartId, items: [{productId: $productId, quantity: 1}],
        shippingAddress: {street:"1 Perf St", city:"HCMC", state:"HCM", country:"Vietnam", zipCode:"70000"}}')

    t=$(curl -s -o /dev/null -w "%{time_total}" -X POST "$BASE_URL/api/v1/orders" \
      -H "Content-Type: application/json" \
      -H "X-User-Id: $USER_ID" \
      -d "$PAYLOAD")
    ms=$(echo "$t * 1000" | bc | cut -d. -f1)
    times+=("$ms")
    total=$((total + ms))
    [[ "$ms" -lt "$min" ]] && min="$ms"
    [[ "$ms" -gt "$max" ]] && max="$ms"
  done

  local avg=$((total / n))
  local sorted
  sorted=$(printf '%s\n' "${times[@]}" | sort -n)
  local p50_idx=$(( (n + 1) / 2 ))
  local p50
  p50=$(echo "$sorted" | sed -n "${p50_idx}p")

  printf "  %-40s  n=%-3s  min=%4sms  p50=%4sms  avg=%4sms  max=%4sms\n" \
    "$label" "$n" "$min" "$p50" "$avg" "$max"
}

# ── Measurements ──────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Read operations (n=$PRODUCT_REQUESTS each):${RESET}"
measure_get "GET /api/v1/products (list)"       "$BASE_URL/api/v1/products"
measure_get "GET /api/v1/products/1 (single)"   "$BASE_URL/api/v1/products/1"
measure_get "GET /api/v1/products?q=shoes"      "$BASE_URL/api/v1/products?q=shoes"

echo ""
echo -e "${BOLD}Auth operations (n=$PRODUCT_REQUESTS each):${RESET}"
measure_get "POST /api/v1/auth/login" "$BASE_URL/api/v1/auth/login" \
  -X POST -H "Content-Type: application/json" \
  -d '{"email":"customer@example.com","password":"Customer@123"}'

echo ""
echo -e "${BOLD}Write operations (n=$ORDER_REQUESTS each):${RESET}"
measure_order "POST /api/v1/orders (create order)"

echo ""
echo -e "${YELLOW}Note: these are single-threaded baselines. For concurrency testing, see Week 15 k6 load tests.${RESET}"
echo ""
