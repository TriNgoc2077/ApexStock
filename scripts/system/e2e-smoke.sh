#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT_DIR"

echo "=========================================================="
echo " ApexStock Full System End-to-End Smoke Test"
echo "=========================================================="

# 1. Check service health
echo "1. Checking Health Endpoints..."
curl -s -f http://localhost:8081/healthz >/dev/null || { echo "Baseline :8081 not healthy" >&2; exit 1; }
curl -s -f http://localhost:8082/healthz >/dev/null || { echo "Optimized :8082 not healthy" >&2; exit 1; }
curl -s -f http://localhost:9000/healthz >/dev/null || { echo "War Room :9000 not healthy" >&2; exit 1; }
echo ">>> All service health endpoints OK."

# 2. Reset and Seed Data
echo "2. Resetting & Seeding Datasets..."
./scripts/reset-data.sh
./scripts/seed-data.sh
./scripts/warmup-redis.sh

# 3. Create Orders across both architectures
echo "3. Testing Flash Sale Order Creation..."
ORDER_BASE=$(curl -s -X POST http://localhost:8081/api/v1/flash-sale/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: e2e-base-1" \
  -d '{"user_id":"33333333-3333-3333-3333-000000000001","flash_sale_item_id":"22222222-2222-2222-2222-222222222222","quantity":1}')

ORDER_OPT=$(curl -s -X POST http://localhost:8082/api/v1/flash-sale/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: e2e-opt-1" \
  -d '{"user_id":"33333333-3333-3333-3333-000000000002","flash_sale_item_id":"22222222-2222-2222-2222-222222222222","quantity":1}')

echo ">>> Baseline Order: $(echo "$ORDER_BASE" | jq -r .order_id)"
echo ">>> Optimized Order: $(echo "$ORDER_OPT" | jq -r .order_id)"

# 4. Verify Prometheus Metrics Exposition
echo "4. Checking Prometheus /metrics endpoints..."
curl -s http://localhost:8081/metrics | grep -q "http_requests_total" || { echo "Baseline metrics missing" >&2; exit 1; }
curl -s http://localhost:8082/metrics | grep -q "http_requests_total" || { echo "Optimized metrics missing" >&2; exit 1; }

# 5. Run Invariant Verifier
echo "5. Executing Invariant Verifier..."
go test -v ./test/system -run TestInvariantVerifier_StockEquation_Calculation

echo "=========================================================="
echo " >>> FULL SYSTEM E2E SMOKE TEST PASSED! <<<"
echo "=========================================================="
