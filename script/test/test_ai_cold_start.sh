#!/usr/bin/env bash
# Phase 2.B — AI service cold-start test.
# Restarts the ai-service container, polls /health/ready every 250ms until 200,
# records elapsed time.  Target: testing_target.md §4.C — < 15s.
#
# Usage: bash test_ai_cold_start.sh [output_json]

set -uo pipefail
OUT="${1:-script/k6/results/ai_cold_start.json}"
mkdir -p "$(dirname "$OUT")"

TIMEOUT_S="${TIMEOUT_S:-60}"

echo "→ restarting ai-service..."
docker compose restart ai-service >/dev/null

t0_ms=$(python3 -c 'import time; print(int(time.time()*1000))')

# Check via product-service network (or expose port). ai-service is internal-only,
# so we poll through `docker exec` against the in-container endpoint.
echo "→ polling /health/ready..."
ELAPSED_MS=-1
while true; do
  if docker exec ecommerce-ai-service \
       python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://localhost:9000/health/ready').status==200 else 1)" \
       >/dev/null 2>&1; then
    t1_ms=$(python3 -c 'import time; print(int(time.time()*1000))')
    ELAPSED_MS=$((t1_ms - t0_ms))
    break
  fi
  t1_ms=$(python3 -c 'import time; print(int(time.time()*1000))')
  if (( (t1_ms - t0_ms) > TIMEOUT_S * 1000 )); then
    ELAPSED_MS=$((t1_ms - t0_ms))
    echo "  timeout ($TIMEOUT_S s)"
    break
  fi
  sleep 0.25
done

STATUS=$([[ "$ELAPSED_MS" -ge 0 && "$ELAPSED_MS" -lt 15000 ]] && echo "PASS" || echo "FAIL")
cat > "$OUT" <<EOF
{
  "cold_start_ms": $ELAPSED_MS,
  "target_ms": 15000,
  "status": "$STATUS"
}
EOF
echo "→ cold_start_ms=$ELAPSED_MS  status=$STATUS"
[[ "$STATUS" == "PASS" ]] && exit 0 || exit 1
