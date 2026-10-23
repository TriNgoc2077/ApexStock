# ApexStock — Agent Context

Hệ thống flash sale high-concurrency so sánh **Baseline** (Go → PostgreSQL `SELECT ... FOR UPDATE`, `:8081`) và **Optimized** (Nginx → Go → Redis/Lua → NATS JetStream → Worker → PostgreSQL, `:8082`) trên một laptop (i5-10500H 6C/12T, 16GB RAM, Docker Compose).

## Thứ tự ưu tiên tài liệu (khi mâu thuẫn, file trên thắng file dưới)

1. `docs/decisions/D0-decisions-summary.md` — quyết định đã khóa (D0.1–D0.15)
2. `docs/contracts/openapi.yaml`, `docs/contracts/api-flash-sale-orders.md` — API contract
3. `docs/domain/inventory-state-model.md` — state kho & invariants
4. `docs/benchmark/benchmark-matrix.yaml` — resource budget, seed IDs, scenarios
5. `IMPLEMENTATION_PLAN.md` — deliverables/acceptance từng stage
6. `docs/design/*.md` — **chỉ tra cứu chi tiết** (schema, Lua, UI). Các file này viết trước D0 và còn nội dung đã bị thay thế, ví dụ: kill container từ UI / `docker.sock` / `/api/v1/warroom/chaos/kill` (bỏ theo D0.12), route `:id` (dùng `[id]` theo D0.14), `allkeys-lru` (dùng `noeviction` theo D0.13), 5.000 user (dùng `seeded_users` trong matrix), số RPS/latency (chỉ là hypothesis theo D0.10).

**Không đọc toàn bộ `docs/design/`** — chỉ grep/đọc đoạn cần cho stage hiện tại.

## Trạng thái & quy trình

- Trạng thái thực thi nằm ở `IMPLEMENTATION_LEDGER.md` — đọc đầu tiên, cập nhật sau mỗi chuyển phase.
- Chạy tuần tự Stage 1 → 13, một orchestrator, mỗi lần một subagent (xem `.claude/agents/orchestrator.md`).
- Không tự đổi semantics đã khóa. Điểm chưa rõ nhưng không vi phạm I1–I5: ra quyết định, ghi vào mục *Technical Rulings* của ledger.

## Invariants bắt buộc (chi tiết: IMPLEMENTATION_PLAN.md §3.1)

- I1: `available >= 0`, `total = available + reserved + paid`, không oversell.
- I2: reserve/release/confirm idempotent; release sau confirm và confirm sau expired là no-op.
- I3: cùng `(user, idempotency_key)` + cùng payload → cùng kết quả; khác payload → 409 `idempotency_conflict`.
- I4: order chỉ đi `pending_payment → paid | payment_failed | expired | cancelled`; late payment giữ `expired` + `webhook_logs.status='refund'`.
- I5: NATS at-least-once → consumer idempotent bằng DB unique constraint/processed-event.

## Môi trường đã kiểm tra

- Go 1.27 local (`go.mod` khai báo `go 1.22`, module `apexstock`), Node 24 / npm 11, Docker 29 + Compose v5.
- **k6 không cài trên host** → chạy k6 bằng image `grafana/k6` trong Compose/`docker run`.
- PostgreSQL: một container, hai database `apexstock_baseline` và `apexstock_optimized` (D0.15), cùng migration và seed IDs.
- Không expose PostgreSQL/Redis ra host mặc định; không commit secret (`.env` đã gitignore, chỉ commit `.env.example`).

## Lệnh verify chuẩn

```bash
gofmt -l . && go vet ./... && go test -race ./...
docker compose -f deploy/compose/docker-compose.yaml config -q   # đường dẫn compose do Stage 1 chốt
npm --prefix web run lint && npm --prefix web run build          # từ Stage 8
bash -n scripts/**/*.sh
```

## Commit

- Danh tính commit lấy từ biến môi trường do `scripts/run-sequential.sh` đặt (`Claude <noreply@anthropic.com>`). Không chạy `git config` để đổi danh tính.
- Hai commit mỗi stage: `wip(stage-X): implement ...` sau IMPLEMENT và `feat(stage-X): ...` sau VERIFY.
- Không commit `node_modules`, `.next`, binary, `.env`, raw k6 output lớn (chỉ commit summary/metadata trong evidence pack).
- Không `git push`, không rewrite history, không `--no-verify`.
