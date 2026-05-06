#!/usr/bin/env bash
# End-to-end saga test: Login → Create Order → Wait → Verify Payment + Order status
# Proves the Kafka choreography saga: orders.created → payment-service → payments.completed/failed → order CONFIRMED/CANCELLED
#
# Usage:
#   bash script/e2e-payment.sh
#
# Override service URLs:
#   ORDER_SVC=http://localhost:8082 PAYMENT_SVC=http://localhost:8003 bash script/e2e-payment.sh

set -euo pipefail

USER_SVC="${USER_SVC:-http://localhost:8001}"
PRODUCT_SVC="${PRODUCT_SVC:-http://localhost:8081}"
PAYMENT_SVC="${PAYMENT_SVC:-http://localhost:8003}"
ORDER_SVC="${ORDER_SVC:-http://localhost:8082}"
SAGA_WAIT_SECONDS="${SAGA_WAIT_SECONDS:-20}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
RESET='\033[0m'

PASS=0
FAIL=0

pass() { echo -e "  ${GREEN}✓${RESET} $1"; ((PASS++)) || true; }
fail() { echo -e "  ${RED}✗${RESET} $1"; ((FAIL++)) || true; }
info() { echo -e "  ${YELLOW}→${RESET} $1"; }

assert_status() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    pass "$label (HTTP $actual)"
    return 0
  else
    fail "$label — expected HTTP $expected, got HTTP $actual"
    return 1
  fi
}

BODY_FILE=$(mktemp)
trap 'rm -f "$BODY_FILE"' EXIT

request() {
  local method="$1" url="$2"
  shift 2
  curl -s -o "$BODY_FILE" -w "%{http_code}" -X "$method" "$url" "$@"
}

echo ""
echo -e "${BOLD}=== E2E Saga Test: Order → Payment → Order Confirmation ===${RESET}"
echo ""

# ── Step 1: Login ──────────────────────────────────────────────────────────────
echo -e "${BOLD}Step 1: Login as customer@example.com${RESET}"
status=$(request POST "$USER_SVC/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"customer@example.com","password":"Customer@123"}')
assert_status "Login" "200" "$status"

ACCESS_TOKEN=$(jq -r '.data.access_token' "$BODY_FILE")
USER_ID=$(jq -r '.data.user.id' "$BODY_FILE")

if [[ -z "$ACCESS_TOKEN" || "$ACCESS_TOKEN" == "null" ]]; then
  echo -e "${RED}Fatal: could not extract access_token. Aborting.${RESET}"
  exit 1
fi
info "user_id = $USER_ID"
info "token   = ${ACCESS_TOKEN:0:40}..."

# ── Step 2: Get a product ──────────────────────────────────────────────────────
# Use the single-product endpoint (paginated list has a pre-existing Spring PageImpl/Redis issue).
echo ""
echo -e "${BOLD}Step 2: Pick product id=1 (Laptop ProBook)${RESET}"
PRODUCT_ID="${PRODUCT_ID:-1}"
status=$(request GET "$PRODUCT_SVC/api/v1/products/$PRODUCT_ID")
assert_status "Get product $PRODUCT_ID" "200" "$status"

PRODUCT_NAME=$(jq -r '.data.name' "$BODY_FILE")
if [[ -z "$PRODUCT_NAME" || "$PRODUCT_NAME" == "null" ]]; then
  echo -e "${RED}Fatal: product $PRODUCT_ID not found. Ensure seed data is applied.${RESET}"
  exit 1
fi
info "product_id   = $PRODUCT_ID"
info "product_name = $PRODUCT_NAME"

# ── Step 3: Create order (triggers OrderCreatedEvent → orders.created) ─────────
echo ""
echo -e "${BOLD}Step 3: Create order${RESET}"

CART_UUID=$(python3 -c "import uuid; print(uuid.uuid4())" 2>/dev/null || uuidgen | tr '[:upper:]' '[:lower:]')

ORDER_PAYLOAD=$(jq -n \
  --arg cartId "$CART_UUID" \
  --argjson productId "$PRODUCT_ID" \
  '{
    cartId: $cartId,
    items: [{ productId: $productId, quantity: 1 }],
    shippingAddress: {
      street: "123 Saga Street",
      city: "Ho Chi Minh City",
      state: "HCM",
      country: "Vietnam",
      zipCode: "70000"
    }
  }')

status=$(request POST "$ORDER_SVC/api/v1/orders" \
  -H "Content-Type: application/json" \
  -H "X-User-Id: $USER_ID" \
  -d "$ORDER_PAYLOAD")
assert_status "Create order" "201" "$status"

ORDER_ID=$(jq -r '.data.id' "$BODY_FILE")
ORDER_STATUS_INITIAL=$(jq -r '.data.status' "$BODY_FILE")
info "order_id            = $ORDER_ID"
info "order_status (initial) = $ORDER_STATUS_INITIAL"

if [[ "$ORDER_STATUS_INITIAL" == "PENDING" ]]; then
  pass "Order created in PENDING state — saga starting"
else
  fail "Expected PENDING, got $ORDER_STATUS_INITIAL"
fi

# ── Step 4: Poll until payment appears (max SAGA_WAIT_SECONDS seconds) ─────────
echo ""
echo -e "${BOLD}Step 4: Polling for payment (up to ${SAGA_WAIT_SECONDS}s)...${RESET}"
POLL_START=$(date +%s)
PAYMENT_FOUND=false
while true; do
  sleep 1
  http_code=$(request GET "$PAYMENT_SVC/api/v1/payments/order/$ORDER_ID" \
    -H "Authorization: Bearer $ACCESS_TOKEN")
  if [[ "$http_code" == "200" ]]; then
    PAYMENT_FOUND=true
    break
  fi
  ELAPSED=$(( $(date +%s) - POLL_START ))
  if [[ "$ELAPSED" -ge "$SAGA_WAIT_SECONDS" ]]; then
    break
  fi
done
ELAPSED=$(( $(date +%s) - POLL_START ))
info "saga completed in ${ELAPSED}s"

# ── Step 5: Verify payment was created ────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 5: Verify payment via GET /api/v1/payments/order/{orderId}${RESET}"
status=$(request GET "$PAYMENT_SVC/api/v1/payments/order/$ORDER_ID" \
  -H "Authorization: Bearer $ACCESS_TOKEN")
assert_status "Get payment by orderId" "200" "$status"

PAYMENT_ID=$(jq -r '.data.id' "$BODY_FILE")
PAYMENT_STATUS=$(jq -r '.data.status' "$BODY_FILE")
PAYMENT_AMOUNT=$(jq -r '.data.amount' "$BODY_FILE")
GATEWAY_REF=$(jq -r '.data.gatewayReference // "n/a"' "$BODY_FILE")
info "payment_id        = $PAYMENT_ID"
info "payment_status    = $PAYMENT_STATUS"
info "payment_amount    = $PAYMENT_AMOUNT"
info "gateway_reference = $GATEWAY_REF"

if [[ "$PAYMENT_STATUS" == "COMPLETED" || "$PAYMENT_STATUS" == "FAILED" ]]; then
  pass "Payment is in terminal state: $PAYMENT_STATUS"
else
  fail "Payment still in non-terminal state: $PAYMENT_STATUS"
fi

# ── Step 6: Verify order status transitioned ──────────────────────────────────
echo ""
echo -e "${BOLD}Step 6: Verify order transitioned via GET /api/v1/orders/{orderId}${RESET}"
status=$(request GET "$ORDER_SVC/api/v1/orders/$ORDER_ID" \
  -H "X-User-Id: $USER_ID")
assert_status "Get order detail" "200" "$status"

ORDER_STATUS_FINAL=$(jq -r '.data.status' "$BODY_FILE")
info "order_status (final) = $ORDER_STATUS_FINAL"

if [[ "$PAYMENT_STATUS" == "COMPLETED" && "$ORDER_STATUS_FINAL" == "CONFIRMED" ]]; then
  pass "Saga happy path: COMPLETED payment → CONFIRMED order"
elif [[ "$PAYMENT_STATUS" == "FAILED" && "$ORDER_STATUS_FINAL" == "CANCELLED" ]]; then
  pass "Saga failure path: FAILED payment → CANCELLED order"
else
  fail "Saga mismatch: payment=$PAYMENT_STATUS but order=$ORDER_STATUS_FINAL"
fi

# ── Step 7: Verify payment listing ────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 7: Verify payment appears in user payment list${RESET}"
status=$(request GET "$PAYMENT_SVC/api/v1/payments?page=1&size=10" \
  -H "Authorization: Bearer $ACCESS_TOKEN")
assert_status "List payments" "200" "$status"

PAYMENT_COUNT=$(jq '.data | length' "$BODY_FILE")
info "payments returned = $PAYMENT_COUNT"
if [[ "$PAYMENT_COUNT" -ge 1 ]]; then
  pass "Payment appears in user payment list"
else
  fail "Payment list is empty"
fi

# ── Step 8: Verify /health/ready ──────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 8: Health check — both postgres and kafka UP${RESET}"
status=$(request GET "$PAYMENT_SVC/health/ready")
assert_status "Health ready" "200" "$status"
POSTGRES_STATUS=$(jq -r '.checks.postgres' "$BODY_FILE")
KAFKA_STATUS=$(jq -r '.checks.kafka' "$BODY_FILE")
info "postgres = $POSTGRES_STATUS"
info "kafka    = $KAFKA_STATUS"
if [[ "$POSTGRES_STATUS" == "UP" && "$KAFKA_STATUS" == "UP" ]]; then
  pass "Both dependencies healthy"
else
  fail "Dependency not healthy: postgres=$POSTGRES_STATUS kafka=$KAFKA_STATUS"
fi

# ── Summary ────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}=== Results ===${RESET}"
echo -e "  ${GREEN}Passed: $PASS${RESET}"
if [[ "$FAIL" -gt 0 ]]; then
  echo -e "  ${RED}Failed: $FAIL${RESET}"
  echo ""
  exit 1
else
  echo "  Failed: $FAIL"
  echo ""
  echo -e "${GREEN}All saga checks passed. Kafka choreography is working end-to-end.${RESET}"
fi
