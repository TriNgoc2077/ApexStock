#!/usr/bin/env bash
set -euo pipefail

# Chạy tuần tự từ Current Stage trong IMPLEMENTATION_LEDGER.md đến hết Stage 13
# bằng một orchestrator duy nhất.
# Đã tối ưu biến môi trường chống timeout/ECONNRESET trên sub2api.

cd "$(dirname "$0")/.."

export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
export API_TIMEOUT_MS=300000

# Commit do agent tạo mang tên Claude (chỉ áp dụng trong phiên này,
# không đổi git config của bạn).
export GIT_AUTHOR_NAME="Claude"
export GIT_AUTHOR_EMAIL="noreply@anthropic.com"
export GIT_COMMITTER_NAME="Claude"
export GIT_COMMITTER_EMAIL="noreply@anthropic.com"

# Pre-flight
for bin in claude git go docker node npm; do
  command -v "$bin" >/dev/null || { echo "Thiếu '$bin' trong PATH" >&2; exit 1; }
done
docker info >/dev/null 2>&1 || { echo "Docker daemon chưa chạy" >&2; exit 1; }
if [ -n "$(git status --porcelain)" ]; then
  echo "Working tree chưa sạch — hãy commit/stash trước để commit của agent không lẫn thay đổi của bạn." >&2
  git status --short >&2
  exit 1
fi

PROMPT="Đọc CLAUDE.md và IMPLEMENTATION_LEDGER.md trước, sau đó đọc docs/decisions/D0-decisions-summary.md, docs/contracts/openapi.yaml, docs/domain/inventory-state-model.md, docs/benchmark/benchmark-matrix.yaml và phần Stage hiện tại trong IMPLEMENTATION_PLAN.md. Bạn là orchestrator duy nhất. Hãy thực thi TUẦN TỰ, LIÊN TỤC từng giai đoạn từ Current Stage trong IMPLEMENTATION_LEDGER.md đến hết Stage 13, mỗi lần chỉ dispatch một subagent, theo đúng chu trình State Machine trong orchestrator.md (Implement -> Checkpoint Commit -> Code Review -> Fix -> Verify -> Commit & Update IMPLEMENTATION_LEDGER.md -> Next Stage). Mỗi stage kế thừa trực tiếp codebase đã test pass của stage trước. Không dừng lại hỏi tôi giữa các stage."

echo "=================================================================="
echo " Khởi động Claude Code — CHẾ ĐỘ TUẦN TỰ (Current Stage -> Stage 13)"
echo " Commit author: ${GIT_AUTHOR_NAME} <${GIT_AUTHOR_EMAIL}>"
echo "=================================================================="
exec claude --agent orchestrator --dangerously-skip-permissions "$PROMPT"
