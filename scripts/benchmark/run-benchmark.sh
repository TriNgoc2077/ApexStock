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
chmod 777 "$OUTPUT_DIR"

# Stock/limit sized to the load profile. sustainable-throughput sends ~59 x PEAK_RPS requests
# (default PEAK_RPS=2000 -> ~118000), so seed 60 x PEAK_RPS to avoid sold-out 409s distorting RPS.
# Override with BENCH_STOCK / BENCH_MAX_PER_USER. Other scenarios keep seed defaults.
PEAK_RPS="${PEAK_RPS:-2000}"
# Redis budget (512MB, noeviction) fits ~460k reservations (~2 keys/order), so cap stock at 350k by default.
DEFAULT_STOCK=$((PEAK_RPS * 60))
[ "$DEFAULT_STOCK" -gt 350000 ] && DEFAULT_STOCK=350000
if [ "$SCENARIO" = "sustainable-throughput" ]; then
  export INITIAL_STOCK="${BENCH_STOCK:-$DEFAULT_STOCK}"
  export MAX_PER_USER="${BENCH_MAX_PER_USER:-100000000}"
elif [ -n "${BENCH_STOCK:-}" ]; then
  export INITIAL_STOCK="$BENCH_STOCK"
  [ -n "${BENCH_MAX_PER_USER:-}" ] && export MAX_PER_USER="$BENCH_MAX_PER_USER"
fi

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
    set +e
    if command -v k6 >/dev/null 2>&1; then
      k6 run \
        -e TARGET_URL="$url" -e PEAK_RPS="$PEAK_RPS" \
        --summary-export "$OUTPUT_DIR/${arch}_run${i}_summary.json" \
        "loadtests/scenarios/${SCENARIO}.js"
    elif command -v docker >/dev/null 2>&1; then
      docker run --rm -i --user "$(id -u):$(id -g)" --network host -v "$ROOT_DIR:/work" -w /work grafana/k6:latest run \
        -e TARGET_URL="$url" -e PEAK_RPS="$PEAK_RPS" \
        --summary-export "$OUTPUT_DIR/${arch}_run${i}_summary.json" \
        "loadtests/scenarios/${SCENARIO}.js"
    else
      echo "ERROR: k6 is not installed on host and Docker is not available" >&2
      exit 1
    fi
    k6_status=$?
    set -e
    if [ "$k6_status" -ne 0 ] && [ "$k6_status" -ne 99 ]; then
      echo "k6 failed with fatal error code $k6_status" >&2
      exit "$k6_status"
    fi

    summary="$OUTPUT_DIR/${arch}_run${i}_summary.json"
    if [ -f "$summary" ] && command -v jq >/dev/null 2>&1; then
      echo "[Run $i/$RUNS] k6 summary: $(jq -r '"RPS=\(.metrics.http_reqs.rate|floor) reqs=\(.metrics.http_reqs.count) p95=\(.metrics.http_req_duration["p(95)"])ms success=\(.metrics.orders_success_total.count // 0) rejected=\(.metrics.orders_business_rejection_total.count // 0) 5xx=\(.metrics.orders_system_failure_total.count // 0)"' "$summary")"
    fi

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
