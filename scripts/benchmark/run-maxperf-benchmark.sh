#!/usr/bin/env bash
# =============================================================================
# run-maxperf-benchmark.sh — Maximum Performance Benchmark Runner
# =============================================================================
# Mục tiêu: đạt 40-50k RPS bằng cách:
#   1. Kill các tiến trình dư thừa trên host
#   2. Apply OS-level tuning (sysctl, ulimits)
#   3. Khởi động stack maxperf (không có Grafana/Prometheus/web)
#   4. Chạy benchmark với PEAK_RPS cao
#   5. Dọn dẹp và khôi phục sau khi xong
#
# Sử dụng:
#   ./scripts/benchmark/run-maxperf-benchmark.sh [PEAK_RPS] [RUNS]
#
# Ví dụ:
#   ./scripts/benchmark/run-maxperf-benchmark.sh 40000 3   # mục tiêu 40k RPS
#   ./scripts/benchmark/run-maxperf-benchmark.sh 50000 2   # mục tiêu 50k RPS
# =============================================================================
set -euo pipefail

PEAK_RPS="${1:-40000}"
RUNS="${2:-3}"
SCENARIO="${SCENARIO:-sustainable-throughput}"

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT_DIR"

COMPOSE_FILE="deploy/compose/docker-compose.maxperf.yaml"
COMPOSE_CMD="docker compose -f $COMPOSE_FILE"

RUN_ID="maxperf-$(date +%Y%m%d-%H%M%S)"
OUTPUT_DIR="loadtests/results/$RUN_ID"
mkdir -p "$OUTPUT_DIR"

# =============================================================================
# Màu sắc terminal
# =============================================================================
RED='\033[0;31m'; YELLOW='\033[1;33m'; GREEN='\033[0;32m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

log()  { echo -e "${CYAN}[MAXPERF]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
err()  { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# =============================================================================
# Kiểm tra quyền và dependencies
# =============================================================================
check_deps() {
  log "Kiểm tra dependencies..."
  for cmd in docker jq; do
    command -v "$cmd" >/dev/null 2>&1 || { err "Missing: $cmd"; exit 1; }
  done
  if ! command -v k6 >/dev/null 2>&1; then
    warn "k6 không có trên host, sẽ dùng Docker k6"
  fi
  ok "Dependencies OK"
}

# =============================================================================
# Kill các tiến trình dư thừa trên host
# =============================================================================
kill_host_noise() {
  log "Tắt các tiến trình dư thừa trên host..."

  # Danh sách các process nên tắt trong khi benchmark
  # (Chỉ kill nếu chúng đang chạy — bỏ qua lỗi nếu không có)
  local noise_procs=(
    "baloo_file"          # KDE file indexer — ngốn I/O
    "tracker-miner"       # GNOME file indexer
    "zeitgeist"           # activity logger
    "evolution-data"      # mail sync
    "gvfsd"               # GNOME virtual filesystem daemon
    "update-notifier"     # apt update checker
    "packagekitd"         # package manager daemon
    "snapd"               # snap daemon (nếu có)
  )

  for proc in "${noise_procs[@]}"; do
    if pgrep -x "$proc" >/dev/null 2>&1; then
      warn "Killing: $proc"
      pkill -x "$proc" 2>/dev/null || true
    fi
  done

  # Tắt swap tạm thời để loại bỏ latency spikes từ swapping
  if swapon --show | grep -q .; then
    warn "Tắt swap tạm thời (sẽ bật lại sau benchmark)..."
    sudo swapoff -a 2>/dev/null && SWAP_WAS_ON=true || warn "Không thể tắt swap (thiếu quyền sudo), bỏ qua"
  fi
  SWAP_WAS_ON="${SWAP_WAS_ON:-false}"

  ok "Host noise reduction done"
}

# =============================================================================
# OS-level kernel tuning
# =============================================================================
apply_sysctl_tuning() {
  log "Apply OS sysctl tuning cho high-throughput..."

  # Kiểm tra có quyền sudo không
  if ! sudo -n true 2>/dev/null; then
    warn "Không có sudo không cần mật khẩu — bỏ qua sysctl tuning"
    warn "Để áp dụng đầy đủ, hãy chạy: sudo bash scripts/benchmark/apply-sysctl.sh"
    return 0
  fi

  # Lưu lại giá trị cũ để restore
  OLD_SOMAXCONN=$(cat /proc/sys/net/core/somaxconn)
  OLD_RMEM_MAX=$(cat /proc/sys/net/core/rmem_max)
  OLD_WMEM_MAX=$(cat /proc/sys/net/core/wmem_max)
  OLD_SYNCOOKIES=$(cat /proc/sys/net/ipv4/tcp_syncookies)

  # TCP/Network tuning
  sudo sysctl -w net.core.somaxconn=65535 >/dev/null
  sudo sysctl -w net.core.netdev_max_backlog=65535 >/dev/null
  sudo sysctl -w net.ipv4.tcp_max_syn_backlog=65535 >/dev/null
  sudo sysctl -w net.ipv4.tcp_syncookies=1 >/dev/null
  sudo sysctl -w net.ipv4.tcp_tw_reuse=1 >/dev/null
  sudo sysctl -w net.ipv4.ip_local_port_range="1024 65535" >/dev/null
  sudo sysctl -w net.core.rmem_max=134217728 >/dev/null  # 128MB
  sudo sysctl -w net.core.wmem_max=134217728 >/dev/null
  sudo sysctl -w net.ipv4.tcp_rmem="4096 87380 134217728" >/dev/null
  sudo sysctl -w net.ipv4.tcp_wmem="4096 65536 134217728" >/dev/null
  sudo sysctl -w net.ipv4.tcp_fin_timeout=10 >/dev/null
  sudo sysctl -w net.ipv4.tcp_keepalive_time=60 >/dev/null
  sudo sysctl -w net.ipv4.tcp_keepalive_intvl=10 >/dev/null
  sudo sysctl -w net.ipv4.tcp_keepalive_probes=6 >/dev/null

  # File descriptor limits
  sudo sysctl -w fs.file-max=2097152 >/dev/null
  ulimit -n 65535 2>/dev/null || true

  ok "sysctl tuning applied"
}

# =============================================================================
# Restore OS settings
# =============================================================================
restore_sysctl() {
  log "Khôi phục sysctl settings..."
  if [ -n "${OLD_SOMAXCONN:-}" ]; then
    sudo sysctl -w net.core.somaxconn="$OLD_SOMAXCONN" >/dev/null 2>&1 || true
    sudo sysctl -w net.core.rmem_max="$OLD_RMEM_MAX" >/dev/null 2>&1 || true
    sudo sysctl -w net.core.wmem_max="$OLD_WMEM_MAX" >/dev/null 2>&1 || true
    sudo sysctl -w net.ipv4.tcp_syncookies="$OLD_SYNCOOKIES" >/dev/null 2>&1 || true
  fi

  # Bật lại swap nếu đã tắt
  if [ "${SWAP_WAS_ON:-false}" = "true" ]; then
    sudo swapon -a 2>/dev/null || true
    ok "Swap đã được bật lại"
  fi
  ok "Sysctl restored"
}

# =============================================================================
# Khởi động maxperf stack
# =============================================================================
start_stack() {
  log "Dừng stack cũ nếu đang chạy..."
  $COMPOSE_CMD down --remove-orphans 2>/dev/null || true

  # Dừng luôn stack benchmark/demo nếu đang chạy
  docker compose -f deploy/compose/docker-compose.yaml down --remove-orphans 2>/dev/null || true

  log "Khởi động maxperf stack..."
  $COMPOSE_CMD up -d --build

  log "Chờ services healthy..."
  local max_wait=120
  local elapsed=0
  while [ $elapsed -lt $max_wait ]; do
    if $COMPOSE_CMD ps | grep -E "(unhealthy|starting)" | grep -qv "^$"; then
      sleep 5
      elapsed=$((elapsed + 5))
    else
      break
    fi
  done

  # Final check
  $COMPOSE_CMD ps
  ok "Stack maxperf đang chạy"
}

# =============================================================================
# Record metadata
# =============================================================================
record_metadata() {
  cat > "$OUTPUT_DIR/metadata.json" <<EOF
{
  "run_id": "$RUN_ID",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "scenario": "$SCENARIO",
  "peak_rps": $PEAK_RPS,
  "runs": $RUNS,
  "git_commit": "$(git rev-parse HEAD)",
  "os": "$(uname -srm)",
  "cpu_model": "$(lscpu | grep 'Model name' | cut -d: -f2 | xargs || echo 'Unknown')",
  "cpu_cores": $(nproc),
  "total_ram_kb": $(grep MemTotal /proc/meminfo | awk '{print $2}' || echo 0),
  "stack": "maxperf",
  "notes": "Full resource allocation: PostgreSQL 4CPU/3GB, api-baseline 4CPU, api-optimized 3x0.75CPU, autovacuum off, synchronous_commit off"
}
EOF
  ok "Metadata saved to $OUTPUT_DIR/metadata.json"
}

# =============================================================================
# Tính INITIAL_STOCK: sustainable-throughput gửi ~59 × PEAK_RPS requests
# Redis budget (640MB noeviction) chứa được ~600k reservations
# =============================================================================
calc_stock() {
  local default_stock=$((PEAK_RPS * 60))
  [ "$default_stock" -gt 600000 ] && default_stock=600000
  echo "$default_stock"
}

# =============================================================================
# Chạy benchmark cho một kiến trúc
# =============================================================================
run_arch() {
  local arch="$1"
  local url="$2"
  local dsn="$3"
  local stock="$4"

  echo ""
  echo -e "${BOLD}============================================================${NC}"
  echo -e "${BOLD}  [MAXPERF] Benchmarking: $arch | PEAK_RPS=$PEAK_RPS${NC}"
  echo -e "${BOLD}============================================================${NC}"

  for i in $(seq 1 "$RUNS"); do
    log "[Run $i/$RUNS] Resetting & Seeding Database (INITIAL_STOCK=$stock)..."
    INITIAL_STOCK="$stock" MAX_PER_USER=100000000 ./scripts/reset-data.sh
    INITIAL_STOCK="$stock" MAX_PER_USER=100000000 ./scripts/seed-data.sh

    if [ "$arch" = "optimized" ]; then
      log "[Run $i/$RUNS] Warming up Redis cache..."
      ./scripts/warmup-redis.sh
    fi

    log "[Run $i/$RUNS] Cooldown 15s (thermal parity)..."
    sleep 15

    # Drop OS page cache để có kết quả nhất quán
    if sudo -n true 2>/dev/null; then
      sync && sudo sh -c 'echo 1 > /proc/sys/vm/drop_caches' 2>/dev/null || true
    fi

    log "[Run $i/$RUNS] Chạy k6 (PEAK_RPS=$PEAK_RPS)..."
    local summary_file="$OUTPUT_DIR/${arch}_run${i}_summary.json"

    set +e
    if command -v k6 >/dev/null 2>&1; then
      PEAK_RPS="$PEAK_RPS" INITIAL_STOCK="$stock" MAX_PER_USER=100000000 \
        k6 run \
          -e TARGET_URL="$url" \
          -e PEAK_RPS="$PEAK_RPS" \
          --summary-export "$summary_file" \
          "loadtests/scenarios/${SCENARIO}.js"
    else
      docker run --rm -i \
        --user "$(id -u):$(id -g)" \
        --network host \
        -v "$ROOT_DIR:/work" -w /work \
        grafana/k6:latest run \
          -e TARGET_URL="$url" \
          -e PEAK_RPS="$PEAK_RPS" \
          --summary-export "$summary_file" \
          "loadtests/scenarios/${SCENARIO}.js"
    fi
    local k6_status=$?
    set -e

    if [ "$k6_status" -ne 0 ] && [ "$k6_status" -ne 99 ]; then
      err "k6 failed: exit code $k6_status"
      exit "$k6_status"
    fi

    # Parse và in kết quả
    if [ -f "$summary_file" ] && command -v jq >/dev/null 2>&1; then
      local result
      result=$(jq -r '
        "RPS=\(.metrics.http_reqs.rate|floor) " +
        "reqs=\(.metrics.http_reqs.count) " +
        "p95=\(.metrics.http_req_duration["p(95)"] // 0 | floor)ms " +
        "p99=\(.metrics.http_req_duration["p(99)"] // 0 | floor)ms " +
        "success=\(.metrics.orders_success_total.count // 0) " +
        "rejected=\(.metrics.orders_business_rejection_total.count // 0) " +
        "5xx=\(.metrics.orders_system_failure_total.count // 0)"
      ' "$summary_file")
      ok "[Run $i/$RUNS] $arch: $result"
    fi

    log "[Run $i/$RUNS] Invariant verification..."
    BASELINE_DSN="$dsn" go test -v ./test/system -run TestInvariantVerifier_StockEquation_Calculation 2>&1 | tail -5
  done
}

# =============================================================================
# Cleanup on exit
# =============================================================================
cleanup() {
  log "Cleanup..."
  restore_sysctl || true
  ok "Done. Results: $OUTPUT_DIR"
}
trap cleanup EXIT

# =============================================================================
# MAIN
# =============================================================================
echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║       ApexStock — MAXIMUM PERFORMANCE BENCHMARK             ║${NC}"
echo -e "${BOLD}║  Target: ${PEAK_RPS} RPS  |  Runs: ${RUNS}  |  Scenario: ${SCENARIO}  ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""

check_deps
kill_host_noise
apply_sysctl_tuning
start_stack
record_metadata

STOCK=$(calc_stock)
log "INITIAL_STOCK = $STOCK (= 60 × PEAK_RPS, capped at 600k)"

BASELINE_DSN="postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"
OPTIMIZED_DSN="postgres://apexstock:apexstock@localhost:5432/apexstock_optimized?sslmode=disable"

# --- 1. Baseline ---
run_arch "baseline" "http://localhost:8081" "$BASELINE_DSN" "$STOCK"

# --- Cooldown giữa 2 kiến trúc ---
echo ""
log "Cooldown 30s (CPU thermal parity giữa 2 kiến trúc)..."
sleep 30

# --- 2. Optimized ---
run_arch "optimized" "http://localhost:8082" "$OPTIMIZED_DSN" "$STOCK"

# =============================================================================
# Summary
# =============================================================================
echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║              BENCHMARK COMPLETE                              ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "Results saved to: $OUTPUT_DIR"
echo ""

if command -v jq >/dev/null 2>&1; then
  log "=== SUMMARY TABLE ==="
  for arch in baseline optimized; do
    for i in $(seq 1 "$RUNS"); do
      f="$OUTPUT_DIR/${arch}_run${i}_summary.json"
      [ -f "$f" ] || continue
      jq -r --arg a "$arch" --arg r "$i" \
        '"[\($a) run\($r)] RPS=\(.metrics.http_reqs.rate|floor) p95=\(.metrics.http_req_duration["p(95)"] // 0 | floor)ms p99=\(.metrics.http_req_duration["p(99)"] // 0 | floor)ms"' "$f"
    done
  done
fi
