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

## 📊 Benchmark Scenarios (k6)

Run fair, sequential load tests adhering to Decision Gate D0.11:
```bash
# Flash-sale inventory contention shock test
./scripts/benchmark/run-benchmark.sh flash-sale-shock 3

# Maximum sustainable throughput measurement (Plateau Stepped RPS)
./scripts/benchmark/run-benchmark.sh sustainable-throughput 3
```

---

## 💥 Chaos Scenarios (Operator Drills)

In accordance with Decision Gate D0.12 (no destructive actions exposed via browser UI):
```bash
# Simulate failure of an Optimized API instance under load
./scripts/chaos/kill-optimized-api.sh

# Restore the instance and observe real-time recovery on the War Room
./scripts/chaos/restore-optimized-api.sh
```
