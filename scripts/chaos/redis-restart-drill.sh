#!/usr/bin/env bash
set -euo pipefail

CONTAINER=$(docker ps --format '{{.Names}}' | grep 'redis' | head -n 1 || echo "")
if [ -z "$CONTAINER" ]; then
  CONTAINER="compose-redis-1"
fi

cleanup() {
  echo ">>> [TRAP CLEANUP] Ensuring $CONTAINER is running..."
  docker start "$CONTAINER" 2>/dev/null || true
}
trap cleanup EXIT SIGINT SIGTERM

echo "=========================================================="
echo " [CHAOS DRILL] Redis Outage & Circuit Breaker Drill"
echo " Target: $CONTAINER"
echo "=========================================================="

echo "1. Stopping Redis container..."
docker stop "$CONTAINER"
echo ">>> Redis is STOPPED. Verifying API returns 503 without overselling..."

sleep 2

echo "2. Probing Optimized API /api/v1/flash-sale/orders..."
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8082/api/v1/flash-sale/orders \
  -H "Content-Type: application/json" \
  -d '{"user_id":"33333333-3333-3333-3333-000000000001","flash_sale_item_id":"22222222-2222-2222-2222-222222222222","quantity":1,"idempotency_key":"drill-1"}' || echo "000")

echo ">>> API Response Status during Redis outage: $HTTP_STATUS"

if [ "$HTTP_STATUS" = "503" ]; then
  echo ">>> [SUCCESS] Circuit Breaker activated: Returned 503 Service Unavailable safely!"
else
  echo ">>> [WARNING] Unexpected status: $HTTP_STATUS"
fi

echo "3. Restarting Redis container..."
docker start "$CONTAINER"
echo ">>> Redis restarted. Running warm-up..."
sleep 2
./scripts/warmup-redis.sh

echo "4. Probing Optimized API post-recovery..."
HTTP_STATUS_RECOVERED=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8082/api/v1/flash-sale/orders \
  -H "Content-Type: application/json" \
  -d '{"user_id":"33333333-3333-3333-3333-000000000002","flash_sale_item_id":"22222222-2222-2222-2222-222222222222","quantity":1,"idempotency_key":"drill-2"}' || echo "000")
echo ">>> API Response Status after Redis recovered: $HTTP_STATUS_RECOVERED"

echo "=========================================================="
echo " Drill Complete! Post-drill Invariant Check:"
./scripts/reconcile.sql.sh
echo "=========================================================="
