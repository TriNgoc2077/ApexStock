# ⚡ ApexStock — High-Concurrency Flash Sale Benchmark Platform

> **Graduation Capstone Project**  
> An empirical performance evaluation comparing a Traditional Architecture (**Baseline**: PostgreSQL Row Locking) against an Optimized Architecture (**Optimized**: Redis Lua Atomic Reservation + NATS JetStream Durable Queue) under extreme traffic contention and Chaos Engineering scenarios.

---

## 🚀 Quick Start (1-Minute Setup)

### 1. Launch the Entire System
```bash
# Starts core infrastructure, both API variants, Worker daemon, War Room Controller, and Next.js Web UI
docker compose -f deploy/compose/docker-compose.yaml --profile demo up -d
```

### 2. Seed & Warm Up Sample Data
```bash
./scripts/reset-data.sh && ./scripts/seed-data.sh && ./scripts/warmup-redis.sh
```

### 3. Run Automated System Rehearsal
```bash
./scripts/demo/rehearsal.sh
```

---

## 🌐 Service Ports & Access Points

| Service | URL | Purpose |
| :--- | :--- | :--- |
| **Web UI (Next.js)** | `http://localhost:3000` | Flash sale shopping storefront & VietQR payment checkout |
| **War Room Dashboard** | `http://localhost:3000/war-room` | Real-time control center comparing RPS, latency, and invariants |
| **War Room Controller** | `http://localhost:9000` | Telemetry aggregation API & SSE event stream |
| **Optimized API (NGINX)** | `http://localhost:8082` | Distributed load gateway (Redis Lua + NATS JetStream) |
| **Baseline API** | `http://localhost:8081` | Traditional architecture endpoint (PostgreSQL Row Lock) |
| **Prometheus** | `http://localhost:9090` | Metrics scraper and invariant violation alerting |
| **Grafana** | `http://localhost:3001` | Pre-provisioned analytical performance dashboards |

---

## 🛡️ Invariant Verification & Zero Overselling Guarantee

ApexStock strictly enforces 5 mathematical system invariants:
- **I1:** `Available Stock + Reserved Stock + Paid Quantity == Initial Total Quantity` (Zero overselling guaranteed).
- **I2:** Every stock reservation request terminates in either a `confirmed` or `released` state.
- **I3:** Each `idempotency_key` creates at most one order.
- **I4:** Order and payment lifecycles strictly adhere to a deterministic State Machine.
- **I5:** Worker processes each JetStream message at most once (`processed_events` deduplication).

Verify all invariants at any time using the automated reconciliation tool:
```bash
./scripts/reconcile.sql.sh
```

---

## 🧪 Smoke Test & System Verification

Before running heavy load tests, verify end-to-end health, API contracts, and system invariants:

```bash
# 1. Full System E2E Smoke Test (Health checks -> Seed -> Order Checkout -> Metrics -> Invariants)
./scripts/system/e2e-smoke.sh

# 2. k6 Smoke Benchmark (10 VUs, 30s across Baseline & Optimized sequentially)
./scripts/benchmark/run-benchmark.sh smoke 1

# 3. API Contract & Security Verification (Validation, Error Envelopes, SQLi/Payload limits, D0.12 check)
go test -v ./test/contract/...
```

---

## 📊 Benchmark Scenarios (k6)

Run fair, sequential load tests adhering to Decision Gate D0.11 (`./scripts/benchmark/run-benchmark.sh <scenario> <runs>`):

```bash
# 1. Smoke Test — Connectivity, contract & invariant pre-check (10 VUs, 30s)
./scripts/benchmark/run-benchmark.sh smoke 1

# 2. Flash-Sale Shock — Hot-spot inventory contention shock test (500 VUs, 60s)
./scripts/benchmark/run-benchmark.sh flash-sale-shock 3

# 3. Sustainable Throughput — Stepped RPS plateau capacity measurement
./scripts/benchmark/run-benchmark.sh sustainable-throughput 3

# 4. Payment & Restock — Order creation + 50% payment success / 50% cancel restock loop (50 VUs, 60s)
./scripts/benchmark/run-benchmark.sh payment-restock 3

#    -> View the Paid (~50%) vs. Cancelled/Restocked (~50%) & Stock Conservation summary table:
./scripts/show-restock-summary.sh
```

### Extreme Performance Benchmark (MaxPerf 40k–50k RPS Mode)
Isolates host noise, applies OS kernel/sysctl tuning, and launches the dedicated high-capacity topology (`docker-compose.maxperf.yaml`):
```bash
# Run MaxPerf benchmark (e.g., Target 40,000 Peak RPS, 3 runs)
./scripts/benchmark/run-maxperf-benchmark.sh 40000 3

# View pre-recorded 40k RPS empirical report
./scripts/benchmark/view-40k-report.sh
```

---

## 💥 Chaos Engineering & Resilience Scenarios

In accordance with Decision Gate D0.12 (no destructive actions exposed via browser UI), execute operator drills and automated resilience tests from the CLI:

### 1. API Instance Crash & Recovery Drill
```bash
# Optimized API: Kill 1 replica under load (NGINX automatically fails over to surviving replica)
./scripts/chaos/kill-optimized-api.sh
./scripts/chaos/restore-optimized-api.sh

# Baseline API: Simulate single-point-of-failure outage and recovery
./scripts/chaos/kill-baseline-api.sh
./scripts/chaos/restore-baseline-api.sh
```

### 2. Redis Outage & Circuit Breaker Drill
Stops Redis, verifies that the Optimized API trips its circuit breaker and safely returns `503 Service Unavailable` (zero stock leak / zero oversell), then restarts Redis, warms up cache, and verifies recovery + SQL invariants:
```bash
./scripts/chaos/redis-restart-drill.sh
```

### 3. NATS JetStream Redelivery & Worker DB Idempotency Test
Verifies that duplicate/redelivered NATS JetStream messages are deduplicated atomically via `processed_events` (`ON CONFLICT DO NOTHING`) and that failed DB transactions roll back cleanly to allow redelivery:
```bash
# Test NATS redelivery deduplication (Invariant I5)
go test -v ./test/system -run TestChaos_NATSRedelivery_IdempotentDeduplication

# Full E2E Worker persistence, transactional rollback on failure, and malformed message handling
INTEGRATION_TEST=1 go test -v ./internal/optimized -run "TestIntegration_EndToEnd_OrderCreationAndWorkerPersistence|TestWorker_TransactionalRollback_AllowsRedelivery|TestWorker_MalformedMessage_HandledGracefully"
```

### 4. Payment Confirmation vs. Expiry Sweeper Race Test
Simulates concurrent execution where a user payment webhook/confirmation arrives at the exact millisecond the background expiry sweeper attempts to time out the order, verifying atomic state serialization (`paid` XOR `expired`):
```bash
go test -v ./test/system -run TestChaos_PaymentExpiryRace_AtomicSerialization
```

### 5. Run All Chaos & System Invariant Tests
```bash
# Execute all chaos & invariant tests (Redis 503 circuit breaker, Payment race, NATS idempotency, Stock equations)
go test -v ./test/system/...
```
