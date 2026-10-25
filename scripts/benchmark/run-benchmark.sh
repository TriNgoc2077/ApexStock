#!/usr/bin/env bash
set -euo pipefail

# Decision Gate D0.11: Enforce sequential execution
SCENARIO="${1:-smoke}"
RUNS="${2:-3}"

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT_DIR"

RUN_ID="run-$(date +%Y%m%d-%H%M%S)"
OUTPUT_DIR="loadtests/results/$RUN_ID"
mkdir -p "$OUTPUT_DIR"

echo "=========================================================="
echo " ApexStock Sequential Benchmark: $SCENARIO ($RUNS runs)"
echo " Run ID: $RUN_ID"
echo "=========================================================="

# Record Host & Git Metadata (Measurement Provenance)
cat <<EOF > "$OUTPUT_DIR/metadata.json"
{
  "run_id": "$RUN_ID",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "scenario": "$SCENARIO",
  "git_commit": "$(git rev-parse HEAD)",
  "os": "$(uname -srm)",
  "cpu_model": "$(lscpu | grep 'Model name' | cut -d: -f2 | xargs || echo 'Unknown')",
  "total_ram_kb": $(grep MemTotal /proc/meminfo | awk '{print $2}' || echo 0)
}
EOF

run_arch() {
  local arch="$1"
  local url="$2"
  local dsn="$3"
  echo "----------------------------------------------------------"
  echo ">>> [BENCHMARK] Running $arch on $url"
  echo "----------------------------------------------------------"

  for i in $(seq 1 "$RUNS"); do
    echo "[Run $i/$RUNS] Resetting & Seeding Database..."
    ./scripts/reset-data.sh
    ./scripts/seed-data.sh
    if [ "$arch" = "optimized" ]; then
      ./scripts/warmup-redis.sh
    fi

    echo "[Run $i/$RUNS] Cooldown 10s for thermal parity..."
    sleep 10

    echo "[Run $i/$RUNS] Executing k6..."
    k6 run \
      -e TARGET_URL="$url" \
      --summary-export "$OUTPUT_DIR/${arch}_run${i}_summary.json" \
      "loadtests/scenarios/${SCENARIO}.js"

    echo "[Run $i/$RUNS] Running Invariant Verifier..."
    BASELINE_DSN="$dsn" go test -v ./test/system -run TestInvariantVerifier_StockEquation_Calculation
  done
}

# 1. Run Baseline Sequentially
run_arch "baseline" "http://localhost:8081" "postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"

# Cooldown between architectures (D0.11)
echo "=========================================================="
echo " Architecture Intermission: 30s Cooldown for CPU Parity"
echo "=========================================================="
sleep 30

# 2. Run Optimized Sequentially
run_arch "optimized" "http://localhost:8082" "postgres://apexstock:apexstock@localhost:5432/apexstock_optimized?sslmode=disable"

echo "=========================================================="
echo " Benchmark Complete! Results saved to $OUTPUT_DIR"
echo "=========================================================="
