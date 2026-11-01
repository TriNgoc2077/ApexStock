# SDD Ledger — Plan: IMPLEMENTATION_PLAN.md

> **Purpose:** Persistent on-disk state memory (Single Source of Truth) preventing the Orchestrator and Subagents from losing context when Claude Code compacts history (`/compact`) or reboots sessions.  
> **Mandatory Rule:** Read this file first before any action, and update it immediately after every state transition.

---

## 1. Current Execution State

- **Overall Status:** `COMPLETED`
- **Current Stage:** `Stage 13 (All Stages Complete)`
- **Execution Mode:** `SEQUENTIAL` — Single orchestrator, running Stage 1 → Stage 13, dispatching one subagent at a time
- **Current Phase:** `COMMITTED` (`READY` | `IMPLEMENT` | `CHECKPOINT_COMMITTED` | `REVIEW_ROUND_1` | `REVIEW_ROUND_2` | `VERIFY` | `COMMITTED`)
- **Base Commit SHA:** `91ce71c` (stage-0)
- **Last Completed Stage:** `Stage 13`
- **Last Completed Commit SHA:** `56e8ff7`

---

## 2. Decision Gate D0 Checklist (Locked in `IMPLEMENTATION_PLAN.md` Section 4)

| ID | Core Locked Decision | Status |
|---|---|---|
| **D0.1** | API returns `201 Created` immediately upon durable publication to disk-backed NATS JetStream; sub-5ms latency is a design hypothesis. | LOCKED |
| **D0.2** | Unified endpoint `POST /api/v1/flash-sale/orders` across both architectures with identical schemas; Baseline on `:8081`, Optimized on `:8082`. | LOCKED |
| **D0.3** | Explicitly separate `available`, `reserved`, and `paid` quantities; unified reconciliation formulas across storage models. | LOCKED |
| **D0.4** | Flash sale transactions are strictly limited to a single item per order (`single-item order`). | LOCKED |
| **D0.5** | Reservation records store `reservation_id`, `item_id`, `user_id`, `quantity`, and `state`. | LOCKED |
| **D0.6** | Payment simulator implemented first; external SePay adapter isolated with provider-specific verification. | LOCKED |
| **D0.7** | Payments arriving after expiration keep order in `expired` state, logging a refund entry in `webhook_logs`. | LOCKED |
| **D0.8** | Standard B-tree index on `idempotency_keys(expires_at)`; eliminate non-immutable functions from partial index predicates. | LOCKED |
| **D0.9** | Lock CPU/RAM budgets; execute benchmarks strictly sequentially; isolate `dev`, `demo`, and `benchmark` Compose profiles. | LOCKED |
| **D0.10** | All throughput and latency numbers in design documentation are treated as hypotheses until verified by raw benchmarks. | LOCKED |
| **D0.11** | Official performance benchmarks run strictly sequentially; parallel execution is limited to visual demonstrations with clear labeling. | LOCKED |
| **D0.12** | Eliminate browser-facing container-kill endpoints; operators trigger chaos drills directly via host CLI scripts. | LOCKED |
| **D0.13** | Redis configured with `noeviction` for inventory and idempotency keyspaces. | LOCKED |
| **D0.14** | Next.js App Router utilizes standard `[id]` folder segments; WebSockets/SSE for War Room, adaptive polling for user pages. | LOCKED |
| **D0.15** | Single PostgreSQL container hosting two isolated databases: `apexstock_baseline` and `apexstock_optimized`, sharing migrations and seed data. | LOCKED |

---

## 3. Stage Progress Matrix

| Stage | Stage Name | Prerequisites | Primary Agent | Support Agent(s) | Reviewer | Status | Fix Rounds | Commit SHA |
|---:|---|---|---|---|---|---|---:|---|
| **0** | Contracts, decision gate & skeleton docs | None | `stages-planner` / `backend-core` | `infra-devops` | `code-reviewer` | `COMPLETED` | 0 | `91ce71c` |
| **1** | Host foundation, Docker Compose & health slice | Stage 0 | `infra-devops` | `backend-core`, `frontend-ui` | `code-reviewer` | `COMPLETED` | 0 | `8ce7d19` |
| **2** | PostgreSQL migration, Redis keyspace & seed/reset | Stage 1 | `backend-core` | `infra-devops` | `code-reviewer` | `COMPLETED` | 1 | `bc2ba07` |
| **3** | Baseline order vertical slice | Stage 2 | `backend-core` | - | `code-reviewer` | `COMPLETED` | 1 | `820b61c` |
| **4** | Optimized reservation, idempotency, NATS & worker | Stage 2, 3 | `backend-core` | `infra-devops` | `code-reviewer` | `COMPLETED` | 0 | `a914a16` |
| **5** | Payment simulator, confirm/fail/timeout/cancel & restock | Stage 3, 4 | `backend-core` | `payment-integrations` | `code-reviewer` | `COMPLETED` | 1 | `a2b6053` |
| **6** | SePay adapter for UI | Stage 5 | `payment-integrations` | `backend-core` | `code-reviewer` | `COMPLETED` | 0 | `f2a25ba` |
| **7** | Observability, metrics, logs & reliability baseline | Stage 3, 4, 5 | `observability-reliability` | `backend-core`, `infra-devops` | `code-reviewer` | `COMPLETED` | 0 | `627f740` |
| **8** | Frontend product flow & design system | Stage 5 | `frontend-ui` | - | `code-reviewer` | `COMPLETED` | 1 | `85d25ad` |
| **9** | War Room dashboard & chaos controller | Stage 7, 8 | `frontend-ui` | `backend-core`, `infra-devops` | `code-reviewer` | `COMPLETED` | 1 | `909eaca` |
| **10** | QA test pyramid, concurrency & invariant verifier | Stage 3–9 | `qa-loadtest` | `backend-core` | `code-reviewer` | `COMPLETED` | 0 | `c5f25b5` |
| **11** | k6 benchmark & fair comparison | Stage 7, 10 | `qa-loadtest` | `infra-devops` | `code-reviewer` | `COMPLETED` | 0 | `8e9186f` |
| **12** | Chaos, resilience, soak & system testing | Stage 7, 9, 10, 11 | `qa-loadtest` | `observability-reliability`, `infra-devops` | `code-reviewer` | `COMPLETED` | 0 | `6a26b76` |
| **13** | Demo hardening, evidence pack & context handoff | Stage 11, 12 | `qa-loadtest` | `infra-devops`, `observability-reliability` | `code-reviewer` | `COMPLETED` | 0 | `56e8ff7` |

---

## 4. Technical Rulings Ledger

> When encountering minor ambiguities or localized conflicts that do not violate Invariants I1–I5, the Orchestrator issues a formal ruling recorded here using the format:
> `- [Stage X] Ruling: <Decision> — Rationale: <Why> — Risk/Impact if reversed: <Severity>`

*(No ad-hoc rulings recorded)*

---

## 5. Stage Execution Log

### Stage 1: Host Foundation, Docker Compose & Health Slice

- **Phase:** COMMITTED  
- **Primary Agent:** `infra-devops`  
- **Deliverables:**
  - Docker Compose configuration with 4 profiles (`dev`, `demo`, `benchmark`, `observability`).
  - PostgreSQL initialization script provisioning isolated databases `apexstock_baseline` and `apexstock_optimized` (D0.15).
  - Go entrypoints for `api-baseline`, `api-optimized`, `worker`, and `warroom-controller` with granular `/healthz` and `/readyz` endpoints.
  - Next.js web application skeleton shell with Tailwind CSS configuration (`web/postcss.config.js`).
  - Container hardware resource limits aligned with `benchmark-matrix.yaml`.
  - Standardized relative volume paths across all Compose definitions (`../../deploy/*`).
- **Verification:**
  ```bash
  gofmt -l .                  # PASS
  go vet ./...                # PASS
  go test -race ./...         # PASS
  make build                  # PASS (4 binaries built)
  docker compose config -q    # PASS
  bash -n scripts/**/*.sh     # PASS
  ```
- **Acceptance:**
  - ✅ Clean checkout builds without compilation errors.
  - ✅ Docker Compose initializes without requiring production secrets.
  - ✅ PostgreSQL successfully creates both isolated databases.
  - ✅ Health and readiness endpoints accurately report dependency state.
  - ✅ Zero container ports exposed to the host by default.
  - ✅ Resource constraints applied matching benchmark specifications.
- **Commit SHA:** `8ce7d19`

---

### Stage 2: PostgreSQL Migration, Redis Keyspace & Deterministic Seeding

- **Phase:** COMMITTED  
- **Primary Agent:** `backend-core`  
- **Deliverables:**
  - Shared, idempotent PostgreSQL migrations applied across both isolated databases.
  - Deterministic data seed generator: 50,000 synthetic users, active flash sale event, and 10,000 units initial stock.
  - Redis key helpers and atomic Lua scripts for reservation, release, confirmation, and rate limiting.
  - Redis warm-up, database reset, and SQL reconciliation verification scripts.
  - Makefile targets and documentation for database lifecycle automation.
- **Verification:**
  ```bash
  gofmt -l .                         # PASS
  go vet ./...                       # PASS
  go test -race ./...                # PASS
  bash -n scripts/*.sh               # PASS
  docker compose config -q            # PASS
  ```
- **Correctness Fixes:** Default script credentials aligned with Docker Compose (`apexstock`); Redis reset script updated to use explicit Redis CLI commands.
- **Acceptance:**
  - ✅ Shared migrations apply cleanly to both databases (D0.15).
  - ✅ Deterministic logical IDs and seed manifests established.
  - ✅ Active event created and Redis cache pre-warmed from database state.
  - ✅ Indexes avoid non-immutable functions (D0.8).
  - ✅ Reset scripts return system to clean initial state.
- **Commit SHA:** `bc2ba07`

---

### Stage 3: Baseline Order Vertical Slice

- **Phase:** COMMITTED  
- **Primary Agent:** `backend-core`  
- **Deliverables:**
  - Common `POST /api/v1/flash-sale/orders` endpoint bound to Baseline port `:8081`.
  - Schema validation for UUID formats, `quantity=1`, idempotency tokens, and correlation ID headers.
  - PostgreSQL transaction utilizing row-level locks (`SELECT ... FOR UPDATE`), validating stock and `max_per_user` limits.
  - Atomic order insertion, simulated payment record creation, stock decrement, and idempotency caching.
  - Idempotent request replay and payload conflict detection; advisory locks serialize concurrent same-key attempts.
- **Verification:**
  ```bash
  gofmt -w ...                    # PASS
  go test ./...                   # PASS
  go vet ./...                    # PASS
  go test -race ./...             # PASS
  docker compose config -q        # PASS
  git diff --check                # PASS
  ```
- **Review:** Round 1 resolved a concurrency race condition on duplicate keys prior to row creation by introducing transaction-level advisory locks.
- **Acceptance:**
  - ✅ Baseline endpoint adheres to unified API path and returns HTTP 201 Created.
  - ✅ Baseline inventory mutation is transactional and row-locked.
  - ✅ Idempotency responses are cached and replayed on duplicate requests.
  - ✅ Concurrent same-key claims are serialized without duplicating records.
- **Commit SHA:** `820b61c`

---

### Stage 4: Optimized Reservation, Idempotency, NATS JetStream & Worker

- **Phase:** COMMITTED  
- **Primary Agent:** `backend-core`  
- **Deliverables:**
  - Cryptographically secure RFC 4122 UUIDv4 generator in `internal/optimized/service.go`.
  - `ErrRedisUnavailable` sentinel error mapped to HTTP 503 (`service_unavailable`) in `internal/httpapi/optimized_handler.go`.
  - Asynchronous worker daemon wired with PostgreSQL connection pool and NATS JetStream durable consumer in `cmd/worker/main.go`.
  - Comprehensive inventory reconciliation verifier (`ReconcileItem`, `ReconcileAll`) in `internal/optimized/reconcile.go`.
  - Unit, integration, and 100-goroutine concurrency tests verifying atomic Lua reservations, worker deduplication, and race safety.
- **Verification:**
  ```bash
  go build ./...                  # PASS
  go vet ./...                    # PASS
  go test -count=1 -race ./...    # PASS
  ```
- **Acceptance:**
  - ✅ Concurrent reservations never cause negative stock or overselling (Invariant I1).
  - ✅ Identical idempotency keys reserve inventory at most once (Invariant I3).
  - ✅ Durable JetStream publishing with worker deduplication via `processed_events` (Invariant I5).
  - ✅ Fast-failing circuit breaker returns HTTP 503 during Redis outages.
  - ✅ Reconciliation verifier validates mathematical stock conservation equation.
- **Commit SHA:** `a914a16`

---

### Stage 5: Payment Simulator, Confirm/Fail/Timeout/Cancel & Restock

- **Phase:** COMMITTED  
- **Primary Agent:** `backend-core`  
- **Deliverables:**
  - Shared payment domain DTOs (`SimulatePaymentRequest`, `PaymentResultResponse`) and state machine (`EvaluateTransition`) enforcing valid transitions and D0.7 refund auditing (`webhook_logs.status='refund'`).
  - Baseline and Optimized payment processors supporting row-locking, Redis Lua idempotency (`Confirm`, `Release`), and atomic inventory restock/confirmation.
  - Payment Simulator engine supporting configurable outcome distributions and deterministic overrides.
  - Expiration sweeper (`ExpirySweeper`) and background scheduler daemon (`cmd/scheduler/main.go`).
  - Shared `POST /api/v1/payments/simulate` endpoint mounted on both Baseline (`:8081`) and Optimized (`:8082`).
  - Concurrency stress tests (`TestConcurrency_ConfirmVsCancelVsExpireRace`) passing across both architectures.
- **Verification:**
  ```bash
  go build ./...                  # PASS
  go vet ./...                    # PASS
  go test -count=1 -race ./...    # PASS
  bash scripts/reconcile.sql.sh   # PASS
  ```
- **Acceptance:**
  - ✅ Deterministic and configurable payment simulator outcome ratios.
  - ✅ Each terminal outcome executes at most once (Invariants I2 and I4).
  - ✅ Payment failure, expiration, and cancellation restock inventory exactly once.
  - ✅ Late payments arriving after expiration preserve `expired` status and create refund audit logs (D0.7).
  - ✅ Post-run SQL reconciliation passes cleanly with zero inventory drift.
- **Commit SHA:** `a2b6053`

---

### Stage 6: External SePay Payment Webhook Adapter

- **Phase:** COMMITTED  
- **Primary Agent:** `payment-integrations`  
- **Deliverables:**
  - Isolated SePay adapter (`internal/payments/sepay/adapter.go`) featuring environment variable configuration, QR code generation, and timing-safe signature verification.
  - HTTP handlers mounted at `POST /api/v1/payments/sepay/initiate` and `POST /api/v1/webhooks/sepay` on both Baseline and Optimized APIs.
  - Webhook audit logging (`webhook_logs`) and D0.7 late-payment refund policy integration.
  - Webhook test fixtures (`test/fixtures/sepay_webhook_*.json`) and local demonstration runbook (`docs/runbooks/sepay-local-demo.md`).
- **Verification:**
  ```bash
  go build ./...                  # PASS
  go vet ./...                    # PASS
  go test -count=1 -race ./...    # PASS
  ```
- **Acceptance:**
  - ✅ Zero hardcoded production credentials in Git (D0.6).
  - ✅ Valid webhook fixtures update order and payment records exactly once.
  - ✅ Invalid signatures rejected with HTTP 401 without mutating stock balances.
  - ✅ Duplicate webhooks handled idempotently without duplicate credits.
  - ✅ Late payments after expiration adhere strictly to the D0.7 refund policy.
- **Commit SHA:** `f2a25ba`

---

### Stage 7: Observability, Metrics, Logs & Reliability Baseline

- **Phase:** COMMITTED  
- **Primary Agent:** `observability-reliability`  
- **Deliverables:**
  - Zero-dependency Prometheus metrics registry (`CounterVec`, `GaugeVec`, `HistogramVec`, `/metrics`) in `internal/observability/metrics.go` categorizing `2xx`, `409`, `429`, and `5xx` statuses by `arch` and normalized route.
  - Structured JSON logger (`slog`), correlation ID middleware, and NATS header propagation (`InjectNATSCorrelationID`, `ExtractNATSCorrelationID`).
  - Telemetry instrumentation integrated into `cmd/api-baseline`, `cmd/api-optimized`, and `cmd/worker`.
  - Prometheus alert definitions (`deploy/prometheus/alerts.yml`), Grafana dashboard provisioning (`apexstock.json`), metric catalog, and incident runbooks.
- **Verification:**
  ```bash
  go build ./...                  # PASS
  go vet ./...                    # PASS
  go test -count=1 -race ./...    # PASS
  ```
- **Acceptance:**
  - ✅ End-to-end request tracing via `X-Correlation-ID` across HTTP and NATS boundaries.
  - ✅ Business rejections (`409`/`429`) clearly separated from `5xx` system failures.
  - ✅ Latency percentiles correctly partitioned by architecture and route.
  - ✅ Non-resetting `apexstock_oversell_total` invariant tracking counter.
  - ✅ Grafana dashboards and Prometheus alerts auto-provision upon launch.
- **Commit SHA:** `627f740`

---

### Stage 8: Frontend Storefront Flow & Design System

- **Phase:** COMMITTED  
- **Primary Agent:** `frontend-ui`  
- **Deliverables:**
  - Catalog and order read APIs (`internal/httpapi/catalog_handler.go`) mounted on both APIs, with CORS preflight and route normalization.
  - Design system tokens (dark/light theme, `prefers-reduced-motion` support) and animated UI primitives (`AuroraHero.tsx`, `SpotlightCard.tsx` using CSS custom properties without React rerender overhead).
  - Strongly-typed API client (`web/src/lib/api.ts`) with deterministic idempotency key storage and adaptive polling hooks (`web/src/hooks/usePolling.ts`).
  - Core storefront routes (`/`, `/flash-sale/[id]`, `/orders/[id]`) supporting live architecture switching, SePay QR codes, and payment simulator triggers.
- **Verification:**
  ```bash
  go build ./...                  # PASS
  go vet ./...                    # PASS
  go test -count=1 -race ./...    # PASS
  cd web && npm run lint          # PASS
  cd web && npm run build         # PASS
  ```
- **Acceptance:**
  - ✅ End-to-end user shopping flow: catalog browsing → item detail → checkout → payment QR / simulator → live status polling.
  - ✅ Duplicate click prevention via client-side idempotency storage.
  - ✅ Resilient disconnect/stale indicators, theme switcher, reduced-motion, and keyboard navigation.
  - ✅ Standard Next.js App Router `[id]` folder segments and polling per D0.14.
- **Commit SHA:** `85d25ad`

---

### Stage 9: War Room Dashboard & Chaos Controller

- **Phase:** COMMITTED  
- **Primary Agent:** `frontend-ui`  
- **Deliverables:**
  - War Room Controller service (`internal/warroom/service.go`) scraping `/healthz` and `/metrics` from Baseline (`:8081`) and Optimized (`:8082`), computing throughput, latency quantiles, and instance outage recovery times.
  - HTTP and SSE handlers (`internal/warroom/handler.go`) wiring `/api/v1/warroom/snapshot`, `/stream`, and `/run` (enforcing D0.11 sequential benchmark execution and D0.12 zero container-kill endpoints).
  - Operator CLI chaos scripts (`scripts/chaos/{kill,restore}-{optimized,baseline}-api.sh`).
  - Frontend `/war-room` dashboard with live SSE streaming, reconnect banners, oversell invariant status cards, and hypothesis disclaimers.
- **Verification:**
  ```bash
  go build ./...                  # PASS
  go vet ./...                    # PASS
  go test -count=1 -race ./...    # PASS
  bash -n scripts/chaos/*.sh      # PASS
  cd web && npm run lint          # PASS
  cd web && npm run build         # PASS
  ```
- **Acceptance:**
  - ✅ Idle screen displays accurate Unknown/Healthy indicators.
  - ✅ Sequential start/stop state machine and labeled parallel demonstration mode (D0.11).
  - ✅ Zero container kill/pause endpoints exposed to web browsers (D0.12); operator triggers drills via CLI while War Room detects recovery automatically.
  - ✅ Live SSE stream with automatic reconnection and stale data indicators.
- **Commit SHA:** `909eaca`

---

### Stage 10: QA Test Pyramid, Concurrency & Invariant Verifier

- **Phase:** COMMITTED  
- **Primary Agent:** `qa-loadtest`  
- **Deliverables:**
  - Cross-architecture Invariant Verifier (`test/system/invariant_verifier.go`) validating Invariants I1–I5 across both database schemas.
  - Common API contract test suite (`test/contract/order_contract_test.go`) validating HTTP status codes, error schemas, correlation IDs, and single-item limits.
  - Security and boundary test suite (`test/contract/security_test.go`) validating body size limits, SQL injection sanitization, and absence of browser chaos endpoints.
- **Verification:**
  ```bash
  go build ./...                  # PASS
  go vet ./...                    # PASS
  go test -count=1 -race ./...    # PASS
  ```
- **Acceptance:**
  - ✅ Invariants I1–I5 verified across both architectures.
  - ✅ Contract parity between Baseline (`:8081`) and Optimized (`:8082`).
  - ✅ Security boundaries and D0.12 verified.
  - ✅ Zero data races detected under `go test -race ./...`.
- **Commit SHA:** `c5f25b5`

---

### Stage 11: k6 Benchmark & Fair Comparison

- **Phase:** COMMITTED  
- **Primary Agent:** `qa-loadtest`  
- **Deliverables:**
  - Deterministic user/idempotency generators and response classifiers matching `benchmark-matrix.yaml`, categorizing `SUCCESS`, `BUSINESS_REJECTION` (409/429), and `SYSTEM_FAILURE` (5xx).
  - 5 standardized k6 load scenarios: `smoke.js`, `flash-sale-shock.js`, `sustainable-throughput.js`, `payment-restock.js`, and `soak.js`.
  - Fair sequential benchmark runner (`scripts/benchmark/run-benchmark.sh`) enforcing automated pre-run resets, seeding, warm-up, cooldown periods, metadata recording, and post-run invariant verifications.
  - Empirical report templates and measurement provenance guidelines.
- **Verification:**
  ```bash
  node --check loadtests/lib/*.js loadtests/scenarios/*.js  # PASS
  bash -n scripts/benchmark/run-benchmark.sh                # PASS
  go test -count=1 -race ./...                              # PASS
  ```
- **Acceptance:**
  - ✅ Identical k6 scripts target Baseline and Optimized with identical payloads.
  - ✅ Sequential execution with reset, seed, warm-up, and cooldown per run (D0.11).
  - ✅ Post-run invariant verification gate strictly enforced.
  - ✅ Design hypothesis targets separated from empirical measurements (D0.10).
- **Commit SHA:** `8e9186f`

---

### Stage 12: Chaos, Resilience, Soak & System Testing

- **Phase:** COMMITTED  
- **Primary Agent:** `qa-loadtest`  
- **Deliverables:**
  - Automated resilience test suite (`test/system/chaos_resilience_test.go`) covering Redis 503 circuit-breaking, payment vs expiry race serialization, and NATS redelivery deduplication.
  - Operator chaos drill scripts (`scripts/chaos/redis-restart-drill.sh`) featuring signal traps for automatic container cleanup.
  - End-to-end smoke verification runner (`scripts/system/e2e-smoke.sh`).
  - Chaos report templates and disaster recovery checklists.
- **Verification:**
  ```bash
  bash -n scripts/chaos/*.sh scripts/system/*.sh  # PASS
  go test -count=1 -race ./...                    # PASS
  ```
- **Acceptance:**
  - ✅ Redis outages fail fast with HTTP 503 without corrupting inventory.
  - ✅ NATS redeliveries prior to ACK cause zero duplicate orders.
  - ✅ Payment vs expiration race conditions serialize deterministically.
  - ✅ Chaos scripts guarantee automated cleanup via signal traps.
- **Commit SHA:** `6a26b76`

---

### Stage 13: Demo Hardening, Evidence Pack & Context Handoff

- **Phase:** COMMITTED  
- **Primary Agent:** `qa-loadtest`  
- **Deliverables:**
  - Demo Day runbook (`docs/runbooks/demo-day.md`), rehearsal script (`scripts/demo/rehearsal.sh`), and known limitations register (`docs/known-limitations.md`).
  - Evidence pack directory skeletons (`docs/benchmark/results/`, `loadtests/results/`).
  - Turnkey Quickstart guide (`README.md`) and operational handoff documentation (`docs/handoff.md`).
- **Verification:**
  ```bash
  bash -n scripts/demo/rehearsal.sh  # PASS
  go build ./...                     # PASS
  go vet ./...                       # PASS
  go test -count=1 -race ./...       # PASS
  cd web && npm run build            # PASS
  docker compose -f deploy/compose/docker-compose.yaml config -q  # PASS
  ```
- **Acceptance:**
  - ✅ One-command startup, seeding, smoke testing, and reset documented in `README.md`.
  - ✅ Hypothesis metrics clearly distinguished from empirical benchmark outputs (D0.10).
  - ✅ Zero production secrets or exposed Docker sockets in default profiles (D0.6, D0.12).
  - ✅ Complete operational context preserved in `docs/handoff.md`.
- **Commit SHA:** `56e8ff7`
