#!/usr/bin/env bash
# End-to-end test: Login → Browse → Add to Cart → Create Order → Verify Stock
# Uses pre-seeded customer account (no email verification needed).
# Requires: curl, jq
#
# Usage:
#   bash script/e2e-test.sh
#
# Override service URLs:
#   USER_SVC=http://localhost:8001 CART_SVC=http://localhost:8002 bash script/e2e-test.sh

set -euo pipefail

USER_SVC="${USER_SVC:-http://localhost:8001}"
PRODUCT_SVC="${PRODUCT_SVC:-http://localhost:8081}"
CART_SVC="${CART_SVC:-http://localhost:8002}"
ORDER_SVC="${ORDER_SVC:-http://localhost:8082}"

GREEN='\033[0;32m'
RED='\033[0;31m'
BOLD='\033[1m'
RESET='\033[0m'

PASS=0
FAIL=0

pass() { echo -e "  ${GREEN}✓${RESET} $1"; ((PASS++)); }
fail() { echo -e "  ${RED}✗${RESET} $1"; ((FAIL++)); }

assert_status() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    pass "$label (HTTP $actual)"
  else
    fail "$label — expected HTTP $expected, got HTTP $actual"
    return 1
  fi
}

# Runs curl, writes body to $BODY_FILE, returns status code.
# Usage: status=$(request GET "$url" [extra curl args...])
BODY_FILE=$(mktemp)
trap 'rm -f "$BODY_FILE"' EXIT

request() {
  local method="$1" url="$2"
  shift 2
  curl -s -o "$BODY_FILE" -w "%{http_code}" -X "$method" "$url" "$@"
}

echo ""
echo -e "${BOLD}=== E2E Test: Browse → Cart → Order ===${RESET}"
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
echo "  user_id       = $USER_ID"
echo "  access_token  = ${ACCESS_TOKEN:0:40}..."

# ── Step 2: Browse products ────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 2: Browse products${RESET}"
status=$(request GET "$PRODUCT_SVC/api/v1/products")
assert_status "List products" "200" "$status"

PRODUCT_ID=$(jq -r '.data.content[0].id' "$BODY_FILE")
PRODUCT_NAME=$(jq -r '.data.content[0].name' "$BODY_FILE")

if [[ -z "$PRODUCT_ID" || "$PRODUCT_ID" == "null" ]]; then
  echo -e "${RED}Fatal: no products found. Run seed data first.${RESET}"
  exit 1
fi
echo "  selected product id   = $PRODUCT_ID"
echo "  selected product name = $PRODUCT_NAME"

# ── Step 3: Check initial stock ───────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 3: Check initial stock for product $PRODUCT_ID${RESET}"
status=$(request GET "$PRODUCT_SVC/api/v1/inventory/$PRODUCT_ID")
assert_status "Get initial stock" "200" "$status"

INITIAL_STOCK=$(jq -r '.data.stockQuantity' "$BODY_FILE")
echo "  initial stockQuantity = $INITIAL_STOCK"

if [[ "$INITIAL_STOCK" -lt 2 ]]; then
  echo -e "${RED}Fatal: product has less than 2 units in stock. Choose another product.${RESET}"
  exit 1
fi

# ── Step 4: Add item to cart ───────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 4: Add product $PRODUCT_ID to cart (qty 2)${RESET}"
status=$(request POST "$CART_SVC/api/v1/cart/items" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -d "{\"product_id\":$PRODUCT_ID,\"quantity\":2}")
assert_status "Add item to cart" "200" "$status"

CART_ITEMS_COUNT=$(jq '.data.items | length' "$BODY_FILE")
echo "  cart items count = $CART_ITEMS_COUNT"

# ── Step 5: Get cart ───────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 5: Get cart${RESET}"
status=$(request GET "$CART_SVC/api/v1/cart" \
  -H "Authorization: Bearer $ACCESS_TOKEN")
assert_status "Get cart" "200" "$status"

CART_TOTAL=$(jq -r '.data.total' "$BODY_FILE")
echo "  cart total = $CART_TOTAL"

if [[ "$(jq '.data.items | length' "$BODY_FILE")" -lt 1 ]]; then
  fail "Cart should contain at least 1 item"
else
  pass "Cart contains items"
fi

# ── Step 6: Create order ───────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 6: Create order from cart${RESET}"

# Generate a cart UUID to associate with the order (order-service stores but does not validate it).
CART_UUID=$(python3 -c "import uuid; print(uuid.uuid4())" 2>/dev/null || uuidgen | tr '[:upper:]' '[:lower:]')

ORDER_PAYLOAD=$(jq -n \
  --argjson cartId "\"$CART_UUID\"" \
  --argjson productId "$PRODUCT_ID" \
  '{
    cartId: $cartId,
    items: [{ productId: $productId, quantity: 2 }],
    shippingAddress: {
      street: "123 Test Street",
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
ORDER_STATUS=$(jq -r '.data.status' "$BODY_FILE")
echo "  order_id     = $ORDER_ID"
echo "  order_status = $ORDER_STATUS"

if [[ "$ORDER_STATUS" == "PENDING" ]]; then
  pass "Order created in PENDING state"
else
  fail "Order should be PENDING, got $ORDER_STATUS"
fi

# ── Step 7: Verify stock decreased ────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 7: Verify stock decreased by 2${RESET}"
status=$(request GET "$PRODUCT_SVC/api/v1/inventory/$PRODUCT_ID")
assert_status "Get updated stock" "200" "$status"

CURRENT_STOCK=$(jq -r '.data.stockQuantity' "$BODY_FILE")
EXPECTED_STOCK=$((INITIAL_STOCK - 2))
echo "  initial stock  = $INITIAL_STOCK"
echo "  current stock  = $CURRENT_STOCK"
echo "  expected stock = $EXPECTED_STOCK"

if [[ "$CURRENT_STOCK" -eq "$EXPECTED_STOCK" ]]; then
  pass "Stock decreased correctly ($INITIAL_STOCK → $CURRENT_STOCK)"
else
  fail "Stock mismatch: expected $EXPECTED_STOCK, got $CURRENT_STOCK"
fi

# ── Step 8: Verify order detail ────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 8: Verify order detail${RESET}"
status=$(request GET "$ORDER_SVC/api/v1/orders/$ORDER_ID" \
  -H "X-User-Id: $USER_ID")
assert_status "Get order detail" "200" "$status"

FETCHED_STATUS=$(jq -r '.data.status' "$BODY_FILE")
FETCHED_ITEMS=$(jq '.data.items | length' "$BODY_FILE")
echo "  order status = $FETCHED_STATUS"
echo "  order items  = $FETCHED_ITEMS"

if [[ "$FETCHED_ITEMS" -ge 1 ]]; then
  pass "Order detail has items"
else
  fail "Order detail missing items"
fi

# ── Step 9: List user orders ───────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 9: List user orders (paginated)${RESET}"
status=$(request GET "$ORDER_SVC/api/v1/orders" \
  -H "X-User-Id: $USER_ID")
assert_status "List orders" "200" "$status"

ORDER_COUNT=$(jq '.data | length' "$BODY_FILE")
echo "  total orders returned = $ORDER_COUNT"
if [[ "$ORDER_COUNT" -ge 1 ]]; then
  pass "Order appears in user order list"
else
  fail "No orders found in order list"
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
  echo -e "  Failed: $FAIL"
  echo ""
  echo -e "${GREEN}All checks passed. End-to-end flow is working.${RESET}"
fi
