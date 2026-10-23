#!/usr/bin/env bash
set -euo pipefail

# Chạy tuần tự từ Current Stage trong IMPLEMENTATION_LEDGER.md đến hết Stage 13
# bằng một orchestrator duy nhất.
# Đã tối ưu biến môi trường chống timeout/ECONNRESET trên sub2api.

cd "$(dirname "$0")/.."

# Nạp file .env nếu có (tiện cho việc cấu hình ANTHROPIC_API_KEY / ANTHROPIC_BASE_URL)
if [ -f .env ]; then
  set -a
  # shellcheck source=/dev/null
  source .env
  set +a
fi

# 1. Tối ưu mạng chống ECONNRESET & rớt socket TCP trên Linux:
# Ưu tiên IPv4 trước IPv6 giải quyết triệt để lỗi DNS/socket hang khi qua proxy/Cloudflare
export NODE_OPTIONS="${NODE_OPTIONS:-} --dns-result-order=ipv4first"
export UV_THREADPOOL_SIZE=16

# 2. Tối ưu timeout và retry chống gián đoạn trên sub2api:
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
export API_TIMEOUT_MS=300000                        # 5 phút timeout cho request dài
export CLAUDE_STREAM_FIRST_BYTE_TIMEOUT_MS=300000   # 5 phút chờ byte đầu tiên (tránh proxy buffer làm rớt kết nối)
export CLAUDE_STREAM_IDLE_TIMEOUT_MS=300000         # 5 phút idle stream
export CLAUDE_CODE_MAX_RETRIES=5                    # Tự động retry tối đa 5 lần nếu gặp sự cố mạng tạm thời

# Commit do agent tạo mang tên claude[bot] (liên kết với GitHub App của Claude để hiện avatar và badge bot).
export GIT_AUTHOR_NAME="claude[bot]"
export GIT_AUTHOR_EMAIL="209825114+claude[bot]@users.noreply.github.com"
export GIT_COMMITTER_NAME="claude[bot]"
export GIT_COMMITTER_EMAIL="209825114+claude[bot]@users.noreply.github.com"

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
