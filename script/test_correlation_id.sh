#!/usr/bin/env bash
# Phase 1 — Correlation-ID propagation test (testing_target.md §7.C)
# Injects X-Correlation-ID at nginx, exercises a full saga (login → cart → order),
# then greps `docker compose logs` for the uuid across all 5 services.

set -euo pipefail

BASE="${BASE_URL:-http://localhost}"
CID="phase1-cid-$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid)"
SINCE="${SINCE:-2m}"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[0;33m'; BOLD='\033[1m'; RESET='\033[0m'

echo -e "${BOLD}=== Correlation-ID propagation test ===${RESET}"
echo "  X-Correlation-ID = $CID"
echo ""

# 1. Login (touches user-service)
LOGIN=$(curl -sS -X POST "$BASE/api/v1/auth/login" \
  -H "X-Correlation-ID: $CID" \
  -H 'Content-Type: application/json' \
  -d '{"email":"customer@example.com","password":"Customer@123"}')
TOKEN=$(echo "$LOGIN" | jq -r .data.access_token)
USER_ID=$(echo "$LOGIN" | jq -r .data.user.id)
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
  echo -e "${RED}Login failed: $LOGIN${RESET}"; exit 1
fi
echo -e "  ${YELLOW}→${RESET} login ok (user_id=$USER_ID)"

# 2. Add to cart (touches cart-service + product-service via internal HTTP)
curl -sS -X POST "$BASE/api/v1/cart/items" \
  -H "X-Correlation-ID: $CID" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"product_id":1,"quantity":1}' > /dev/null
echo -e "  ${YELLOW}→${RESET} cart/items ok"

# 3. Create order (order-service → Kafka → payment-service)
CART_UUID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid)
ORDER_PAYLOAD=$(jq -n --arg c "$CART_UUID" '{
  cartId: $c,
  items: [{productId: 1, quantity: 1}],
  shippingAddress: {street:"1 CID St", city:"HCM", state:"HCM", country:"VN", zipCode:"70000"}
}')
curl -sS -X POST "$BASE/api/v1/orders" \
  -H "X-Correlation-ID: $CID" \
  -H "X-User-Id: $USER_ID" \
  -H 'Content-Type: application/json' \
  -d "$ORDER_PAYLOAD" > /dev/null
echo -e "  ${YELLOW}→${RESET} orders ok"

# Let Kafka saga reach payment-service
sleep 4

# 4. Grep logs for the uuid in each service
echo ""
echo -e "${BOLD}Log scan (since ${SINCE}):${RESET}"
declare -a SERVICES=(user-service cart-service product-service order-service payment-service)
FAIL=0
for svc in "${SERVICES[@]}"; do
  if docker compose logs --since "$SINCE" "$svc" 2>/dev/null | grep -q "$CID"; then
    echo -e "  ${GREEN}✓${RESET} $svc"
  else
    echo -e "  ${RED}✗${RESET} $svc — correlation id not found in logs"
    FAIL=1
  fi
done

echo ""
if [[ "$FAIL" -eq 0 ]]; then
  echo -e "${GREEN}All 5 services propagated X-Correlation-ID.${RESET}"
  exit 0
else
  echo -e "${RED}Correlation-ID propagation incomplete.${RESET}"
  exit 1
fi
