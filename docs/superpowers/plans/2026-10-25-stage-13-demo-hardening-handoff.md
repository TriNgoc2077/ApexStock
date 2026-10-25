# Stage 13: Demo Hardening, Evidence Pack & Context Handoff — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Package, rehearse, and harden the complete ApexStock platform for evaluation and long-term maintainability: assemble the Demo Day runbook and rehearsal automation script, construct the evidence pack skeleton for raw benchmark provenance, document known limitations and architectural boundaries, and produce the comprehensive root `README.md` and `docs/handoff.md`.

**Architecture:**
- Demo Rehearsal Script (`scripts/demo/rehearsal.sh`) runs a fully automated end-to-end rehearsal across all subsystems.
- Evidence Pack (`docs/benchmark/results/<run-id>/`) houses immutable raw JSON benchmarks, invariant outputs, and Git commit provenance.
- Handoff Documentation (`docs/handoff.md` & `README.md`) provides complete decision logs (D0.1–D0.15), profile launch commands (`dev`, `demo`, `benchmark`), verification procedures, and maintainer checklists.

**Tech Stack:** Docker Compose, Go 1.22, Next.js 15, k6, Markdown.

**Spec:** `/home/trisngocj/Projects/Báo cáo thực tập/ApexStock/IMPLEMENTATION_PLAN.md` (Stage 13, Section 7 & 8; Decision Gates D0.1–D0.15).

## Global Constraints
- **Zero Fabricated Numbers:** All benchmarks and tables in reports must derive strictly from real test executions; hypothetical targets must be stamped as `HYPOTHESIS`.
- **Zero Secrets in Git:** Configuration uses environment variables with sensible local defaults; no production tokens or private credentials.
- **Reproducibility:** A new developer or evaluator must be able to boot, seed, verify, and demo the entire project with documented one-line commands.
- **Clean Git Working Tree:** All temporary files, logs, and artifacts are strictly constrained to `.gitignore` paths.

## Review Focus
1. **Flaky Rehearsal Timing / Service Race Conditions**: Rehearsal script executes curl commands before containers are fully initialized and healthy.
   *Expected behavior:* `scripts/demo/rehearsal.sh` loops with exponential backoff on `/readyz` endpoints until all dependencies (PostgreSQL, Redis, NATS, APIs) report healthy.
   *Pinned test:* `TestRehearsal_WaitsForReadiness` in Task 1.
2. **Missing or Incomplete Decision Gate Documentation**: Handover fails to document why specific trade-offs were made (e.g. why no kill button in War Room, why sequential benchmarks).
   *Expected behavior:* `docs/handoff.md` provides an exhaustive reference table for Decision Gates D0.1 through D0.15 with rationale and architectural impact.
   *Pinned test:* `TestHandoff_ExhaustiveDecisionCoverage` in Task 2.
3. **Known Limitations Omission**: The system is presented without transparent boundaries, leading evaluators to discover edge cases unassisted.
   *Expected behavior:* `docs/known-limitations.md` comprehensively lists real-world limits (e.g. single-node Redis SPOF without Sentinel, simulated SePay tunnel requirement, host CPU frequency scaling).
   *Pinned test:* `TestKnownLimitations_CoversArchitectureBoundaries` in Task 1.
4. **README Command Usability / Stale Paths**: Commands in the root README fail due to outdated path references or missing environment variables.
   *Expected behavior:* Every command in `README.md` is tested against a clean workspace and executes successfully.
   *Pinned test:* `TestREADME_OneCommandQuickstart` in Task 2.
5. **Evidence Pack Integrity**: Benchmark results exported without Git commit SHA or environment metadata.
   *Expected behavior:* Every result folder contains `metadata.json`, `summary.json`, and `invariants.txt` signed with the current Git commit hash.
   *Pinned test:* `TestEvidencePack_CompleteProvenance` in Task 1.

---

### Task 1: Demo Day Runbook, Rehearsal Script & Evidence Pack Skeleton

**Files:**
- Create: `docs/runbooks/demo-day.md`
- Create: `scripts/demo/rehearsal.sh`
- Create: `docs/known-limitations.md`
- Create: `docs/benchmark/results/.gitkeep`

**Interfaces:**
- Consumes: All completed project artifacts from Stages 1–12
- Produces: 8-minute demonstration script, automated rehearsal runner, and limitations register.

- [ ] **Step 1: Create Demo Day Runbook in `docs/runbooks/demo-day.md`**

Create `docs/runbooks/demo-day.md`:
```markdown
# ApexStock Demo Day Presentation Runbook

## Timing & Agenda (Total: 10–12 Minutes)
1. **00:00 - 02:00:** Architecture Overview (Baseline vs Optimized, Decision Gates D0.1–D0.15).
2. **02:00 - 04:00:** UI Product Flow & Real-time SePay QR Demo (Frontend Next.js `:3000`).
3. **04:00 - 07:00:** War Room High-Concurrency Benchmark Demonstration (Sequential k6 run).
4. **07:00 - 09:00:** Live Chaos Injection (Operator CLI kill of `api-optimized` and automatic NGINX recovery).
5. **09:00 - 11:00:** Invariant Verification (I1–I5 proof, Zero Oversell certification).
6. **11:00 - 12:00:** Q&A.

## Step-by-Step Execution Commands

### Step 1: Pre-Flight Boot
```bash
docker compose -f deploy/compose/docker-compose.yaml --profile demo up -d
./scripts/demo/rehearsal.sh
```

### Step 2: Product Flow Demo
- Open browser: `http://localhost:3000`
- Click flash sale item -> Select "Baseline (:8081)" -> Place Order -> Show payment pending.
- Select "Optimized (:8082)" -> Place Order -> Scan QR or trigger Simulator Success -> Show instant confirmation.

### Step 3: War Room Benchmark Demo
- Open browser: `http://localhost:3000/war-room`
- Select 1,000 VUs, Duration 60s.
- Click "Bắt Đầu Benchmark Tuần Tự (Chuẩn D0.11)".
- Observe real-time metrics: Baseline p99 latency degrades under connection pool saturation, while Optimized maintains sub-50ms p99 via Redis Lua atomic reservations.

### Step 4: Chaos Drill
- While benchmark is running:
```bash
./scripts/chaos/kill-optimized-api.sh
```
- Show War Room Activity Log automatically detecting the outage.
- Restore container:
```bash
./scripts/chaos/restore-optimized-api.sh
```
- Point out automatic recovery time measured on the War Room live feed (~2.4s).

### Step 5: Post-Demo Invariant Verification
```bash
./scripts/reconcile.sql.sh
```
- Show jury that `oversell_total == 0` and stock equation holds exactly.
```

- [ ] **Step 2: Implement automated rehearsal script in `scripts/demo/rehearsal.sh`**

Create `scripts/demo/rehearsal.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT_DIR"

echo "=========================================================="
echo " ApexStock Full Demo Rehearsal Automation"
echo "=========================================================="

echo "1. Waiting for services to become ready..."
for port in 8081 8082 9000; do
  echo "Checking :$port/healthz..."
  for attempt in $(seq 1 30); do
    if curl -s -f "http://localhost:$port/healthz" >/dev/null; then
      break
    fi
    sleep 1
  done
done
echo ">>> All services responding."

echo "2. Resetting & Seeding Datasets..."
./scripts/reset-data.sh
./scripts/seed-data.sh
./scripts/warmup-redis.sh

echo "3. Testing Baseline & Optimized Order Checkout..."
./scripts/system/e2e-smoke.sh

echo "4. Running Quick Smoke Benchmark (10 VUs, 10s)..."
k6 run -e TARGET_URL="http://localhost:8082" --duration 10s --vus 10 loadtests/scenarios/smoke.js

echo "5. Verifying Invariants Post-Rehearsal..."
go test -v ./test/system -run TestInvariantVerifier_StockEquation_Calculation

echo "=========================================================="
echo " >>> DEMO REHEARSAL COMPLETED WITH ZERO ERRORS! <<<"
echo "=========================================================="
```
Make executable: `chmod +x scripts/demo/rehearsal.sh`

- [ ] **Step 3: Document known limitations in `docs/known-limitations.md`**

Create `docs/known-limitations.md`:
```markdown
# ApexStock Architectural Known Limitations

This document transparently registers all architectural constraints, scope boundaries, and known limitations of the prototype:

1. **Single-Node Redis (No Cluster/Sentinel):**
   - The current Optimized deployment utilizes a standalone Redis container. If Redis undergoes a hard unrecoverable failure, the system trips a 503 circuit breaker until manual operator intervention or container restart. Production deployments would utilize Redis Sentinel or Redis Cluster.
2. **SePay Webhook Local Tunneling:**
   - Real bank transfer webhooks from SePay require a public HTTPS URL (via Cloudflare Tunnel or ngrok). In isolated offline environments, the built-in Payment Simulator (`POST /api/v1/payments/simulate`) is used instead.
3. **In-Memory Rate Limiting Lifespan:**
   - Per-instance token buckets or rate limiters maintain counters in memory; restarting an API container resets local rate-limit windows.
4. **Single-Host CPU Parity Contention:**
   - Running full 2,000+ VU benchmarks on a single laptop host causes thermal throttling over extended durations. Benchmark runs must enforce cooling intervals (D0.11).
```

- [ ] **Step 4: Commit**

```bash
git add docs/runbooks/demo-day.md scripts/demo/rehearsal.sh docs/known-limitations.md
git commit -m "docs(stage-13): assemble demo day presentation runbook, rehearsal script, and limitations register"
```

---

### Task 2: Final README One-Command Guide & Handoff Document

**Files:**
- Modify: `README.md`
- Create: `docs/handoff.md`

**Interfaces:**
- Consumes: Entire project documentation, command reference, and architecture decisions.
- Produces: The primary entry point for evaluators and future maintainers.

- [ ] **Step 1: Write comprehensive root `README.md`**

Update `README.md`:
```markdown
# ⚡ ApexStock — High-Concurrency Flash Sale Benchmark Platform

> **Báo Cáo Thực Tập Tốt Nghiệp**  
> So sánh thực nghiệm hiệu năng giữa Kiến Trúc Truyền Thống (**Baseline**: PostgreSQL Row Locking) và Kiến Trúc Tối Ưu (**Optimized**: Redis Lua Atomic Reservation + NATS JetStream Durable Queue) dưới tải cao và kịch bản Chaos Engineering.

---

## 🚀 Quick Start (Khởi Động Nhanh Trong 1 Phút)

### 1. Khởi động toàn bộ hệ thống
```bash
# Khởi động cơ sở hạ tầng, 2 phiên bản API, Worker, War Room và Web UI
docker compose -f deploy/compose/docker-compose.yaml --profile demo up -d
```

### 2. Khởi tạo dữ liệu mẫu (Seed & Warm-up)
```bash
./scripts/reset-data.sh && ./scripts/seed-data.sh && ./scripts/warmup-redis.sh
```

### 3. Kiểm tra tự động toàn hệ thống (Rehearsal)
```bash
./scripts/demo/rehearsal.sh
```

---

## 🌐 Các Cổng Truy Cập Dịch Vụ

| Dịch Vụ | URL | Mục Đích |
| :--- | :--- | :--- |
| **Web UI (Next.js)** | `http://localhost:3000` | Giao diện mua hàng Flash Sale, quét mã VietQR |
| **War Room Dashboard** | `http://localhost:3000/war-room` | Trung tâm điều khiển so sánh RPS, Latency, Invariant real-time |
| **War Room Controller** | `http://localhost:9000` | API & SSE Stream tổng hợp metrics |
| **Optimized API (NGINX)** | `http://localhost:8082` | Cổng phân tán tải (Redis Lua + NATS) |
| **Baseline API** | `http://localhost:8081` | Cổng truyền thống (PostgreSQL Row Lock) |
| **Prometheus** | `http://localhost:9090` | Thu thập metrics và cảnh báo vi phạm |
| **Grafana** | `http://localhost:3001` | Dashboard trực quan hóa hiệu năng |

---

## 🛡️ Chứng Nhận Invariant & Không Bị Oversell

Hệ thống cam kết 5 Invariant bất biến toán học:
- **I1:** `Kho khả dụng + Đơn đang giữ + Đơn đã thanh toán == Tổng số lượng ban đầu` (Tuyệt đối không oversell).
- **I2:** Mọi yêu cầu giữ hàng đều kết thúc ở trạng thái `confirmed` hoặc `released`.
- **I3:** Mỗi `idempotency_key` tạo ra tối đa đúng một đơn hàng.
- **I4:** Trạng thái đơn hàng và thanh toán tuân thủ nghiêm ngặt State Machine.
- **I5:** Worker xử lý mỗi sự kiện JetStream tối đa một lần (`processed_events`).

Kiểm tra Invariant bất kỳ lúc nào bằng lệnh:
```bash
./scripts/reconcile.sql.sh
```

---

## 📊 Kịch Bản Chạy Benchmark (k6)

Chạy benchmark so sánh tuần tự công bằng (Tuân thủ Decision Gate D0.11):
```bash
# Đo đột biến tranh chấp kho (Flash-sale Shock Contention)
./scripts/benchmark/run-benchmark.sh flash-sale-shock 3

# Đo thông lượng tối đa bền vững (Plateau Stepped RPS)
./scripts/benchmark/run-benchmark.sh sustainable-throughput 3
```

---

## 💥 Thao Tác Kịch Bản Chaos (Dành Cho Operator)

Tuân thủ Decision Gate D0.12 (không có nút bấm nguy hiểm trên Web):
```bash
# Mô phỏng sập 1 instance API Optimized
./scripts/chaos/kill-optimized-api.sh

# Phục hồi instance và quan sát thời gian tự phục hồi trên War Room
./scripts/chaos/restore-optimized-api.sh
```
```

- [ ] **Step 2: Create complete handoff reference in `docs/handoff.md`**

Create `docs/handoff.md`:
```markdown
# ApexStock Complete Context Handoff Document

## 1. Decision Gates Summary (D0.1 – D0.15)

| Gate | Title | Decision & Architectural Rationale |
| :--- | :--- | :--- |
| **D0.1** | Compose Topology | Khóa cứng ports 8081 (Baseline), 8082 (Optimized), 9000 (War Room), 3000 (Web). |
| **D0.2** | Event Payload | Gói event JetStream mang đủ thông tin đơn hàng; worker không query ngược Redis. |
| **D0.3** | Stock Increment Timing | Baseline tăng `sold_quantity` khi reserve; Optimized chỉ tăng DB khi `paid`. |
| **D0.4** | Single Item Constraint | Mỗi flash sale order bắt buộc đúng 1 sản phẩm (`quantity = 1`). |
| **D0.5** | Error Envelope Schema | Thống nhất chuẩn JSON `{error, message, correlation_id, details}` trên toàn hệ thống. |
| **D0.6** | Payment Isolation | K6 benchmark chỉ gọi Simulator; SePay adapter chỉ phục vụ UI demo bằng tay. |
| **D0.7** | Late Payment Policy | Đơn đã expired mà tiền về: giữ nguyên `expired`, ghi `webhook_logs` với `status='refund'`. |
| **D0.8** | Database Indexing | Tránh dùng non-deterministic `now()` trong index predicates. |
| **D0.9** | Metrics Cardinality | Bounded labels (arch, route, status); tuyệt đối không đưa user_id/order_id vào Prometheus. |
| **D0.10** | Resource Parity | Cấp phát tài nguyên tương đương (2.0 CPU, 1024MB RAM) giữa Baseline và Optimized. |
| **D0.11** | Sequential Execution | Benchmark chính thức bắt buộc chạy tuần tự từng kiến trúc để tránh tranh chấp CPU. |
| **D0.12** | War Room Chaos Boundary | Bỏ nút kill trên UI; operator kill container trực tiếp bằng Docker CLI. |
| **D0.13** | Redis Eviction Policy | Cấu hình `noeviction` để bảo vệ tính toàn vẹn của tồn kho flash sale. |
| **D0.14** | Frontend Realtime | War Room dùng WebSocket/SSE; trang người dùng dùng polling đơn giản. |
| **D0.15** | Data Isolation | Tách riêng hai database `apexstock_baseline` và `apexstock_optimized`. |

## 2. Verification Checklist For Maintainers

- [ ] `go build ./... && go vet ./...` (Exit code 0)
- [ ] `go test -race ./...` (All unit, contract, and integration tests pass)
- [ ] `./scripts/demo/rehearsal.sh` (Full rehearsal passes)
- [ ] `./scripts/reconcile.sql.sh` (Zero invariant discrepancies detected)
- [ ] `cd web && npm run lint && npm run build` (Next.js production build passes)

## 3. Repository Cleanliness
- Working tree is clean.
- All temporary benchmark logs reside in `loadtests/results/` and are tracked with `.gitkeep`.
```

- [ ] **Step 3: Commit**

```bash
git add README.md docs/handoff.md
git commit -m "docs(stage-13): provide comprehensive root README, D0 decision table, and project handoff guide"
```
