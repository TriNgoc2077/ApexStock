#!/usr/bin/env bash
set -euo pipefail

# Launcher chạy song song 4 cửa sổ Claude Code tối ưu cho sub2api (chống ECONNRESET & xung đột file)
# Cách dùng:
#   Terminal 1: ./scripts/run-lane.sh 1
#   Terminal 2: ./scripts/run-lane.sh 2
#   Terminal 3: ./scripts/run-lane.sh 3
#   Terminal 4: ./scripts/run-lane.sh 4

LANE="${1:-}"

export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
export API_TIMEOUT_MS=300000

case "$LANE" in
  1)
    PROMPT="Bạn phụ trách LANE 1. Đọc ledgers/LANE_1_INFRA.md, docs/decisions/D0-decisions-summary.md, docs/contracts/openapi.yaml, docs/domain/inventory-state-model.md, docs/benchmark/benchmark-matrix.yaml và tự động triển khai liên tục Stage 1 -> Stage 2 -> Stage 7 theo đúng chu trình trong orchestrator.md. TUYỆT ĐỐI KHÔNG đọc CONVERSATION_TRANSCRIPT.md. Chỉ sửa các đường dẫn thuộc sở hữu của LANE 1 và dùng ./scripts/safe-commit.sh để commit sau mỗi stage. Không dừng hỏi tôi giữa các stage."
    ;;
  2)
    PROMPT="Bạn phụ trách LANE 2. Đọc ledgers/LANE_2_BACKEND.md, docs/decisions/D0-decisions-summary.md, docs/contracts/openapi.yaml, docs/domain/inventory-state-model.md và tự động triển khai liên tục Stage 3 -> Stage 4 -> Stage 5 -> Stage 6 theo đúng chu trình trong orchestrator.md. TUYỆT ĐỐI KHÔNG đọc CONVERSATION_TRANSCRIPT.md. Chỉ sửa các đường dẫn thuộc sở hữu của LANE 2 và dùng ./scripts/safe-commit.sh để commit sau mỗi stage. Không dừng hỏi tôi giữa các stage."
    ;;
  3)
    PROMPT="Bạn phụ trách LANE 3. Đọc ledgers/LANE_3_FRONTEND.md, docs/decisions/D0-decisions-summary.md, docs/contracts/openapi.yaml, docs/domain/inventory-state-model.md và tự động triển khai liên tục Stage 8 -> Stage 9 theo đúng chu trình trong orchestrator.md. TUYỆT ĐỐI KHÔNG đọc CONVERSATION_TRANSCRIPT.md. Chỉ sửa các đường dẫn thuộc sở hữu của LANE 3 và dùng ./scripts/safe-commit.sh để commit sau mỗi stage. Không dừng hỏi tôi giữa các stage."
    ;;
  4)
    PROMPT="Bạn phụ trách LANE 4. Đọc ledgers/LANE_4_QA.md, docs/decisions/D0-decisions-summary.md, docs/contracts/openapi.yaml, docs/domain/inventory-state-model.md, docs/benchmark/benchmark-matrix.yaml và tự động triển khai liên tục Stage 10 -> Stage 11 -> Stage 12 -> Stage 13 theo đúng chu trình trong orchestrator.md. TUYỆT ĐỐI KHÔNG đọc CONVERSATION_TRANSCRIPT.md. Chỉ sửa các đường dẫn thuộc sở hữu của LANE 4 và dùng ./scripts/safe-commit.sh để commit sau mỗi stage. Không dừng hỏi tôi giữa các stage."
    ;;
  *)
    echo "Cách dùng: ./scripts/run-lane.sh <1|2|3|4>"
    echo "  1 : Lane Infra, DB Migration, Seed & Observability (Stage 1 -> 2 -> 7)"
    echo "  2 : Lane Backend Core: Baseline, Optimized, Payment & SePay (Stage 3 -> 4 -> 5 -> 6)"
    echo "  3 : Lane Frontend Next.js & War Room Controller/UI (Stage 8 -> 9)"
    echo "  4 : Lane QA Test Pyramid, k6 Benchmark, Chaos & Handoff (Stage 10 -> 11 -> 12 -> 13)"
    exit 1
    ;;
esac

echo "=================================================================="
echo " Khởi động Claude Code — LANE $LANE (Tối ưu chống ECONNRESET)"
echo "=================================================================="
exec claude --agent orchestrator --dangerously-skip-permissions "$PROMPT"
