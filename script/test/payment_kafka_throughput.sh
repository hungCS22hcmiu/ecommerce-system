#!/usr/bin/env bash
# Phase 2.A.5 — Kafka consumer throughput test.
# Produces N orders.created messages to Kafka directly, waits for payment-service
# consumer to drain, records messages-per-second.
#
# Target: testing_target.md §1 — ≥ 200 msg/s, P95 handler latency < 100ms.
#
# Usage: bash payment_kafka_throughput.sh [N] [output_dir]

set -uo pipefail
NUM="${1:-10000}"
OUT_DIR="${2:-script/k6/results}"
mkdir -p "$OUT_DIR"

GROUP="${GROUP:-payment-service}"
TOPIC="${TOPIC:-orders.created}"

echo "→ generating $NUM payloads..."
python3 "$(dirname "$0")/gen_kafka_payloads.py" "$NUM" > /tmp/orders_payloads.jsonl
echo "  payload bytes: $(wc -c < /tmp/orders_payloads.jsonl)"

# baseline lag before producing
BASELINE_LAG=$(docker exec ecommerce-kafka kafka-consumer-groups \
  --bootstrap-server kafka:29092 --describe --group "$GROUP" 2>/dev/null \
  | awk 'NR>1 && $2!="" {sum+=$6} END {print sum+0}')
echo "  baseline lag: $BASELINE_LAG"

t0=$(date +%s)
echo "→ producing to $TOPIC..."
docker exec -i ecommerce-kafka kafka-console-producer \
  --bootstrap-server kafka:29092 --topic "$TOPIC" \
  --producer-property "compression.type=snappy" < /tmp/orders_payloads.jsonl
t1=$(date +%s)
produce_secs=$(( t1 - t0 ))
echo "  produced in ${produce_secs}s"

# Wait for consumer to drain back to baseline
echo "→ waiting for consumer drain (lag → $BASELINE_LAG)..."
DRAIN_START=$(date +%s)
TIMEOUT_S="${TIMEOUT_S:-300}"
while true; do
  lag=$(docker exec ecommerce-kafka kafka-consumer-groups \
    --bootstrap-server kafka:29092 --describe --group "$GROUP" 2>/dev/null \
    | awk 'NR>1 && $2!="" {sum+=$6} END {print sum+0}')
  elapsed=$(( $(date +%s) - DRAIN_START ))
  echo "  t+${elapsed}s lag=$lag"
  if [[ "$lag" -le "$BASELINE_LAG" ]]; then break; fi
  if [[ "$elapsed" -ge "$TIMEOUT_S" ]]; then
    echo "  timeout (still lag=$lag after ${TIMEOUT_S}s)"
    break
  fi
  sleep 2
done
t2=$(date +%s)
total_secs=$(( t2 - t0 ))
drain_secs=$(( t2 - t1 ))

throughput=$(( NUM / (total_secs > 0 ? total_secs : 1) ))

OUT="$OUT_DIR/kafka_throughput.json"
cat > "$OUT" <<EOF
{
  "num_messages": $NUM,
  "produce_secs": $produce_secs,
  "drain_secs": $drain_secs,
  "total_secs": $total_secs,
  "throughput_msg_per_s": $throughput,
  "baseline_lag": $BASELINE_LAG
}
EOF
echo "→ wrote $OUT  (throughput: $throughput msg/s)"
