#!/usr/bin/env bash
# Load test: create ORDER_COUNT orders, then verify 0 PENDING orders and 0 DLQ messages.
# Requires the full stack (all services + kafka + postgres) to be running.
#
# Usage:
#   bash script/loadtest-orders.sh
#
# Overrides:
#   ORDER_COUNT=50 CONCURRENCY=5 WAIT_SECONDS=60 bash script/loadtest-orders.sh

set -euo pipefail

USER_SVC="${USER_SVC:-http://localhost:8001}"
ORDER_SVC="${ORDER_SVC:-http://localhost:8082}"
ORDER_COUNT="${ORDER_COUNT:-100}"
CONCURRENCY="${CONCURRENCY:-10}"
WAIT_SECONDS="${WAIT_SECONDS:-30}"
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-postgres}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
RESET='\033[0m'

pass() { echo -e "  ${GREEN}✓${RESET} $1"; }
fail() { echo -e "  ${RED}✗${RESET} $1"; FAILED=1; }
info() { echo -e "  ${YELLOW}→${RESET} $1"; }

FAILED=0
BODY_FILE=$(mktemp)
trap 'rm -f "$BODY_FILE"' EXIT

echo ""
echo -e "${BOLD}=== Load Test: ${ORDER_COUNT} orders, ${CONCURRENCY} concurrent ===${RESET}"
echo ""

# ── Step 1: Login ──────────────────────────────────────────────────────────────
echo -e "${BOLD}Step 1: Login${RESET}"
http_code=$(curl -s -o "$BODY_FILE" -w "%{http_code}" -X POST "$USER_SVC/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"customer@example.com","password":"Customer@123"}')

if [[ "$http_code" != "200" ]]; then
  echo -e "${RED}Fatal: login failed (HTTP $http_code). Is user-service running?${RESET}"
  exit 1
fi

USER_ID=$(jq -r '.data.user.id' "$BODY_FILE")
info "user_id = $USER_ID"

# ── Step 2: Find a product with stock ─────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 2: Find a product${RESET}"
PRODUCT_ID=""
for cid in 1 2 3 4 5 6 7 8 9 10; do
  pcode=$(curl -s -o "$BODY_FILE" -w "%{http_code}" "http://localhost:8081/api/v1/products/$cid")
  if [[ "$pcode" == "200" ]]; then
    pname=$(jq -r '.data.name // empty' "$BODY_FILE")
    if [[ -n "$pname" ]]; then
      PRODUCT_ID="$cid"
      info "product_id=$cid name=$pname"
      break
    fi
  fi
done

if [[ -z "$PRODUCT_ID" ]]; then
  echo -e "${RED}Fatal: no product found. Ensure seed data is applied.${RESET}"
  exit 1
fi

# ── Step 3: Create ORDER_COUNT orders in parallel ─────────────────────────────
echo ""
echo -e "${BOLD}Step 3: Creating ${ORDER_COUNT} orders (${CONCURRENCY} concurrent)...${RESET}"
START_TIME=$(date +%s)

# Export variables so the xargs subshell can access them.
export ORDER_SVC USER_ID PRODUCT_ID

seq 1 "$ORDER_COUNT" | xargs -P "$CONCURRENCY" -I{} bash -c '
  CART_UUID=$(python3 -c "import uuid; print(uuid.uuid4())" 2>/dev/null || uuidgen | tr "[:upper:]" "[:lower:]")
  curl -sf -o /dev/null -X POST "$ORDER_SVC/api/v1/orders" \
    -H "Content-Type: application/json" \
    -H "X-User-Id: $USER_ID" \
    -d "{
      \"cartId\":\"$CART_UUID\",
      \"items\":[{\"productId\":$PRODUCT_ID,\"quantity\":1}],
      \"shippingAddress\":{\"street\":\"Load Test Ave\",\"city\":\"HCMC\",\"state\":\"HCM\",\"country\":\"Vietnam\",\"zipCode\":\"70000\"}
    }" || true
'

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))
info "created ${ORDER_COUNT} orders in ${ELAPSED}s"

# ── Step 4: Wait for saga to complete ─────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 4: Waiting ${WAIT_SECONDS}s for Kafka saga...${RESET}"
sleep "$WAIT_SECONDS"

# ── Step 5: SQL assertions ─────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 5: Checking database state${RESET}"

PAYMENTS_RESULT=$(PGPASSWORD="$POSTGRES_PASSWORD" psql \
  -h "$POSTGRES_HOST" -U postgres -d ecommerce_payments -t -A \
  -c "SELECT
        SUM(CASE WHEN status='COMPLETED' THEN 1 ELSE 0 END),
        SUM(CASE WHEN status='FAILED'    THEN 1 ELSE 0 END),
        SUM(CASE WHEN status='PENDING'   THEN 1 ELSE 0 END),
        COUNT(*)
      FROM payments;" 2>/dev/null)

IFS='|' read -r COMPLETED FAILED PENDING TOTAL <<< "$PAYMENTS_RESULT"
info "payments — completed=$COMPLETED  failed=$FAILED  pending=$PENDING  total=$TOTAL"

if [[ "$PENDING" == "0" ]] || [[ -z "$PENDING" ]]; then
  pass "No PENDING payments (all saga paths terminated)"
else
  fail "Found PENDING payments: $PENDING — saga did not complete in time"
fi

ORDERS_RESULT=$(PGPASSWORD="$POSTGRES_PASSWORD" psql \
  -h "$POSTGRES_HOST" -U postgres -d ecommerce_orders -t -A \
  -c "SELECT
        SUM(CASE WHEN status='CONFIRMED'  THEN 1 ELSE 0 END),
        SUM(CASE WHEN status='CANCELLED'  THEN 1 ELSE 0 END),
        SUM(CASE WHEN status='PENDING'    THEN 1 ELSE 0 END)
      FROM orders;" 2>/dev/null)

IFS='|' read -r CONFIRMED CANCELLED ORD_PENDING <<< "$ORDERS_RESULT"
info "orders   — confirmed=$CONFIRMED  cancelled=$CANCELLED  pending=$ORD_PENDING"

if [[ "$ORD_PENDING" == "0" ]] || [[ -z "$ORD_PENDING" ]]; then
  pass "No PENDING orders (all orders reached terminal state)"
else
  fail "Found PENDING orders: $ORD_PENDING"
fi

# ── Step 6: DLQ check ─────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 6: Checking DLQ depth${RESET}"
DLQ_MSG=$(docker exec ecommerce-kafka kafka-console-consumer \
  --bootstrap-server localhost:29092 --topic payments.dlq \
  --from-beginning --timeout-ms 3000 --max-messages 1 2>/dev/null || true)

if [[ -z "$DLQ_MSG" ]]; then
  pass "payments.dlq is empty"
else
  fail "payments.dlq is NOT empty — unexpected DLQ message found"
  echo "  DLQ sample: ${DLQ_MSG:0:200}"
fi

# ── Step 7: Lag check ─────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}Step 7: Consumer lag snapshot${RESET}"
docker exec ecommerce-kafka kafka-consumer-groups \
  --bootstrap-server localhost:29092 \
  --group payment-service --describe 2>/dev/null \
  | grep -v "^$" | head -10 || info "(kafka-consumer-groups not available)"

# ── Summary ────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}=== Results ===${RESET}"
if [[ "$FAILED" -eq 0 ]]; then
  echo -e "${GREEN}Load test passed: ${ORDER_COUNT} orders, 0 PENDING, 0 DLQ messages.${RESET}"
else
  echo -e "${RED}Load test FAILED — see assertions above.${RESET}"
  exit 1
fi
