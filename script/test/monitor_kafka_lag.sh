#!/usr/bin/env bash
# Samples Kafka consumer-group lag for the payment-service group every 5s.
# Usage:  bash monitor_kafka_lag.sh <output_csv>

set -uo pipefail
OUT="${1:-script/k6/results/monitors/kafka_lag.csv}"
GROUP="${GROUP:-payment-service}"
mkdir -p "$(dirname "$OUT")"

echo "timestamp,topic,partition,lag" > "$OUT"

trap 'exit 0' INT TERM

while true; do
  ts=$(date -u +%s)
  # `kafka-consumer-groups --describe` output columns: GROUP TOPIC PARTITION CURRENT-OFFSET LOG-END-OFFSET LAG CONSUMER-ID HOST CLIENT-ID
  docker exec ecommerce-kafka kafka-consumer-groups \
    --bootstrap-server kafka:29092 --describe --group "$GROUP" 2>/dev/null \
    | awk -v ts="$ts" 'NR>1 && $2!="" {print ts","$2","$3","$6}' >> "$OUT"
  sleep 5
done
