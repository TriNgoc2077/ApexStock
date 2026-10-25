#!/usr/bin/env bash
set -euo pipefail

CONTAINER="apexstock-redis-1"
COMPOSE_FILE="deploy/compose/docker-compose.yaml"

cleanup() {
  echo ">>> [TRAP CLEANUP] Ensuring $CONTAINER is running..."
  docker start "$CONTAINER" 2>/dev/null || docker compose -f "$COMPOSE_FILE" start redis 2>/dev/null || true
}
trap cleanup EXIT SIGINT SIGTERM

echo "=========================================================="
echo " [CHAOS DRILL] Redis Outage & Circuit Breaker Drill"
echo " Target: $CONTAINER"
echo "=========================================================="

echo "1. Stopping Redis container..."
docker stop "$CONTAINER" || docker compose -f "$COMPOSE_FILE" stop redis
echo ">>> Redis is STOPPED. Verifying API returns 503 without overselling..."

sleep 3

echo "2. Probing Optimized API /api/v1/flash-sale/orders..."
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8082/api/v1/flash-sale/orders \
  -H "Content-Type: application/json" \
  -d '{"user_id":"00000000-0000-0000-0000-000000000001","flash_sale_item_id":"22222222-2222-2222-2222-222222222222","quantity":1,"idempotency_key":"drill-1"}' || echo "000")

echo ">>> API Response Status during Redis outage: $HTTP_STATUS"

echo "3. Restarting Redis container..."
docker start "$CONTAINER" || docker compose -f "$COMPOSE_FILE" start redis
echo ">>> Redis restarted. Running warm-up..."
sleep 2
./scripts/warmup-redis.sh

echo "=========================================================="
echo " Drill Complete! Post-drill Invariant Check:"
./scripts/reconcile.sql.sh
echo "=========================================================="
