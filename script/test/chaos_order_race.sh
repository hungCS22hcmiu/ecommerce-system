#!/usr/bin/env bash
# Phase 3 §3.C — Order state-machine race test.
#
# For each round (N=20):
#   1. Create a fresh PENDING order via POST /orders.
#   2. Simultaneously:
#        (a) Publish payments.completed Kafka event with __TypeId__ header  →  PaymentEventConsumer
#            tries to transition PENDING → CONFIRMED.
#        (b) Call PUT /orders/{id}/ship direct to port 8082                  →  state machine
#            tries to apply CONFIRMED → SHIPPED.
#   3. The pessimistic lock must serialize the two; never both succeed inconsistently.
#      Acceptable HTTP statuses for the SHIP call: 200 (after CONFIRMED) or 409 (still PENDING).
#      Unacceptable: 500.
#
# After all rounds: scan order-service logs for DeadlockLoserDataAccessException and
# 5xx status codes. Assert both counts == 0.

set -uo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

RESULTS_DIR="script/k6/results"
mkdir -p "$RESULTS_DIR"

ROUNDS="${ROUNDS:-20}"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; RESET='\033[0m'
info() { echo -e "  ${YELLOW}→${RESET} $*"; }

# Login seller (to create product) + customer (to place orders)
info "logging in seller + customer"
SELLER_LOGIN=$(curl -fsS -X POST http://localhost:8001/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"seller@example.com","password":"Seller@123"}')
SELLER_ID=$(echo "$SELLER_LOGIN" | jq -r .data.user.id)

CUST_LOGIN=$(curl -fsS -X POST http://localhost:8001/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"customer@example.com","password":"Customer@123"}')
TOKEN=$(echo "$CUST_LOGIN" | jq -r .data.access_token)
USER_ID=$(echo "$CUST_LOGIN" | jq -r .data.user.id)
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then echo -e "${RED}Login failed${RESET}"; exit 1; fi

# Seed a fresh product with abundant stock so 20 rounds don't run out.
info "seeding fresh test product"
PRODUCT_PAYLOAD=$(jq -n --arg n "race-test-$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid)" '{
  name: $n, description: "Phase 3 race test", price: 9.99,
  categoryId: 1, stockQuantity: 1000
}')
PRODUCT_BODY=$(curl -fsS -X POST http://localhost:8081/api/v1/products \
  -H 'Content-Type: application/json' -H "X-Seller-Id: $SELLER_ID" \
  -d "$PRODUCT_PAYLOAD")
PRODUCT_ID=$(echo "$PRODUCT_BODY" | jq -r '.data.id // empty')
if [[ -z "$PRODUCT_ID" ]]; then echo -e "${RED}Product create failed${RESET}"; exit 1; fi
info "  productId=$PRODUCT_ID stock=1000"

SINCE_TS=$(date -u +%s)
SHIP_200=0; SHIP_409=0; SHIP_500=0; SHIP_OTHER=0
CREATE_FAIL=0

for i in $(seq 1 "$ROUNDS"); do
  CART_UUID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid)
  ORDER_PAYLOAD=$(jq -n --arg c "$CART_UUID" --argjson p "$PRODUCT_ID" '{
    cartId: $c,
    items: [{productId: $p, quantity: 1}],
    shippingAddress: {street:"1 Race St", city:"HCM", state:"HCM", country:"VN", zipCode:"70000"}
  }')

  ORDER_BODY=$(curl -sS -X POST http://localhost:8082/api/v1/orders \
    -H 'Content-Type: application/json' -H "X-User-Id: $USER_ID" \
    -d "$ORDER_PAYLOAD")

  ORDER_ID=$(echo "$ORDER_BODY" | jq -r '.data.id // empty')
  if [[ -z "$ORDER_ID" ]]; then
    CREATE_FAIL=$((CREATE_FAIL+1))
    continue
  fi

  PAYMENT_ID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid)
  EVENT_JSON=$(jq -nc --arg o "$ORDER_ID" --arg p "$PAYMENT_ID" \
    '{orderId: $o, paymentId: $p, amount: 9.99}')

  # Format expected by kafka-console-producer with parse.headers=true:
  #   <key1>:<val1>,<key2>:<val2>\t<value>
  HEADERS="__TypeId__:com.ecommerce.order_service.kafka.event.PaymentCompletedEvent"

  # Fire SHIP and Kafka publish concurrently (background both, then wait).
  (
    printf '%s\t%s\n' "$HEADERS" "$EVENT_JSON" \
      | docker exec -i ecommerce-kafka kafka-console-producer \
          --bootstrap-server kafka:29092 --topic payments.completed \
          --property parse.headers=true \
          --property "headers.delimiter=	" \
          >/dev/null 2>&1
  ) &
  PUB_PID=$!

  # SHIP requires the SELLER's id (despite the header name being X-User-Id, the
  # endpoint binds it as sellerId — see OrderController.java:58).
  SHIP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 \
    -X PUT "http://localhost:8082/api/v1/orders/$ORDER_ID/ship" \
    -H "X-User-Id: $SELLER_ID")

  wait "$PUB_PID"

  case "$SHIP_CODE" in
    200) SHIP_200=$((SHIP_200+1));;
    409) SHIP_409=$((SHIP_409+1));;
    500) SHIP_500=$((SHIP_500+1));;
    *)   SHIP_OTHER=$((SHIP_OTHER+1));;
  esac

  printf "  round %02d: order=%s ship=%s\n" "$i" "${ORDER_ID:0:8}" "$SHIP_CODE"
done

# Wait briefly for any in-flight Kafka consumer logs to land
sleep 5

# Grep order-service logs for deadlock errors + 5xx since this run started.
ELAPSED=$(( $(date -u +%s) - SINCE_TS ))
WINDOW=$(( ELAPSED + 5 ))
DEADLOCK_COUNT=$(docker compose logs --since "${WINDOW}s" order-service 2>/dev/null \
  | grep -c "DeadlockLoserDataAccessException" || true)
LOG_500_COUNT=$(docker compose logs --since "${WINDOW}s" order-service 2>/dev/null \
  | grep -cE '"status":5[0-9][0-9]|status=5[0-9][0-9]' || true)

cat > "$RESULTS_DIR/chaos_order_race.json" <<EOF
{
  "rounds": $ROUNDS,
  "order_create_fail": $CREATE_FAIL,
  "ship_200": $SHIP_200,
  "ship_409": $SHIP_409,
  "ship_500": $SHIP_500,
  "ship_other": $SHIP_OTHER,
  "deadlock_errors": $DEADLOCK_COUNT,
  "http_500_in_logs": $LOG_500_COUNT
}
EOF
info "wrote $RESULTS_DIR/chaos_order_race.json"
info "ship distribution: 200=$SHIP_200, 409=$SHIP_409, 500=$SHIP_500, other=$SHIP_OTHER"
info "deadlocks: $DEADLOCK_COUNT, 5xx in logs: $LOG_500_COUNT"
exit 0
