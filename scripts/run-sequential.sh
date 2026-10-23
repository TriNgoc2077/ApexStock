#!/usr/bin/env bash
set -euo pipefail

# Chạy tuần tự chuẩn xác từ Stage hiện tại (Stage 1) đến hết Stage 13
# Đã tối ưu biến môi trường chống timeout/ECONNRESET trên sub2api

export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
export API_TIMEOUT_MS=300000

PROMPT="Đọc IMPLEMENTATION_LEDGER.md, docs/decisions/D0-decisions-summary.md, docs/contracts/openapi.yaml, docs/domain/inventory-state-model.md, docs/benchmark/benchmark-matrix.yaml và IMPLEMENTATION_PLAN.md. Stage 0 đã COMPLETED. Hãy bắt đầu thực thi TUẦN TỰ, LIÊN TỤC từng giai đoạn từ Stage 1 đến hết Stage 13 theo đúng chu trình State Machine trong orchestrator.md (Implement -> Code Review -> Fix -> Verify -> Commit & Update IMPLEMENTATION_LEDGER.md -> Next Stage). Mỗi stage kế thừa trực tiếp codebase đã test pass của stage trước. Không dừng lại hỏi tôi giữa các stage."

echo "=================================================================="
echo " Khởi động Claude Code — CHẾ ĐỘ TUẦN TỰ CHUẨN (Stage 1 -> Stage 13)"
echo "=================================================================="
exec claude --agent orchestrator --dangerously-skip-permissions "$PROMPT"
