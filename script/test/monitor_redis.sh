#!/usr/bin/env bash
# Samples Redis ping latency for a fixed duration. Writes raw output + a summary.
# Usage:  bash monitor_redis.sh <duration_seconds> <output_log>

set -uo pipefail
DURATION="${1:-60}"
OUT="${2:-script/k6/results/monitors/redis.log}"
mkdir -p "$(dirname "$OUT")"

# redis-cli --latency reports min/max/avg every second; -i 1 = sample interval, -t = total time.
docker exec ecommerce-redis redis-cli --latency-history -i 1 > "$OUT" 2>&1 &
PID=$!
sleep "$DURATION"
kill "$PID" 2>/dev/null || true
wait "$PID" 2>/dev/null || true

# Summary: peak max latency observed during the window (in ms — redis-cli reports ms)
PEAK=$(awk '/max:/ {for(i=1;i<=NF;i++) if ($i ~ /^max:/) print $(i+1)}' "$OUT" | sort -n | tail -1)
echo "redis_peak_max_ms=${PEAK:-n/a}" >> "$OUT"
