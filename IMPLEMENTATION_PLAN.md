# ApexStock — Implementation Plan

> **Language:** English  
> **Status:** Implementation Blueprint (Historical execution plan; empirical benchmarks recorded in benchmark reports)  
> **Scope:** Go Backend, Baseline Architecture, Optimized Architecture, PostgreSQL, Redis/Lua, NATS JetStream, Next.js Frontend, SePay Payment Integration, Observability, War Room Dashboard, QA Test Pyramid, Load/Concurrency/Chaos/System Testing  
> **Target Environment:** Single host (Intel i5-10500H, 6C/12T, 16GB RAM, NVMe SSD, Docker Compose)  
> **Guiding Principle:** Never claim performance metrics (RPS, latency, recovery duration) as empirical facts before benchmarks are executed and raw evidence is preserved.

---

## 1. Scope and Reading Guide

The project repository contains design documentation, locked contract specifications (`docs/decisions`, `docs/contracts`, `docs/domain`, `docs/benchmark`), agent profiles, `CLAUDE.md`, and `go.mod` (module `apexstock`). File paths referenced throughout this plan designate target architectural locations across the implementation lifecycle.

This plan maintains the architectural decisions defined in the Software Design Document (SDD) and specifications while resolving ambiguities through **Decision Gate D0** and the foundational rulings listed in Section 4. Every development stage proceeding after D0 assumes relevant contracts are formally captured in `docs/decisions/` or within the decision registers of the SDD.

The implementation follows a **rapidly testable vertical slice** roadmap in strict sequence:

```text
health → migration/seed → Baseline order → Optimized reservation
→ payment simulator/restock → metrics → frontend → War Room
→ contract/concurrency/system tests → benchmark → chaos/soak/demo
```

This project does not deploy a distributed multi-node cloud production cluster. Containerized processes simulate the competing architectural topologies on a single physical host; NATS, Redis, PostgreSQL, and API replicas operate under single-node constraints matching physical hardware boundaries.

---

## 2. Goals, Non-Goals, and Success Criteria

### 2.1. Implementation Goals

1. Execute both Baseline and Optimized architectures on a single host via Docker Compose.
2. Utilize identical workloads, synthetic datasets, execution windows, hardware resource quotas, and k6 load generation harnesses for parity.
3. Support the complete end-to-end order lifecycle: request submission, payment processing, expiration sweeps, business failure handling, and inventory restocking.
4. Strictly enforce non-negotiable system invariants:
   - Zero inventory overselling.
   - Idempotent `reserve`, `release`, and `confirm` primitives.
   - Zero duplicate order creation across client retries and identical idempotency keys.
   - Strict enforcement of valid order and payment state transitions.
   - At-least-once message redelivery without duplicate inventory debits or credits.
5. Provide high-provenance telemetry, structured logging with correlation IDs, and dashboards that explicitly display disconnected/stale indicators instead of fabricating metrics.
6. Verify functional correctness across unit, integration, and concurrency suites prior to load execution; benchmarks are deemed valid only when invariant and reconciliation checks pass.
7. Deliver a polished frontend web application adhering to the design system: minimalist dark theme, aurora animated heroes, spotlight card interactions, spinning borders, and dark/light switching.
8. Implement an operator-driven War Room dashboard and safe chaos drills, defaulting to terminating single API instances rather than corrupting Redis or PostgreSQL directly.

### 2.2. Non-Goals

- Constructing ancillary e-commerce features (logistics tracking, shipment carriers, customer reviews, mobile applications, multi-tenant tenancy).
- Provisioning production Kubernetes clusters, multi-region Redis clusters, PostgreSQL streaming replication, or multi-host cloud topologies.
- Emitting live HTTP requests to third-party payment providers during automated k6 runs; all load testing relies on the internal payment simulator.
- Treating hypothetical target metrics from design documents as accepted delivery benchmarks.
- Treating visual UI quality as proof of backend performance.

### 2.3. Project-Level Acceptance Criteria

| Domain | Mandatory Acceptance Conditions |
|---|---|
| **Platform Bootstrap** | Docker Compose boots cleanly, distinguishing liveness from readiness; migrations and seeding are 100% deterministic and repeatable. |
| **Baseline** | Employs transactional `SELECT ... FOR UPDATE`; does not use external caching or queues to distort the baseline; correct 201 orders and 409 out-of-stock handling. |
| **Optimized** | Atomic inventory reservation via Lua scripts; durable message publication to NATS JetStream; worker redeliveries cause zero duplicate side effects. |
| **Inventory** | Zero negative stock or overselling; repeated release/confirm invocations are idempotent no-ops; post-benchmark reconciliation passes with zero drift. |
| **Payments** | Payment simulator engine with configurable outcome ratios; provider-neutral state machine; optional external SePay webhook adapter; duplicate/out-of-order/late payments audited with zero stock corruption. |
| **Observability** | Prometheus metrics clearly distinguish `2xx`, `409`, `429`, and `5xx`; exports `p50`/`p95`/`p99` latency histograms, queue backlog, and non-resetting oversell counters. |
| **Frontend** | Does not hardcode benchmark figures; renders real-time stale/loading/reconnect states; supports `prefers-reduced-motion` and keyboard accessibility. |
| **War Room** | Delivers telemetry snapshots and real-time SSE streaming; sequential benchmark execution; operator-driven chaos via CLI without dangerous browser-exposed kill buttons. |
| **Benchmarks** | Both architectures run sequentially under strict resource parity; outputs raw JSON summaries, host metadata, and percentile distributions; invalidated if invariant tests fail. |
| **Reliability** | API instance termination recovers gracefully; Redis/DB/queue failure scenarios validated with dedicated local drill scripts and runbooks. |

---

## 3. Non-Negotiable Technical Principles

### 3.1. Inventory & Order/Payment Invariants

Every stage modifying inventory must validate the following invariants across unit tests, integration suites, concurrency stress tests, and post-run reconciliation scripts.

**Invariant I1 — Non-Negative Stock & Conservation Equation**

```text
stock_available >= 0
successful_paid_quantity + active_reserved_quantity + stock_available = total_quantity
```

This mathematical equation accounts for asynchronous projection delays where PostgreSQL `sold_quantity` is updated via background workers. Reconciliation queries must strictly evaluate true business states rather than masking discrepancies.

**Invariant I2 — Reservation Idempotency**

- A single `reservation_id` / `order_id` cannot reserve inventory more than once.
- The initial `release` restores reserved quantity; subsequent invocations are logged no-ops.
- The initial `confirm` transitions the reservation to confirmed; subsequent calls do not alter stock balances.
- A `release` executed after a `confirm` must never restock inventory.
- A `confirm` executed after a `release` or expiration must never transition an expired order into paid.

**Invariant I3 — Request Idempotency**

- Concurrent or sequential requests sharing identical `(user_id, idempotency_key)` and identical payload fingerprints return identical responses.
- Conflicting payloads sharing the same key must return HTTP 409 Conflict without creating orders.
- Concurrent requests using the same key must allow only a single reservation attempt.
- Failures during intermediate execution stages must allow deterministic retry or reconciliation without duplicating orders.

**Invariant I4 — State Machine Integrity**

Order lifecycle transitions exclusively adhere to:

```text
pending_payment → paid
pending_payment → payment_failed
pending_payment → expired
pending_payment → cancelled
```

Under no circumstances may an order transition `paid → pending_payment`, `paid → expired`, or `payment_failed → paid`. Out-of-order or duplicate payment webhook events must never revert an order state.

**Invariant I5 — Durable Asynchronous Processing**

- NATS at-least-once delivery guarantees that messages arrive at least once; consumers must be strictly idempotent.
- Database unique constraints and `processed_events` inbox records must guard against duplicate worker execution.
- "Eventual consistency" cannot be cited as justification for dropping confirmed reservations or losing messages.

### 3.2. Fairness Between Baseline and Optimized Architectures

Both implementations must strictly adhere to:

- Identical external API request schemas, validation rules, and response contracts.
- Identical product catalog configurations, initial stock values, `max_per_user` limits, and idempotency generation algorithms.
- Identical synthetic traffic distributions (valid requests, out-of-stock rejections, rate limits, retries, and payment outcomes).
- Identical pre-run warm-up periods, execution durations, database resets, run counts, and collection windows.
- Sequential benchmark execution ensuring k6, Prometheus, Grafana, and backend services never contend for host CPU or memory bandwidth.
- Identical total hardware resource budgets locked prior to benchmarking; an architecture must never receive disproportionate CPU or RAM quotas.

### 3.3. Single-Host Resource Discipline

- Development relies on minimal profiles: PostgreSQL, Redis, NATS, a single API instance, worker daemon, and k6 on demand.
- The observability profile (Prometheus/Grafana) is engaged during benchmarks and demonstrations, avoiding background resource overhead during standard local development.
- Baseline and Optimized benchmarks must never execute simultaneously.
- The Redis inventory cache must employ `noeviction` to prevent evicting active inventory or idempotency keys under memory pressure.
- Resource limits represent locked testing budgets rather than raw system capacity. Any adjustment must be formally documented in measurement metadata.

---

## 4. Decision Gate D0 — Locked Pre-Implementation Decisions

The following architectural decisions resolve conflicting specifications identified in design artifacts. All decisions are locked and binding.

| ID | Conflict / Ambiguity | Architectural Impact | Locked Resolution | Governing Artifact |
|---|---|---|---|---|
| **D0.1** | Dual descriptions: Optimized was specified as async `Redis → NATS → worker → DB`, yet elsewhere noted as writing orders synchronously to DB before publish. Conflicting response codes (`201` vs `202`). | Determines durability, latency, and crash recovery semantics. | **LOCKED:** API returns HTTP `201 Created` immediately upon durable publication to disk-backed NATS JetStream. Sub-5ms latency is a design hypothesis, not an acceptance condition. | `D0-decisions-summary.md`, API Contract |
| **D0.2** | Conflicting URL routes across drafts: `/flash-sale/order`, `/optimized/orders`, `/baseline/orders`. | Risk of measuring disparate endpoints during load tests. | **LOCKED:** Unified endpoint `POST /api/v1/flash-sale/orders` across both architectures with identical schemas, isolated by port (`:8081` Baseline, `:8082` Optimized). | `openapi.yaml`, API Contract |
| **D0.3** | Baseline decrements `sold_quantity` at order time; Optimized increments `sold_quantity` only upon payment, while reserving stock atomically in Redis. | Inconsistent stock reconciliation formulas and success metrics. | **LOCKED:** Explicitly separate `available`, `reserved`, and `paid` quantities. Reconciliation verifies unified conservation equations matching each architecture's storage model. | `inventory-state-model.md` |
| **D0.4** | Multi-item orders require atomic all-or-nothing reservations, yet Lua scripts only addressed single-item keys. | Incomplete multi-item reservations cause partial stock leaks. | **LOCKED:** Each flash sale transaction is strictly limited to a single item (`quantity=1`). Multi-item support is deferred to future roadmaps. | Lua scripts, Contract Tests |
| **D0.5** | `cleanup_expired.lua` attempted to parse set members to deduce user keys across retried orders. | Incorrect key parsing causes double-release or stock leaks. | **LOCKED:** Reservations explicitly encapsulate `reservation_id`, `item_id`, `user_id`, `quantity`, and `state`. Confirmed orders delete reservations before cleanup sweeps. | Redis Schema, Sweeper |
| **D0.6** | SePay documentation mentioned HMAC/checksum signatures without providing definitive payload structures or credentials. | Risk of invalid webhook parsing or fabricated credentials. | **LOCKED:** Implement an internal payment simulator first. External SePay adapters are isolated with provider-specific verification requiring verified credentials. | Payment ADR, Secrets Policy |
| **D0.7** | `refund_pending`, `late_payment`, and `webhook_logs` appeared in workflows without complete state machine definitions. | Late payments could corrupt completed orders or lose audit logs. | **LOCKED:** Payments arriving after order expiration maintain the order in `expired` state, creating a `webhook_logs` record with status `refund` for manual settlement. | Payment State Machine |
| **D0.8** | Database migration proposed a partial index `WHERE expires_at < now()`, which PostgreSQL rejects because `now()` is not immutable. | Migration failure upon initialization. | **LOCKED:** Standard B-tree index on `expires_at`; cleanup queries filter explicitly with `WHERE expires_at < now()`. | PostgreSQL Migrations |
| **D0.9** | Conflicting resource limit specifications across documentation (varying API replica counts and memory limits). | Compromises benchmark fairness and War Room metrics. | **LOCKED:** Lock hardware budgets in `benchmark-matrix.yaml`. Enforce sequential benchmarking with dedicated `dev`, `demo`, and `benchmark` Compose profiles. | `benchmark-matrix.yaml` |
| **D0.10** | Unverified target ranges in documents (`200–500 RPS`, `3,000–8,000 RPS`, `<500ms p99`). | Misrepresents design hypotheses as verified results. | **LOCKED:** All documentation and dashboards label pre-run figures as hypotheses. Empirical figures are published only after benchmark execution. | Benchmark Reports |
| **D0.11** | Proposal to run k6 against Baseline and Optimized concurrently in the War Room contradicts fairness rules. | CPU and I/O contention skew comparative performance. | **LOCKED:** Official benchmarks run strictly sequentially. Concurrent visualization is restricted to demo modes and explicitly labeled as non-evaluative. | War Room Runner |
| **D0.12** | War Room UI proposed calling the Docker daemon socket directly to terminate containers during chaos demos. | Severe host security exposure and unconstrained chaos actions. | **LOCKED:** Eliminate browser-facing kill endpoints. Operators execute chaos drills directly via host CLI scripts; War Room UI monitors recovery via health telemetry. | Security Review |
| **D0.13** | Setting `maxmemory-policy allkeys-lru` threatens active inventory and idempotency keys under load. | Redis evicts active stock tracking keys, corrupting inventory. | **LOCKED:** Configure Redis with `noeviction` for inventory instances. Trigger memory headroom alerts if utilization nears budget. | Redis Config |
| **D0.14** | Conflicting route parameters (`:id` Express syntax vs Next.js App Router `[id]`). Real-time stock polling vs WebSockets. | Framework routing errors and broken client streams. | **LOCKED:** Next.js App Router uses standard `[id]` folder structures. War Room utilizes Server-Sent Events / WebSockets; storefront pages use efficient polling. | Frontend Architecture |
| **D0.15** | Baseline and Optimized sharing a single database risks data pollution during demo runs. | Cross-architecture reconciliation failures and dirty state. | **LOCKED:** Single PostgreSQL container hosting two isolated databases: `apexstock_baseline` and `apexstock_optimized`, sharing migrations and seed data. | `postgres-init.sh` |

---

## 5. Target Project Directory Layout

```text
.
├── cmd/
│   ├── api-baseline/           # Baseline HTTP server (:8081)
│   ├── api-optimized/          # Optimized HTTP server (:8082)
│   ├── worker/                 # NATS JetStream asynchronous persistence worker
│   ├── scheduler/              # Inventory expiration sweeper daemon
│   └── warroom-controller/     # Telemetry aggregator & SSE stream hub (:9000)
├── internal/
│   ├── domain/                 # Order, payment, and inventory state machines
│   ├── httpapi/                # Shared HTTP handlers, validation, error envelopes
│   ├── baseline/               # Direct PostgreSQL row-locking implementation
│   ├── optimized/              # Redis Lua atomic reservation & NATS publisher
│   ├── payments/               # Payment simulator, state machine, SePay adapter
│   ├── persistence/            # PostgreSQL repositories & Redis Lua integrations
│   ├── messaging/              # NATS JetStream connection & consumer wrappers
│   └── observability/          # Prometheus metrics registry & logging middleware
├── db/
│   ├── migrations/             # Idempotent SQL schema migrations
│   └── seed/                   # Deterministic test dataset manifests
├── scripts/
│   ├── benchmark/              # Sequential k6 benchmark runners & metadata scripts
│   ├── chaos/                  # Operator chaos injection drills (CLI-driven)
│   ├── demo/                   # Automated system rehearsal & health checkers
│   └── reconcile.sql.sh        # SQL inventory reconciliation verification tool
├── deploy/
│   ├── compose/                # Docker Compose files (dev, demo, benchmark profiles)
│   ├── docker/                 # Production multi-stage Dockerfiles
│   ├── nginx/                  # NGINX reverse proxy & load balancer configuration
│   ├── prometheus/             # Prometheus scrapers & alert rule configurations
│   └── grafana/                # Pre-provisioned Grafana dashboards & datasources
├── web/
│   ├── src/app/                # Next.js App Router (catalog, orders, war-room)
│   ├── src/components/         # Reusable design system & War Room telemetry widgets
│   ├── src/hooks/              # Polling hooks & connection resilience handlers
│   └── src/lib/                # Strongly-typed API client & idempotency managers
├── loadtests/
│   ├── scenarios/              # k6 load test suites (flash-sale-shock, sustainable, etc.)
│   ├── lib/                    # Synthetic user generators & assertion libraries
│   └── results/                # Empirical benchmark output directory (JSON summaries)
├── test/
│   ├── contract/               # HTTP API contract & security boundary test suites
│   ├── concurrency/            # High-concurrency race condition test suites
│   └── system/                 # Invariant verifier & chaos resilience integration tests
├── docs/
│   ├── decisions/              # Architectural Decision Records (D0.1–D0.15)
│   ├── runbooks/               # Operator execution runbooks (demo, chaos, recovery)
│   └── benchmark/              # Methodology specifications & empirical benchmark reports
└── README.md                   # Quickstart guide & repository documentation
```

---

## 6. Stage Overview and Dependencies

| Stage | Name | Core Objectives | Prerequisites | Deliverable Vertical Slice |
|---:|---|---|---|---|
| **0** | Contracts & Decision Gates | Lock D0 ambiguities, API schemas, state models, and benchmark matrices. | None | Complete formal contracts; zero ambiguous assumptions. |
| **1** | Platform Bootstrap & Health | Provision multi-profile Compose, isolated databases, and liveness endpoints. | Stage 0 | Functional container stack with accurate `/healthz` and `/readyz`. |
| **2** | Schema, Migrations & Seeding | Establish SQL migrations, Redis keyspace, and deterministic test data. | Stage 1 | Reproducible database state across baseline and optimized schemas. |
| **3** | Baseline Order Slice | Implement PostgreSQL row-locking order workflow and error contracts. | Stage 2 | End-to-end Baseline order placement on port `:8081`. |
| **4** | Optimized Reservation Slice | Implement Redis Lua reservations, NATS JetStream publishing, and worker. | Stage 2, 3, D0.1–D0.5 | End-to-end Optimized order placement on port `:8082`. |
| **5** | Payment Simulator & Restock | Implement payment state machines, expiration scheduler, and restock logic. | Stage 3, 4 | Complete order lifecycle with deterministic payment outcomes. |
| **6** | SePay Webhook Adapter | Implement optional SePay QR generation, webhook parsing, and refund audit. | Stage 5, D0.6–D0.7 | Real-world bank transfer support via VietQR in the web UI. |
| **7** | Observability & Telemetry | Zero-dependency Prometheus metrics, structured logs, and Grafana dashboard. | Stage 3, 4, 5 | Real-time telemetry distinguishing business logic from failures. |
| **8** | Frontend Storefront Flow | Deliver responsive Next.js storefront, checkout flows, and theme system. | Stage 5, Contracts | Interactive user shopping experience for flash sale products. |
| **9** | War Room Dashboard | Real-time SSE telemetry aggregator, comparison views, and status cards. | Stage 7, 8, D0.11–D0.12 | Live operational dashboard for benchmark evaluation. |
| **10** | QA Test Pyramid & Invariants | Comprehensive contract, concurrency, race, and mathematical invariant tests. | Stages 3–9 | Automated verification proving zero overselling and data safety. |
| **11** | Fair k6 Benchmark Harness | Sequential benchmark runner, metadata provenance, and empirical reports. | Stage 7, 10, D0.9–D0.10 | Validated performance comparisons under fair resource parity. |
| **12** | Chaos, Soak & Resilience | Operator chaos drills, component failure tests, and memory leak evaluation. | Stage 7, 9, 10, 11 | Empirical validation of high availability and fault recovery. |
| **13** | Demo Hardening & Handoff | Rehearsal scripts, evidence pack packaging, and context documentation. | Stage 11, 12 | Turnkey demonstration platform with comprehensive documentation. |

---

## 7. Stage-by-Stage Specifications

### Stage 0: Contracts, Decision Gates, and Architecture Blueprint

- **Objective:** Convert SDD specifications into enforceable, testable architectural contracts; formally lock all ambiguities identified in Decision Gate D0.
- **Deliverables:**
  - ADR documents D0.1–D0.15 inside `docs/decisions/`.
  - Common OpenAPI 3.0 contract (`docs/contracts/openapi.yaml`) and markdown specification.
  - State machine models for inventory and order lifecycles (`docs/domain/`).
  - Benchmark configuration matrix (`docs/benchmark/benchmark-matrix.yaml`).
- **Target Files:** `docs/decisions/*.md`, `docs/contracts/openapi.yaml`, `docs/domain/*.md`, `docs/benchmark/benchmark-matrix.yaml`.
- **Acceptance Criteria:** All D0 conflict points resolved without ambiguous assumptions; single request schema shared across architectures; performance metrics explicitly labeled as design hypotheses.
- **Handoff:** Provides binding contracts, invariants, and resource budgets for all subsequent development stages.

---

### Stage 1: Platform Bootstrap, Docker Compose, and Health Slice

- **Objective:** Construct a single-host, fully reproducible container environment with reliable health and readiness probes prior to business logic implementation.
- **Deliverables:**
  - Root `go.mod` module (`apexstock`), service entry points in `cmd/`.
  - Next.js web application skeleton in `web/`.
  - Multi-stage Dockerfiles for all microservices.
  - Docker Compose configuration supporting `dev`, `demo`, `benchmark`, and `observability` profiles.
  - Granular `/healthz` (liveness) and `/readyz` (readiness) HTTP endpoints.
  - `.env.example`, initial `Makefile`, and minimal `README.md`.
- **Target Files:** `cmd/*/main.go`, `deploy/compose/*.yaml`, `internal/httpapi/health.go`, `internal/config/config.go`.
- **Acceptance Criteria:** Clean repository checkout boots without errors; Compose initializes without requiring real secrets; PostgreSQL automatically provisions `apexstock_baseline` and `apexstock_optimized` databases; readiness probes fail until backing databases respond; zero container ports exposed to the host by default.
- **Handoff:** Establishes standardized container hostnames, networking, and configuration environments.

---

### Stage 2: Schema Migrations, Redis Keyspace, and Deterministic Seeding

- **Objective:** Implement reproducible database schemas and deterministic initial datasets ensuring identical starting states across all benchmark runs.
- **Deliverables:**
  - Idempotent SQL migrations for users, products, flash sale items, orders, and payments.
  - Core database repository interfaces and query bindings.
  - Deterministic data seed manifests (50,000 synthetic users, active flash sale event, 10,000 units stock).
  - Automated database reset (`scripts/reset-data.sh`) and seed (`scripts/seed-data.sh`) tools.
  - Redis cache warm-up tool (`scripts/warmup-redis.sh`) and SQL reconciliation verifier (`scripts/reconcile.sql.sh`).
- **Target Files:** `db/migrations/*.sql`, `internal/persistence/postgres/*.go`, `scripts/*.sh`.
- **Acceptance Criteria:** Migrations apply cleanly to both isolated databases; seeding creates identical record IDs in both environments; index predicates avoid non-deterministic SQL functions (D0.8); reset tool completely restores initial baseline states; initial reconciliation check passes.
- **Handoff:** Schema contracts and seed datasets become mandatory fixtures for order services and load generators.

---

### Stage 3: Baseline Order Vertical Slice

- **Objective:** Construct the Baseline flash sale order workflow using traditional pessimistic locking (`SELECT ... FOR UPDATE`) in PostgreSQL.
- **Deliverables:**
  - Common `POST /api/v1/flash-sale/orders` endpoint bound to port `:8081`.
  - Transactional order service utilizing pessimistic row locks, validating stock balances and `max_per_user` limits.
  - Consistent error envelope mapping `400 Bad Request`, `404 Not Found`, `409 Conflict`, `429 Too Many Requests`, and `500 Internal Error`.
  - Transaction-level advisory lock serializing same-key idempotency claims.
- **Target Files:** `cmd/api-baseline/main.go`, `internal/baseline/*.go`, `internal/httpapi/order_handler.go`, `internal/persistence/postgres/order_repository.go`.
- **Acceptance Criteria:** Valid requests create orders in `pending_payment` status and decrement available stock; concurrent requests never produce negative stock or overselling; identical idempotency keys replay responses without duplicating orders; transaction rollbacks leave zero partial state.
- **Handoff:** Baseline serves as the comparative performance control and functional oracle for test suites.

---

### Stage 4: Optimized Reservation Slice (Redis/Lua, NATS JetStream, Worker)

- **Objective:** Implement the Optimized flash sale architecture using atomic Redis Lua scripts for inventory reservation, NATS JetStream for durable queuing, and background asynchronous database workers.
- **Deliverables:**
  - Common `POST /api/v1/flash-sale/orders` endpoint bound to port `:8082` returning HTTP 201 upon durable message publication.
  - Atomic Lua scripts: `reserve_order.lua`, `confirm_order.lua`, and `release_order.lua`.
  - Worker daemon (`cmd/worker`) consuming JetStream events with database deduplication via `processed_events`.
  - Fast-failing circuit breaker returning HTTP 503 (`service_unavailable`) during Redis outages.
- **Target Files:** `cmd/api-optimized/main.go`, `cmd/worker/main.go`, `internal/optimized/*.go`, `internal/persistence/redis/lua/*.lua`, `internal/messaging/*.go`.
- **Acceptance Criteria:** Concurrent reservations never exceed inventory bounds; same-key requests reserve stock exactly once; worker redeliveries cause zero duplicate orders; Redis failures return 503 immediately; post-execution reconciliation passes with zero drift.
- **Handoff:** Provides the high-throughput asynchronous order processing path for benchmarking.

---

### Stage 5: Payment Simulator, Expiration Sweeper, and Stock Restoration

- **Objective:** Implement the complete payment and order lifecycle across both architectures, guaranteeing accurate stock restoration upon order failure, cancellation, or expiration.
- **Deliverables:**
  - Unified payment state machine (`EvaluateTransition`) enforcing valid state transitions.
  - Payment simulator engine (`POST /api/v1/payments/simulate`) supporting configurable success/failure distributions.
  - Background expiration sweeper daemon (`cmd/scheduler`) releasing unpaid reservations.
  - Late-payment handler retaining `expired` order states and logging `webhook_logs.status='refund'` (D0.7).
- **Target Files:** `internal/payments/*.go`, `cmd/scheduler/main.go`, `internal/messaging/payment_handlers.go`.
- **Acceptance Criteria:** Payment failures and expirations restore stock exactly once; duplicate payment callbacks are idempotent; late payments never resurrect expired orders; SQL reconciliation passes across all lifecycle outcomes.
- **Handoff:** Delivers full lifecycle simulation capabilities for storefront testing and load scenarios.

---

### Stage 6: External SePay Payment Webhook Adapter

- **Objective:** Enable real-world VietQR payment workflows within the web interface while strictly isolating credentials and eliminating external dependencies during load testing.
- **Deliverables:**
  - SePay provider adapter with environment variable configuration and timing-safe signature verification.
  - Webhook endpoint (`POST /api/v1/webhooks/sepay`) and payment initiation endpoint (`POST /api/v1/payments/sepay/initiate`).
  - Webhook audit logging (`webhook_logs`) and local demonstration runbook.
- **Target Files:** `internal/payments/sepay/*.go`, `internal/httpapi/sepay_webhook_handler.go`, `docs/runbooks/sepay-local-demo.md`.
- **Acceptance Criteria:** Zero hardcoded credentials in the repository; valid webhook payloads update payment records exactly once; invalid signatures are rejected with HTTP 401 without modifying stock; k6 load test scripts never invoke external payment gateways.
- **Handoff:** Allows live banking demonstration flows in the Next.js storefront.

---

### Stage 7: Observability, Metrics, Logs, and Telemetry

- **Objective:** Implement a zero-dependency Prometheus metrics registry, structured logging with correlation IDs, and analytical dashboards.
- **Deliverables:**
  - Zero-dependency Prometheus client exposing `/metrics` with `arch` and normalized `route` labels.
  - Metrics catalog distinguishing `2xx`, `409`, `429`, and `5xx` statuses, latency percentiles (`p50`/`p95`/`p99`), and non-resetting oversell counters (`apexstock_oversell_total`).
  - Structured JSON logging with `X-Correlation-ID` tracing across HTTP and NATS boundaries.
  - Provisioned Grafana dashboard (`apexstock.json`) and Prometheus alert rules (`alerts.yml`).
- **Target Files:** `internal/observability/*.go`, `deploy/prometheus/*.yml`, `deploy/grafana/provisioning/**/*`, `docs/observability/metric-catalog.md`.
- **Acceptance Criteria:** End-to-end request tracing via correlation IDs; business rejections (409/429) clearly separated from system failures; latency percentiles correctly calculated; Grafana automatically provisioned upon startup.
- **Handoff:** Supplies telemetry metrics for the War Room dashboard and k6 benchmark analysis.

---

### Stage 8: Frontend Storefront Flow and Design System

- **Objective:** Construct a modern, accessible web storefront supporting product browsing, checkout, architecture selection, and live payment status tracking.
- **Deliverables:**
  - Next.js App Router storefront (`web/src/app/`) featuring catalog browsing, item details (`[id]`), and order status views.
  - Modern design system: minimalist dark theme, animated aurora hero, spotlight cards, and theme switching.
  - Strongly-typed API client (`web/src/lib/api.ts`) with deterministic idempotency key storage and adaptive polling hooks.
  - Architecture toggle allowing live switching between Baseline (`:8081`) and Optimized (`:8082`) backends.
- **Target Files:** `web/src/app/**/*`, `web/src/components/**/*`, `web/src/lib/*.ts`, `web/src/hooks/*.ts`.
- **Acceptance Criteria:** Seamless manual demonstration flow from catalog through order placement and payment; duplicate clicks prevented via idempotency tokens; clear disconnect/stale error handling; zero linting or build errors.
- **Handoff:** Delivers an interactive user-facing application for manual feature demonstrations.

---

### Stage 9: War Room Dashboard and Operational Telemetry

- **Objective:** Deliver an operational control center visualizing comparative performance metrics, system health, and fault recovery in real time.
- **Deliverables:**
  - War Room Controller daemon (`cmd/warroom-controller`) scraping metrics from both APIs and streaming updates via SSE.
  - Web dashboard (`/war-room`) featuring side-by-side architecture telemetry panels, latency indicators, and oversell invariant status badges.
  - Sequential benchmark execution state machine enforcing Decision Gate D0.11.
  - Operator chaos scripts (`scripts/chaos/*.sh`) for CLI-based failure injection (adhering to D0.12).
- **Target Files:** `cmd/warroom-controller/main.go`, `internal/warroom/*.go`, `web/src/app/war-room/**/*`, `scripts/chaos/*.sh`.
- **Acceptance Criteria:** Real-time telemetry streaming over SSE with automatic reconnection handling; zero container-kill endpoints exposed to the browser; operator-initiated CLI failures detected within polling intervals; clear display of system invariants.
- **Handoff:** Serves as the primary operational interface for live benchmark and chaos demonstrations.

---

### Stage 10: QA Test Pyramid, Concurrency, and Invariant Verifier

- **Objective:** Validate functional correctness, concurrency safety, and mathematical invariants across both architectures before running load tests.
- **Deliverables:**
  - Automated Invariant Verifier (`test/system/invariant_verifier.go`) validating Invariants I1–I5 across both database schemas.
  - Common API contract test suite (`test/contract/order_contract_test.go`) validating status codes, error payloads, and correlation IDs.
  - High-concurrency race condition tests (100+ concurrent workers contending for identical stock and idempotency keys).
  - Security boundary tests verifying payload limits, SQL injection protection, and absence of browser chaos endpoints.
- **Target Files:** `test/contract/*.go`, `test/system/invariant_verifier*.go`, `test/concurrency/*.go`.
- **Acceptance Criteria:** Zero invariant violations across unit, contract, and concurrency suites; zero data races under `go test -race ./...`; zero overselling under heavy lock contention; benchmarks blocked if invariant tests fail.
- **Handoff:** Provides the testing gates required to authorize performance benchmarking.

---

### Stage 11: Fair k6 Benchmark Harness and Comparative Analysis

- **Objective:** Execute fair, reproducible performance benchmarks comparing Baseline and Optimized architectures under identical resource constraints.
- **Deliverables:**
  - 5 standardized k6 load scenarios: `smoke.js`, `flash-sale-shock.js`, `sustainable-throughput.js`, `payment-restock.js`, and `soak.js`.
  - Sequential benchmark runner (`scripts/benchmark/run-benchmark.sh`) enforcing automated pre-run resets, seeding, warm-up, cooldown intervals, and post-run invariant verifications.
  - Host and execution provenance logger (`metadata.json`).
  - Empirical report templates and measurement provenance documentation.
- **Target Files:** `loadtests/scenarios/*.js`, `loadtests/lib/*.js`, `scripts/benchmark/run-benchmark.sh`, `docs/benchmark/*.md`.
- **Acceptance Criteria:** Identical test scripts target both architectures with identical request payloads; benchmarks run strictly sequentially with 30s thermal cooldowns; runs automatically rejected if metadata is missing or invariants fail; output captures throughput, latencies, rejection rates, and system errors.
- **Handoff:** Produces verified empirical evidence packs documenting comparative performance.

---

### Stage 12: Chaos Engineering, Resilience, Soak, and System Testing

- **Objective:** Validate high availability, fault tolerance, and automated recovery mechanisms under deliberate failure injection.
- **Deliverables:**
  - Automated resilience test suite (`test/system/chaos_resilience_test.go`) covering Redis outages, NATS redeliveries, and worker crash scenarios.
  - Operator chaos drill scripts (`scripts/chaos/redis-restart-drill.sh`, `kill-optimized-api.sh`) with signal trap cleanups.
  - End-to-end smoke verification runner (`scripts/system/e2e-smoke.sh`).
  - Comprehensive recovery checklist (`docs/runbooks/recovery-checklist.md`).
- **Target Files:** `scripts/chaos/*.sh`, `scripts/system/*.sh`, `test/system/chaos_*_test.go`, `docs/runbooks/*.md`.
- **Acceptance Criteria:** Redis outages fail fast with HTTP 503 without corrupting stock; NATS redeliveries prior to ACK cause zero duplicate orders; API failures recover automatically upon container restart; chaos scripts guarantee cleanup on exit; post-chaos invariant verification passes.
- **Handoff:** Delivers empirical resilience evidence and verified operational disaster recovery runbooks.

---

### Stage 13: Demo Hardening, Evidence Pack Packaging, and Handoff

- **Objective:** Finalize project deliverables into an easily reproducible demonstration package with complete operational documentation.
- **Deliverables:**
  - Automated full rehearsal script (`scripts/demo/rehearsal.sh`) testing the entire system workflow.
  - Demo day execution guide (`docs/runbooks/demo-day.md`) and known limitations register (`docs/known-limitations.md`).
  - Empirical benchmark reports populated exclusively with measured data (`docs/benchmark/results/`).
  - Comprehensive Quickstart guide (`README.md`) and operational handoff documentation (`docs/handoff.md`).
- **Target Files:** `README.md`, `docs/handoff.md`, `docs/known-limitations.md`, `docs/runbooks/demo-day.md`, `scripts/demo/rehearsal.sh`.
- **Acceptance Criteria:** An external reviewer can clone the repository, launch the stack, execute seed scripts, and run benchmarks using documented commands; all figures in final reports trace to empirical test runs; working directory is clean of untracked build artifacts.
- **Handoff:** Project handoff complete; all architectural deliverables and empirical findings preserved.

---

## 8. Condensed Critical Path

```text
D0 Contracts & Decisions (Stage 0)
  ↓
Platform Bootstrap & Compose Health (Stage 1)
  ↓
Database Migrations & Deterministic Seeding (Stage 2)
  ↓
Baseline Order Slice (PostgreSQL Row Locks) (Stage 3)
  ↓
Optimized Order Slice (Redis Lua + NATS JetStream + Worker) (Stage 4)
  ↓
Payment Simulator, Expiration Sweeper & Restock (Stage 5)
  ↓
Common Observability, Structured Logs & Metrics (Stage 7)
  ├── Frontend Storefront Flow (Stage 8)
  └── War Room Dashboard & Telemetry Hub (Stage 9)
  ↓
QA Test Pyramid & Mathematical Invariant Verifier (Stage 10)
  ↓
Fair Sequential k6 Benchmark Harness (Stage 11)
  ↓
Chaos Drills, Soak Testing & Fault Recovery (Stage 12)
  ↓
Demo Hardening, Empirical Evidence Packaging & Final Handoff (Stage 13)
```

The fastest path to a reliable technical demonstration proceeds through Stages 1–5, 7, and 10 with minimal UI before finalizing frontend widgets. Correctness and invariant verification are mandatory prerequisites for load benchmarking.

---

## 9. Comprehensive Testing Matrix

| Level | Baseline Validation | Optimized Validation | Execution Schedule | Acceptance Gate |
|---|---|---|---|---|
| **Unit / Domain** | Order/payment state transitions, validation logic | State machines, Lua payload generation, error mapping | On every commit | 100% Pass |
| **API Contract** | Common order schema, status codes, error envelopes | Common order schema, JetStream event payloads | On API changes | 100% Pass |
| **Integration** | PostgreSQL transaction isolation, row locking | Redis Lua scripts, NATS JetStream, worker consumers | Prior to merge | 100% Pass |
| **Concurrency** | Same-item contention, same-key duplicate requests | High-concurrency reserve, release, worker deduplication | Prior to benchmark | Zero overselling or duplicate orders |
| **Lifecycle** | Payment success, failure, cancellation, expiration | Payment success, failure, cancellation, stock restock | From Stage 5 onward | Invariant equation verified |
| **Storefront E2E** | Manual checkout flow via Baseline endpoint | Manual checkout flow via Optimized endpoint | During rehearsals | Clean user flow without errors |
| **Load Benchmark** | Standardized k6 suites (`sustainable`, `shock`) | Standardized k6 suites (`sustainable`, `shock`) | Stage 11 | Resource parity, post-run invariant pass |
| **Chaos Drills** | Single-point-of-failure observation | API instance kill, Redis downtime, worker restart | Stage 12 | Graceful recovery, zero lost orders |
| **Soak Testing** | Memory stability under sustained baseline traffic | Worker memory, queue lag, Redis connection stability | Stage 12 | Zero memory leaks or inventory drift |
| **Security** | Input sanitization, body size limits, SQL injection | Webhook signature verification, localhost chaos boundaries | Prior to demo | Zero security vulnerabilities |

---

## 10. Stage Merge and Review Checklist

- [ ] Reviewed relevant Decision Gate ADRs; no unauthorized semantic modifications introduced.
- [ ] Automated tests cover both happy paths and failure scenarios.
- [ ] Invariant verifier confirms compliance with Invariants I1–I5 for any inventory modification.
- [ ] Asynchronous event handlers verify at-least-once message deduplication and crash recovery.
- [ ] Payment processing logic verifies duplicate callbacks, late payments, and refund audits.
- [ ] Telemetry metrics maintain low label cardinality and distinguish business logic from system outages.
- [ ] Frontend interfaces feature clear loading, error, stale, and reduced-motion states without hardcoded metrics.
- [ ] Chaos mechanisms rely strictly on operator CLI scripts; zero container-kill endpoints exposed to web clients.
- [ ] Verification commands execute cleanly (`gofmt`, `go vet`, `go test -race`, `npm run build`).
- [ ] Context handoff documentation updated with modified files, configurations, and known limitations.

---

## 11. Locked Global Architecture Decisions

1. Both architectures expose the identical endpoint `POST /api/v1/flash-sale/orders` with identical schemas; Baseline binds to `:8081`, Optimized binds to `:8082`.
2. Redis Lua scripts process single-item orders (`quantity=1`); multi-item transactions are deferred.
3. The internal payment simulator serves as the primary load test engine; external SePay integration is isolated for manual demonstrations.
4. Late payments arriving after order expiration retain the order in `expired` state, logging a refund entry in `webhook_logs`.
5. Storefront user interfaces utilize adaptive polling; the War Room dashboard utilizes real-time Server-Sent Events.
6. The platform executes entirely on a single physical host managed by Docker Compose.
7. PostgreSQL represents the permanent source of truth for orders and payments; Redis manages ephemeral high-concurrency inventory state.
8. NATS JetStream operating with on-disk storage is mandatory for the Optimized architecture; APIs return HTTP 201 Created upon durable queue publish.
9. k6 serves as the primary load testing engine; benchmarks execute strictly sequentially under pre-locked hardware budgets.
10. The War Room web interface contains zero container-kill buttons; operators execute chaos drills directly via host CLI scripts.
11. Historical throughput targets in design drafts (`3,000+ RPS`, `p95 < 500ms`) are strictly hypotheses until confirmed by empirical benchmark runs.
12. External payment adapters avoid speculative signature implementations and require verified configuration contracts.

---

*End of Implementation Plan.*
