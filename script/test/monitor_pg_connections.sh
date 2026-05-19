#!/usr/bin/env bash
# Samples pg_stat_activity every second into a CSV. Exits when killed.
# Usage:  bash monitor_pg_connections.sh <output_csv>
#   e.g.  bash script/test/monitor_pg_connections.sh script/k6/results/monitors/pg.csv

set -uo pipefail
OUT="${1:-script/k6/results/monitors/pg.csv}"
mkdir -p "$(dirname "$OUT")"

DB_USER="${DB_USER:-postgres}"

echo "timestamp,datname,connections" > "$OUT"

trap 'exit 0' INT TERM

while true; do
  ts=$(date -u +%s)
  docker compose exec -T postgres psql -U "$DB_USER" -d postgres -At -c \
    "SELECT datname || ',' || count(*) FROM pg_stat_activity WHERE datname IS NOT NULL GROUP BY datname" 2>/dev/null \
    | while IFS= read -r line; do
        echo "${ts},${line}" >> "$OUT"
      done
  sleep 1
done
