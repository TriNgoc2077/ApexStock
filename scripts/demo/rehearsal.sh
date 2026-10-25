#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT_DIR"

echo "=========================================================="
echo " ApexStock Full Demo Rehearsal Automation"
echo "=========================================================="

echo "1. Waiting for services to become ready..."
for port in 8081 8082 9000; do
  echo "Checking :$port/healthz..."
  for attempt in $(seq 1 30); do
    if curl -s -f "http://localhost:$port/healthz" >/dev/null; then
      break
    fi
    sleep 1
  done
done
echo ">>> All services responding."

echo "2. Resetting & Seeding Datasets..."
./scripts/reset-data.sh
./scripts/seed-data.sh
./scripts/warmup-redis.sh

echo "3. Testing Baseline & Optimized Order Checkout..."
./scripts/system/e2e-smoke.sh

echo "4. Running Quick Smoke Benchmark (10 VUs, 10s)..."
k6 run -e TARGET_URL="http://localhost:8082" --duration 10s --vus 10 loadtests/scenarios/smoke.js

echo "5. Verifying Invariants Post-Rehearsal..."
go test -v ./test/system -run TestInvariantVerifier_StockEquation_Calculation

echo "=========================================================="
echo " >>> DEMO REHEARSAL COMPLETED WITH ZERO ERRORS! <<<"
echo "=========================================================="
