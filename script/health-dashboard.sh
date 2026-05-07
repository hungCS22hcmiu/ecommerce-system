#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE_URL:-http://localhost}"

declare -a SERVICES=(
    "nginx|${BASE}/health/live"
    "user-service|${BASE}/api/v1/auth/health/live"
    "product-service|${BASE}/api/v1/products/health/live"
    "cart-service|${BASE}/api/v1/cart/health/live"
    "order-service|${BASE}/api/v1/orders/health/live"
    "payment-service|${BASE}/health/ready"
)

echo "── Health Dashboard ──────────────────────── $(date '+%Y-%m-%dT%H:%M:%S')"
printf "%-20s %-6s %s\n" "SERVICE" "STATUS" "URL"
printf "%-20s %-6s %s\n" "-------" "------" "---"

all_ok=true
for entry in "${SERVICES[@]}"; do
    name="${entry%%|*}"
    url="${entry##*|}"
    http_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "$url" 2>/dev/null || echo "000")
    if [[ "$http_code" == "200" ]]; then
        printf "  ✓ %-18s %-6s %s\n" "$name" "$http_code" "$url"
    else
        printf "  ✗ %-18s %-6s %s\n" "$name" "$http_code" "$url"
        all_ok=false
    fi
done

echo "─────────────────────────────────────────────────────"
if $all_ok; then
    echo "  All services healthy"
else
    echo "  One or more services degraded"
    exit 1
fi
