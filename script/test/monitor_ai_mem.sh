#!/usr/bin/env bash
# Samples ai-service memory usage (docker stats) every 1s.
# Usage:  bash monitor_ai_mem.sh <output_csv>

set -uo pipefail
OUT="${1:-script/k6/results/monitors/ai_mem.csv}"
mkdir -p "$(dirname "$OUT")"

echo "timestamp,mem_usage,mem_limit,mem_pct" > "$OUT"

trap 'exit 0' INT TERM

while true; do
  ts=$(date -u +%s)
  line=$(docker stats ecommerce-ai-service --no-stream --format '{{.MemUsage}},{{.MemPerc}}' 2>/dev/null)
  if [[ -n "$line" ]]; then
    usage="${line%%/*}"
    rest="${line#*/}"
    limit="${rest%%,*}"
    pct="${rest##*,}"
    echo "${ts},${usage// /},${limit// /},${pct}" >> "$OUT"
  fi
  sleep 1
done
