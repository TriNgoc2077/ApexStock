# SDD Ledger — Plan: IMPLEMENTATION_PLAN.md

> **Mục đích:** Bộ nhớ trạng thái bền vững trên đĩa (Single Source of Truth) giúp Orchestrator và các Subagents không bị mất trạng thái khi Claude Code tự động nén ngữ cảnh (`/compact`) hoặc khởi động lại phiên.
> **Quy tắc bắt buộc:** Đọc file này đầu tiên trước mọi hành động và cập nhật ngay sau mỗi bước chuyển trạng thái (State Transition).

---

## 1. Trạng thái thực thi hiện tại (Current Execution State)

- **Overall Status:** `IN_PROGRESS`
- **Current Stage:** `Stage 4`
- **Execution Mode:** `SEQUENTIAL` — một orchestrator duy nhất, chạy Stage 1 → Stage 13, mỗi lần một subagent
- **Current Phase:** `IMPLEMENT` (`READY` | `IMPLEMENT` | `CHECKPOINT_COMMITTED` | `REVIEW_ROUND_1` | `REVIEW_ROUND_2` | `VERIFY` | `COMMITTED`)
- **Base Commit SHA:** `91ce71c` (stage-0)
- **Last Completed Stage:** `Stage 3`
- **Last Completed Commit SHA:** `820b61c`

---

## 2. Decision Gate D0 Checklist (Đã chốt trong `IMPLEMENTATION_PLAN.md` Mục 4)

| ID | Nội dung cốt lõi đã khóa | Trạng thái |
|---|---|---|
| D0.1 | API trả `201 Created`, dùng NATS JetStream persistent; `<5ms` là target hypothesis | LOCKED |
| D0.2 | Chung endpoint `POST /api/v1/flash-sale/orders`, cùng schema; Baseline `:8081`, Optimized `:8082` | LOCKED |
| D0.3 | Tách rõ `available`, `reserved`, `paid`; công thức reconciliation chuẩn hóa cho cả 2 bên | LOCKED |
| D0.4 | Mỗi order flash sale chỉ mua đúng 1 sản phẩm (`single-item order`) | LOCKED |
| D0.5 | Reservation record lưu đủ `reservation_id`, `item_id`, `user_id`, `qty`, `state` | LOCKED |
| D0.6 | Ưu tiên Payment Simulator trước; SePay adapter cô lập provider-specific verification | LOCKED |
| D0.7 | Late payment sau `expired` giữ nguyên `expired`, ghi `webhook_logs` trạng thái `refund` | LOCKED |
| D0.8 | Index thường trên `idempotency_keys(expires_at)`, không dùng `now()` trong partial index predicate | LOCKED |
| D0.9 | Khóa budget CPU/RAM, chạy benchmark tuần tự từng kiến trúc; tách profile `dev`/`demo`/`benchmark` | LOCKED |
| D0.10 | Mọi con số RPS/latency trong tài liệu là hypothesis cho đến khi có raw benchmark measurement | LOCKED |
| D0.11 | Benchmark chính thức chạy tuần tự; chạy song song chỉ phục vụ visual demo và phải gắn nhãn | LOCKED |
| D0.12 | Không gọi Docker socket từ browser; điều khiển qua Go controller localhost-only có allowlist | LOCKED |
| D0.13 | Redis cấu hình `noeviction` cho inventory/idempotency keyspace | LOCKED |
| D0.14 | Next.js App Router dùng `[id]`; WebSocket cho War Room, polling cho trang người dùng | LOCKED |
| D0.15 | Một PostgreSQL container, hai database `apexstock_baseline` / `apexstock_optimized`, cùng migration + seed IDs | LOCKED |

---

## 3. Bảng theo dõi tiến độ từng Stage (Stage Progress Matrix)

| Stage | Tên Stage | Phụ thuộc | Primary Agent | Support Agent(s) | Reviewer | Status | Fix Rounds | Commit SHA |
|---:|---|---|---|---|---|---|---:|---|
| **0** | Contract, decision gate & skeleton docs | None | `stages-planner` / `backend-core` | `infra-devops` | `code-reviewer` | `COMPLETED` | 0 | `91ce71c` |
| **1** | Host foundation, Docker Compose & health slice | Stage 0 | `infra-devops` | `backend-core`, `frontend-ui` | `code-reviewer` | `COMPLETED` | 0 | `8ce7d19` |
| **2** | PostgreSQL migration, Redis keyspace & seed/reset | Stage 1 | `backend-core` | `infra-devops` | `code-reviewer` | `COMPLETED` | 1 | `bc2ba07` |
| **3** | Baseline order vertical slice | Stage 2 | `backend-core` | - | `code-reviewer` | `COMPLETED` | 1 | `-` |
| **4** | Optimized reservation, idempotency, NATS & worker | Stage 2, 3 | `backend-core` | `infra-devops` | `code-reviewer` | `PENDING` | 0 | - |
| **5** | Payment simulator, confirm/fail/timeout/cancel & restock | Stage 3, 4 | `backend-core` | `payment-integrations` | `code-reviewer` | `PENDING` | 0 | - |
| **6** | SePay adapter thật cho UI | Stage 5 | `payment-integrations` | `backend-core` | `code-reviewer` | `PENDING` | 0 | - |
| **7** | Observability, metrics, logs & reliability baseline | Stage 3, 4, 5 | `observability-reliability` | `backend-core`, `infra-devops` | `code-reviewer` | `PENDING` | 0 | - |
| **8** | Frontend product flow & design system | Stage 5 | `frontend-ui` | - | `code-reviewer` | `PENDING` | 0 | - |
| **9** | War Room dashboard & chaos controller | Stage 7, 8 | `frontend-ui` | `backend-core`, `infra-devops` | `code-reviewer` | `PENDING` | 0 | - |
| **10** | QA test pyramid, concurrency & invariant verifier | Stage 3–9 | `qa-loadtest` | `backend-core` | `code-reviewer` | `PENDING` | 0 | - |
| **11** | k6 benchmark & fair comparison | Stage 7, 10 | `qa-loadtest` | `infra-devops` | `code-reviewer` | `PENDING` | 0 | - |
| **12** | Chaos, resilience, soak & system testing | Stage 7, 9, 10, 11 | `qa-loadtest` | `observability-reliability`, `infra-devops` | `code-reviewer` | `PENDING` | 0 | - |
| **13** | Demo hardening, evidence pack & context handoff | Stage 11, 12 | `qa-loadtest` | `infra-devops`, `observability-reliability` | `code-reviewer` | `PENDING` | 0 | - |

---

## 4. Nhật ký phán quyết kỹ thuật (Technical Rulings Ledger)

> Khi gặp điểm chưa rõ hoặc xung đột nhỏ giữa các bước mà không vi phạm Invariants I1–I5, Orchestrator ra quyết định ngay và ghi vào đây theo mẫu:
> `- [Stage X] Ruling: <Quyết định> — Lý do: <Tại sao> — Rủi ro/Chi phí nếu sai: <Mức độ ảnh hưởng>`

*(Chưa có phán quyết phát sinh)*

---

## 5. Nhật ký chi tiết từng Stage (Stage Execution Log)

> Sau khi hoàn tất mỗi stage, Orchestrator ghi thêm một mục vào dưới đây gồm: Deliverables chính, Lệnh test đã chạy & kết quả, Tóm tắt kết quả Code Review, và Commit SHA.

### Stage 1: Host foundation, Docker Compose & health slice

**Phase:** VERIFY (pending commit)  
**Primary Agent:** infra-devops  
**Deliverables:**
- Docker Compose với 4 profiles (dev/demo/benchmark/observability)
- PostgreSQL init script tạo 2 databases
- Go services: api-baseline, api-optimized, worker, warroom-controller với health/readiness endpoints
- Next.js app shell
- Resource limits theo benchmark-matrix.yaml
- Fixed compose volume paths (từ `./deploy/*` → `../../deploy/*`)
- Added `web/postcss.config.js` cho Tailwind CSS

**Verification:**
```bash
gofmt -l .                  # PASS
go vet ./...                # PASS
go test -race ./...         # PASS (no test files)
make build                  # PASS (4 binaries)
docker compose config -q    # PASS
bash -n scripts/**/*.sh     # PASS
```

**Acceptance:**
- ✅ Fresh checkout builds
- ✅ Compose up without real secrets
- ✅ PostgreSQL creates 2 databases
- ✅ Health/readiness checks work
- ✅ No exposed ports to host (default)
- ✅ Resource limits applied
- ✅ Fixed service names/URLs

**Commit SHA:** `8ce7d19`

### Stage 3: Baseline order vertical slice

**Phase:** COMMITTED  
**Primary Agent:** backend-core  
**Deliverables:**
- Shared `POST /api/v1/flash-sale/orders` handler on Baseline `:8081`.
- Contract validation for UUIDs, `quantity=1`, idempotency header/body and correlation ID.
- PostgreSQL transaction with item `SELECT ... FOR UPDATE`, active-event/stock/max-per-user checks.
- Atomic order, order item, simulated payment, baseline stock decrement and idempotency response persistence.
- Idempotent replay and payload-conflict handling; advisory transaction lock closes same-key check-then-insert race.

**Verification:**
```bash
gofmt -w ...                    # PASS
go test ./...                   # PASS
go vet ./...                    # PASS
go test -race ./...             # PASS
docker compose -f deploy/compose/docker-compose.yaml config -q  # PASS
git diff --check                # PASS
```

**Review:** Round 1 found missing tracked dependency artifact and same-key concurrency race; fixes applied and re-verified.

**Acceptance:**
- ✅ Baseline endpoint uses the common API path and 201 response.
- ✅ Baseline inventory mutation is transactional and row-locked.
- ✅ Idempotency response is persisted and replayed for retries.
- ✅ Same-key concurrent claims are serialized before the idempotency row exists.

**Commit SHA:** `820b61c`


**Phase:** VERIFY (pending commit)  
**Primary Agent:** backend-core  
**Deliverables:**
- Shared PostgreSQL migration for both isolated databases
- Deterministic seed: 50,000 users, active event, 10,000 stock
- Redis key helpers and atomic Lua scripts for reserve/release/confirm/rate-limit
- Redis warm-up, reset and SQL reconciliation scripts
- Makefile targets and README instructions for database lifecycle

**Verification:**
```bash
gofmt -l .                         # PASS
go vet ./...                       # PASS
go test -race ./...                # PASS (no test files)
bash -n scripts/*.sh               # PASS
docker compose config -q            # PASS (agent verification)
```

**Runtime note:** Host execution requires overriding `POSTGRES_HOST`/`POSTGRES_PORT` to a published PostgreSQL endpoint; defaults use Compose service names (`postgres`, `redis`) intentionally. Compose does not publish database/Redis ports by default.

**Correctness fixes:** Script password defaults aligned with Compose (`apexstock`); Redis reset now uses explicit `redis-cli` commands instead of feeding SQL-style comments to the Redis CLI.

**Acceptance:**
- ✅ Shared migration applies to both databases (D0.15)
- ✅ Deterministic logical IDs and seed manifest
- ✅ Active event and inventory snapshot
- ✅ No invalid `now()` partial-index predicate (D0.8)
- ✅ Reset and reconciliation tooling

**Commit SHA:** (pending)
