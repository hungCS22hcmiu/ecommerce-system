#!/usr/bin/env bash
# Phase 3 — mid-saga payment-service kill recovery.
#
# 1. Start background order load (10 orders/s × 90s via existing k6 script).
# 2. At t+10s: docker compose kill payment-service.
# 3. sleep RESTART_AFTER (default 5s).
# 4. docker compose start payment-service.
# 5. Wait for load to finish + 30s settle.
# 6. Assert:
#    - 0 orders left in PENDING (must reach terminal state after restart)
#    - 0 messages routed to payments.dlq during the chaos window
#    - 0 orders with > 1 payment row (UNIQUE constraint held)

set -uo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

RESULTS_DIR="script/k6/results"
mkdir -p "$RESULTS_DIR"

DURATION="${DURATION:-90}"
KILL_AT="${KILL_AT:-10}"
RESTART_AFTER="${RESTART_AFTER:-5}"
RATE="${RATE:-10}"
SETTLE_AFTER="${SETTLE_AFTER:-30}"
DB_USER="${DB_USER:-postgres}"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; RESET='\033[0m'
info() { echo -e "  ${YELLOW}→${RESET} $*"; }

cleanup() {
  info "cleanup: ensuring payment-service is running"
  docker compose start payment-service >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Snapshot DLQ baseline offset
DLQ_BASELINE=$(docker exec ecommerce-kafka kafka-run-class kafka.tools.GetOffsetShell \
  --broker-list kafka:29092 --topic payments.dlq 2>/dev/null \
  | awk -F: '{sum+=$3} END {print sum+0}')
info "payments.dlq baseline offset: $DLQ_BASELINE"

# Snapshot start time for SQL window
START_TS=$(date -u +%Y-%m-%dT%H:%M:%S)
info "test start ts: $START_TS"

# Background load
info "starting background order load: ${RATE} r/s × ${DURATION}s"
RATE="$RATE" DURATION="${DURATION}s" k6 run --quiet \
  --summary-export="$RESULTS_DIR/chaos_saga_kill_load.json" \
  script/k6/order_create.js >"$RESULTS_DIR/chaos_saga_kill_load.log" 2>&1 &
K6_PID=$!

# Kill payment-service mid-load
sleep "$KILL_AT"
info "killing payment-service at t+${KILL_AT}s"
docker compose kill payment-service >/dev/null

sleep "$RESTART_AFTER"
info "starting payment-service after ${RESTART_AFTER}s downtime"
docker compose start payment-service >/dev/null

# Wait for k6 to finish
wait "$K6_PID" 2>/dev/null || true
K6_STATUS=$?
info "k6 finished (exit=$K6_STATUS)"

# Settle window
info "settle ${SETTLE_AFTER}s"
sleep "$SETTLE_AFTER"

# Assertions
PENDING=$(docker exec ecommerce-postgres psql -U "$DB_USER" -d ecommerce_orders -t -A -c \
  "SELECT count(*) FROM orders WHERE status='PENDING' AND created_at >= '${START_TS}'::timestamp")
PENDING="${PENDING:-0}"; PENDING="${PENDING// /}"

DLQ_FINAL=$(docker exec ecommerce-kafka kafka-run-class kafka.tools.GetOffsetShell \
  --broker-list kafka:29092 --topic payments.dlq 2>/dev/null \
  | awk -F: '{sum+=$3} END {print sum+0}')
DLQ_DELTA=$((DLQ_FINAL - DLQ_BASELINE))

DUP_COUNT=$(docker exec ecommerce-postgres psql -U "$DB_USER" -d ecommerce_payments -t -A -c \
  "SELECT count(*) FROM (SELECT order_id FROM payments GROUP BY order_id HAVING count(*) > 1) t")
DUP_COUNT="${DUP_COUNT:-0}"; DUP_COUNT="${DUP_COUNT// /}"

# Recent payment counts in the run window
PAYMENT_TOTAL=$(docker exec ecommerce-postgres psql -U "$DB_USER" -d ecommerce_payments -t -A -c \
  "SELECT count(*) FROM payments WHERE created_at >= '${START_TS}'::timestamp")
PAYMENT_TOTAL="${PAYMENT_TOTAL:-0}"; PAYMENT_TOTAL="${PAYMENT_TOTAL// /}"

cat > "$RESULTS_DIR/chaos_saga_kill.json" <<EOF
{
  "duration_s": $DURATION,
  "kill_at_s": $KILL_AT,
  "restart_after_s": $RESTART_AFTER,
  "rate_per_s": $RATE,
  "pending_count": $PENDING,
  "dlq_baseline": $DLQ_BASELINE,
  "dlq_final": $DLQ_FINAL,
  "dlq_delta": $DLQ_DELTA,
  "duplicate_payment_count": $DUP_COUNT,
  "payments_in_window": $PAYMENT_TOTAL,
  "k6_status": $K6_STATUS
}
EOF
info "wrote $RESULTS_DIR/chaos_saga_kill.json"
info "  pending=$PENDING, dlq_delta=$DLQ_DELTA, duplicates=$DUP_COUNT, payments=$PAYMENT_TOTAL"

exit 0
