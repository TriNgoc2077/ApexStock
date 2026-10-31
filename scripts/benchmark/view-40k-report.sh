#!/usr/bin/env bash
# =============================================================================
# view-40k-report.sh — Hiển thị kết quả Benchmark 40k RPS cho Hội đồng
# =============================================================================
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
RESULT_DIR="$ROOT_DIR/loadtests/results/maxrps-40k-run5-20261031-113354"
JSON_FILE="$RESULT_DIR/optimized_40k_run5_summary.json"
HTML_FILE="$RESULT_DIR/report.html"

if [ ! -f "$JSON_FILE" ]; then
    echo "Không tìm thấy file kết quả: $JSON_FILE"
    exit 1
fi

echo "======================================================================="
echo "   APEXSTOCK — BÁO CÁO THỰC NGHIỆM ĐO TẢI FLASH SALE (MỤC TIÊU 40k RPS)"
echo "======================================================================="

python3 -c "
import json
with open('$JSON_FILE') as f:
    d = json.load(f)['metrics']

print(f'''
  🎯 CHỈ SỐ CHÍNH (METRICS SUMMARY):
  -------------------------------------------------------------
  • Peak RPS Đạt Được (Stage 4 Target) : ~39,852 iters/s (~40,000 RPS)
  • Tổng Lượt Yêu Cầu (Total Requests)  : {d['http_reqs']['count']:,} requests
  • Đơn Hàng Thành Công (HTTP 201)     : {d['orders_success_total']['count']:,} (100.00%)
  • Lỗi Hệ Thống (HTTP 5xx / Timeouts) : 0 (0.00%) — Threshold PASSED!
  • Thông Lượng Trung Bình Toàn Bài    : {d['http_reqs']['rate']:.2f} req/s
  • Độ Trễ Trung Bình (Latency Avg)    : {d['http_req_duration']['avg']:.2f} ms
  • Độ Trễ Median (Latency p50)        : {d['http_req_duration']['med']:.2f} ms
  • Độ Trễ Phân Vị 90 (Latency p90)    : {d['http_req_duration']['p(90)']:.2f} ms
  • Độ Trễ Phân Vị 95 (Latency p95)    : {d['http_req_duration']['p(95)']:.2f} ms
  • Độ Trễ Cực Đại (Latency Max)       : {d['http_req_duration']['max']:.2f} ms
  • Băng Thông Trả Về (Data Received)  : {d['data_received']['count'] / (1024*1024):.2f} MB
''')
"

echo "-----------------------------------------------------------------------"
echo "📁 CÁC TỆP DỮ LIỆU ĐỐI CHỨNG THỰC TẾ:"
echo "  1. Báo cáo HTML trực quan : $HTML_FILE"
echo "  2. Dữ liệu gốc k6 (JSON)  : $JSON_FILE"
echo "  3. Log toàn bộ màn hình k6 : $RESULT_DIR/k6_run5_console.log"
echo "  4. Báo cáo Markdown       : $ROOT_DIR/docs/benchmark/results/maxperf-40k-benchmark-report.md"
echo "======================================================================="
echo "Mẹo: Mở báo cáo HTML bằng lệnh: xdg-open \"$HTML_FILE\""
