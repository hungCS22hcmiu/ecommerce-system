#!/usr/bin/env bash
# After a load run, greps Java-service logs for HikariCP leak warnings.
# Writes a one-line summary "leak_count=N" plus the matching lines.
# Usage:  bash monitor_hikari.sh <since> <output_log>

set -uo pipefail
SINCE="${1:-10m}"
OUT="${2:-script/k6/results/monitors/hikari.log}"
mkdir -p "$(dirname "$OUT")"

: > "$OUT"
TOTAL=0
for svc in product-service order-service; do
  count=$(docker compose logs --since "$SINCE" "$svc" 2>/dev/null \
          | grep -c "Connection leak\|leakDetectionThreshold" || true)
  echo "$svc: $count leak warnings" >> "$OUT"
  if [[ "$count" -gt 0 ]]; then
    docker compose logs --since "$SINCE" "$svc" 2>/dev/null \
      | grep "Connection leak\|leakDetectionThreshold" >> "$OUT"
  fi
  TOTAL=$((TOTAL + count))
done
echo "leak_count=$TOTAL" >> "$OUT"
