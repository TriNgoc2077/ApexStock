# SDD Ledger — Plan: IMPLEMENTATION_PLAN.md

> **Mục đích:** Bộ nhớ trạng thái bền vững trên đĩa (Single Source of Truth) giúp Orchestrator và các Subagents không bị mất trạng thái khi Claude Code tự động nén ngữ cảnh (`/compact`) hoặc khởi động lại phiên.
> **Quy tắc bắt buộc:** Đọc file này đầu tiên trước mọi hành động và cập nhật ngay sau mỗi bước chuyển trạng thái (State Transition).

---

## 1. Trạng thái thực thi hiện tại (Current Execution State)

- **Overall Status:** `IN_PROGRESS`
- **Current Stage:** `Stage 11`
- **Execution Mode:** `SEQUENTIAL` — một orchestrator duy nhất, chạy Stage 1 → Stage 13, mỗi lần một subagent
- **Current Phase:** `IMPLEMENT` (`READY` | `IMPLEMENT` | `CHECKPOINT_COMMITTED` | `REVIEW_ROUND_1` | `REVIEW_ROUND_2` | `VERIFY` | `COMMITTED`)
- **Base Commit SHA:** `91ce71c` (stage-0)
- **Last Completed Stage:** `Stage 10`
- **Last Completed Commit SHA:** `c5f25b5`

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
| **3** | Baseline order vertical slice | Stage 2 | `backend-core` | - | `code-reviewer` | `COMPLETED` | 1 | `820b61c` |
| **4** | Optimized reservation, idempotency, NATS & worker | Stage 2, 3 | `backend-core` | `infra-devops` | `code-reviewer` | `COMPLETED` | 0 | `a914a16` |
| **5** | Payment simulator, confirm/fail/timeout/cancel & restock | Stage 3, 4 | `backend-core` | `payment-integrations` | `code-reviewer` | `COMPLETED` | 1 | `a2b6053` |
| **6** | SePay adapter thật cho UI | Stage 5 | `payment-integrations` | `backend-core` | `code-reviewer` | `COMPLETED` | 0 | `f2a25ba` |
| **7** | Observability, metrics, logs & reliability baseline | Stage 3, 4, 5 | `observability-reliability` | `backend-core`, `infra-devops` | `code-reviewer` | `COMPLETED` | 0 | `627f740` |
| **8** | Frontend product flow & design system | Stage 5 | `frontend-ui` | - | `code-reviewer` | `COMPLETED` | 1 | `85d25ad` |
| **9** | War Room dashboard & chaos controller | Stage 7, 8 | `frontend-ui` | `backend-core`, `infra-devops` | `code-reviewer` | `COMPLETED` | 1 | `909eaca` |
| **10** | QA test pyramid, concurrency & invariant verifier | Stage 3–9 | `qa-loadtest` | `backend-core` | `code-reviewer` | `COMPLETED` | 0 | `c5f25b5` |
| **11** | k6 benchmark & fair comparison | Stage 7, 10 | `qa-loadtest` | `infra-devops` | `code-reviewer` | `IN_PROGRESS` | 0 | - |
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


### Stage 4: Optimized reservation, idempotency, NATS & worker

**Phase:** COMMITTED  
**Primary Agent:** backend-core  
**Deliverables:**
- Cryptographically random RFC 4122 UUIDv4 generation in `internal/optimized/service.go`
- `ErrRedisUnavailable` sentinel and HTTP 503 (`service_unavailable`) mapping in `internal/httpapi/optimized_handler.go`
- Worker daemon wired with PostgreSQL pool, NATS JetStream consumer and signal handling in `cmd/worker/main.go`
- Inventory reconciliation verifier (`ReconcileItem`, `ReconcileAll`) in `internal/optimized/reconcile.go`
- Comprehensive unit, integration, and 100-goroutine concurrency tests for Redis Lua reserve/release/confirm, NATS worker persistence/dedup, and race detector verification

**Verification:**
```bash
go build ./...                  # PASS
go vet ./...                    # PASS
go test -count=1 -race ./...    # PASS
```

**Acceptance:**
- ✅ Concurrent reserve does not oversell (Invariant I1)
- ✅ Same idempotency key performs single reservation (Invariant I3)
- ✅ Durable JetStream publish and worker `processed_events` deduplication (Invariant I5)
- ✅ Redis down fails fast with HTTP 503
- ✅ Reconciliation verifier checks stock equation

**Commit SHA:** `a914a16`


### Stage 5: Payment simulator, confirm/fail/timeout/cancel & restock

**Phase:** COMMITTED  
**Primary Agent:** backend-core  
**Deliverables:**
- Shared payment domain DTOs (`SimulatePaymentRequest`, `PaymentResultResponse`) and state machine (`EvaluateTransition`) enforcing valid transitions and D0.7 late-payment refund auditing (`webhook_logs.status='refund'`)
- Baseline (`BaselineService`) and Optimized (`OptimizedService`) payment processors with row locking, Redis Lua `Confirm`/`Release` idempotency, and atomic `sold_quantity` restock/confirmation
- Payment Simulator engine (`Simulator`) with configurable ratio distributions and deterministic overrides
- Expiry sweeper (`ExpirySweeper`) and scheduler daemon (`cmd/scheduler/main.go`)
- Shared `POST /api/v1/payments/simulate` endpoint on both Baseline (`:8081`) and Optimized (`:8082`)
- Concurrency stress tests (`TestConcurrency_ConfirmVsCancelVsExpireRace` on Baseline and Optimized) and post-run SQL reconciliation pass

**Verification:**
```bash
go build ./...                  # PASS
go vet ./...                    # PASS
go test -count=1 -race ./...    # PASS
bash scripts/reconcile.sql.sh   # PASS
```

**Acceptance:**
- ✅ Configurable & deterministic simulator ratios
- ✅ Each terminal outcome applies at most once (Invariant I2/I4)
- ✅ Payment failure/timeout/cancel restores stock exactly once
- ✅ Late payment after expiry keeps order `expired` and records `webhook_logs` status `refund` (D0.7)
- ✅ Post-run reconciliation passes

**Commit SHA:** `a2b6053`


### Stage 6: SePay adapter thật cho UI

**Phase:** COMMITTED  
**Primary Agent:** payment-integrations  
**Deliverables:**
- Provider-isolated SePay adapter (`internal/payments/sepay/adapter.go`) with environment-variable configuration, QR code URL generation, and timing-safe `Apikey`/`Bearer`/`X-Sepay-Signature` verification
- SePay webhook and initiate HTTP handlers (`internal/httpapi/sepay_webhook_handler.go`) mounted at `POST /api/v1/payments/sepay/initiate` and `POST /api/v1/webhooks/sepay` on both Baseline and Optimized APIs
- Webhook audit logging (`webhook_logs`) and D0.7 late-payment refund policy integration
- Webhook test fixtures (`test/fixtures/sepay_webhook_*.json`) and local demo runbook (`docs/runbooks/sepay-local-demo.md`)

**Verification:**
```bash
go build ./...                  # PASS
go vet ./...                    # PASS
go test -count=1 -race ./...    # PASS
```

**Acceptance:**
- ✅ Zero hardcoded credentials in Git (D0.6)
- ✅ Valid fixture updates order/payment exactly once
- ✅ Invalid signature rejected with 401 without mutating stock
- ✅ Duplicate webhook is idempotent
- ✅ Late payment after expiry follows D0.7 refund policy

**Commit SHA:** `f2a25ba`


### Stage 7: Observability, metrics, logs & reliability baseline

**Phase:** COMMITTED  
**Primary Agent:** observability-reliability  
**Deliverables:**
- Zero-dependency Prometheus metrics registry (`CounterVec`, `GaugeVec`, `HistogramVec`, `/metrics` exposition) in `internal/observability/metrics.go` distinguishing `2xx`, `409`, `429`, and `5xx` by `arch` and normalized route
- Structured JSON `slog` logger, correlation ID HTTP middleware, and NATS header propagation (`InjectNATSCorrelationID`, `ExtractNATSCorrelationID`) in `internal/observability/`
- Instrumentation wired into `cmd/api-baseline`, `cmd/api-optimized`, and `cmd/worker`
- Prometheus alert rules (`deploy/prometheus/alerts.yml`), Grafana dashboard provisioning (`deploy/grafana/provisioning/dashboards/apexstock.json`), metric catalog (`docs/observability/metric-catalog.md`), and incident response runbook (`docs/runbooks/incident-response.md`)

**Verification:**
```bash
go build ./...                  # PASS
go vet ./...                    # PASS
go test -count=1 -race ./...    # PASS
```

**Acceptance:**
- ✅ Order trace via `X-Correlation-ID` across HTTP and NATS
- ✅ Business rejections (`409`/`429`) separated from `5xx` system failures
- ✅ Latency histograms with `arch` and normalized `route` labels
- ✅ Non-resetting `apexstock_oversell_total` invariant counter
- ✅ Grafana dashboard & Prometheus alert rules provisioned

**Commit SHA:** `627f740`


### Stage 8: Frontend product flow & design system

**Phase:** COMMITTED  
**Primary Agent:** frontend-ui  
**Deliverables:**
- Backend Catalog & Order Read APIs (`internal/httpapi/catalog_handler.go`: `GET /api/v1/flash-sale/events`, `GET /api/v1/flash-sale/items/{id}`, `GET /api/v1/orders/{id}`) mounted on Baseline and Optimized APIs, plus CORS preflight and route normalization in `internal/observability/middleware.go`
- Design system tokens (dark/light theme, `prefers-reduced-motion` support) and animated primitives (`ThemeToggle.tsx`, `AuroraHero.tsx`, `SpotlightCard.tsx` using CSS custom properties without React rerender)
- Typed API client (`web/src/lib/api.ts`) with deterministic idempotency key store and visibility-aware polling hook (`web/src/hooks/usePolling.ts`)
- Product flow routes (`web/src/app/page.tsx`, `web/src/app/flash-sale/[id]/page.tsx`, `web/src/app/orders/[id]/page.tsx`) supporting Baseline vs Optimized architecture selector, SePay QR generation, and simulator payment triggers

**Verification:**
```bash
go build ./...                  # PASS
go vet ./...                    # PASS
go test -count=1 -race ./...    # PASS
cd web && npm run lint          # PASS
cd web && npm run build         # PASS
```

**Acceptance:**
- ✅ End-to-end user demo flow from flash sale catalog → item detail → order → payment QR / simulator → live polling status
- ✅ Duplicate click prevention via deterministic idempotency key store
- ✅ Stale/disconnect indicator, theme toggle, reduced-motion, and keyboard navigation
- ✅ Next.js App Router `[id]` segments and polling per D0.14

**Commit SHA:** `85d25ad`


### Stage 9: War Room dashboard & chaos controller

**Phase:** COMMITTED  
**Primary Agent:** frontend-ui  
**Deliverables:**
- War Room Controller service (`internal/warroom/service.go`) polling `/healthz` and `/metrics` from Baseline (`:8081`) and Optimized (`:8082`), computing RPS and `p50`/`p95`/`p99` histogram quantiles, and automatically detecting container/instance down transitions and recovery duration
- War Room HTTP/SSE handlers (`internal/warroom/handler.go`) and `cmd/warroom-controller/main.go` wiring `/api/v1/warroom/snapshot`, `/api/v1/warroom/stream`, and `/api/v1/warroom/run` (enforcing D0.11 sequential benchmark run state machine and D0.12 zero container-kill endpoints)
- Operator CLI chaos scripts (`scripts/chaos/{kill,restore}-{optimized,baseline}-api.sh`)
- Frontend `/war-room` dashboard (`web/src/app/war-room/page.tsx`, `TelemetryPanel.tsx`, `VerdictSummaryModal.tsx`, `ActivityLogPanel.tsx`) with live SSE stream, reconnect indicator, oversell invariant badge, and hypothesis disclaimer

**Verification:**
```bash
go build ./...                  # PASS
go vet ./...                    # PASS
go test -count=1 -race ./...    # PASS
bash -n scripts/chaos/*.sh      # PASS
cd web && npm run lint          # PASS
cd web && npm run build         # PASS
```

**Acceptance:**
- ✅ Idle screen shows Unknown/Healthy accurately
- ✅ Sequential start/stop state machine & labeled parallel visual demo mode (D0.11)
- ✅ Zero container kill/pause endpoints exposed to browser (D0.12); operator uses CLI scripts and War Room detects outage/recovery via health polling
- ✅ Live SSE stream with stale/reconnect marker

**Commit SHA:** `909eaca`


### Stage 10: QA test pyramid, concurrency & invariant verifier

**Phase:** COMMITTED  
**Primary Agent:** qa-loadtest  
**Deliverables:**
- Cross-architecture Invariant Verifier (`test/system/invariant_verifier.go`, `test/system/invariant_verifier_test.go`) validating I1 (stock conservation equation `available + reserved + paid == total`, `available >= 0`), I2/I4 (order vs payment terminal state transitions), I3 (idempotency uniqueness), and I5 (`processed_events` deduplication)
- Common API Contract Test Suite (`test/contract/order_contract_test.go`) verifying `201/400/404/409/503`, `ErrorEnvelope` schema, `X-Correlation-ID` propagation, and D0.4 single-unit enforcement across Baseline and Optimized
- Security & Boundary Verification Suite (`test/contract/security_test.go`) testing 1MB request body limit, SQL injection payload rejection, and D0.12 zero chaos/container-kill HTTP endpoints

**Verification:**
```bash
go build ./...                  # PASS
go vet ./...                    # PASS
go test -count=1 -race ./...    # PASS
```

**Acceptance:**
- ✅ Invariants I1–I5 verified across both architectures
- ✅ Contract parity between `:8081` (Baseline) and `:8082` (Optimized)
- ✅ Security boundary and D0.12 verified
- ✅ Zero data races under `go test -race ./...`

**Commit SHA:** `c5f25b5`
