# ApexStock

Flash sale high-concurrency comparison: **Baseline** (Go → PostgreSQL `SELECT ... FOR UPDATE`) vs **Optimized** (Nginx → Go → Redis/Lua → NATS JetStream → Worker → PostgreSQL).

**Host:** Laptop i5-10500H 6C/12T, 16GB RAM, Docker Compose.

---

## Quick Start

```bash
# Build Go binaries
make build

# Run tests
make test

# Start dev environment
docker compose -f deploy/compose/docker-compose.yaml --profile dev up -d

# Check health
curl http://localhost:8081/healthz  # Baseline API
curl http://localhost:8082/healthz  # Optimized API (via Nginx)
```

---

## Profiles

- **dev**: Minimal services for development
- **demo**: Full UI + War Room + both architectures
- **benchmark**: Locked resources, sequential testing
- **observability**: Prometheus + Grafana

```bash
# Start demo profile
docker compose -f deploy/compose/docker-compose.yaml --profile demo up -d

# Access UI
open http://localhost:3000       # Next.js frontend
open http://localhost:3001       # Grafana (admin/admin)
open http://localhost:9090       # Prometheus
```

---

## Architecture

**Baseline (:8081)**
- Direct PostgreSQL with `SELECT ... FOR UPDATE`

**Optimized (:8082)**
- Nginx load balancer (2 replicas)
- Redis inventory cache (`noeviction` policy, D0.13)
- NATS JetStream (persistent)
- Async worker for payment confirmation

**Databases** (D0.15)
- `apexstock_baseline` — Baseline data
- `apexstock_optimized` — Optimized data
- Same migration + seed IDs, isolated data

---

## API Contract (D0.2)

```
POST /api/v1/flash-sale/orders
Content-Type: application/json

{
  "flash_sale_id": "uuid",
  "item_id": "uuid",
  "user_id": "uuid",
  "quantity": 1,
  "idempotency_key": "client-generated-uuid"
}

→ 201 Created (success)
→ 409 Conflict (stock exhausted, idempotency conflict)
→ 422 Unprocessable Entity (validation error)
```

---

## Project Structure

```
.
├── cmd/
│   ├── api-baseline/       # Baseline API (:8081)
│   ├── api-optimized/      # Optimized API (:8082)
│   ├── worker/             # NATS consumer + payment confirm
│   └── warroom-controller/ # War Room WebSocket + chaos control
├── internal/
│   ├── config/             # Environment config
│   └── httpapi/            # Health endpoints
├── web/                    # Next.js frontend
├── deploy/
│   ├── compose/            # Docker Compose
│   ├── docker/             # Dockerfiles
│   ├── nginx/              # Nginx config
│   ├── prometheus/         # Prometheus config
│   └── grafana/            # Grafana provisioning
├── docs/
│   ├── decisions/          # D0.1–D0.15 locked decisions
│   ├── contracts/          # API contracts
│   ├── benchmark/          # Resource matrix & scenarios
│   └── domain/             # Inventory state model
├── scripts/                # Seed, reset, benchmark scripts
└── loadtests/              # k6 scenarios
```

---

## Decisions Gate D0

All 15 decisions locked — see `docs/decisions/D0-decisions-summary.md`:
- **D0.2**: Common endpoint, separate ports (`:8081`, `:8082`)
- **D0.4**: Single-item orders only
- **D0.13**: Redis `noeviction` policy
- **D0.15**: Two databases in one PostgreSQL container

---

## Development

```bash
# Format & vet
gofmt -l . && go vet ./...

# Test with race detector
go test -race ./...

# Build Docker images
docker compose -f deploy/compose/docker-compose.yaml build

# Run migrations & seed data
make db-migrate
make db-seed

# Warm up Redis cache
make db-warmup

# Reset all data
make db-reset

# View logs
docker compose -f deploy/compose/docker-compose.yaml logs -f api-baseline
```

**Note:** Database scripts require PostgreSQL container running. Use `POSTGRES_HOST=localhost POSTGRES_PORT=5432` to run from host when port is exposed, or run inside Compose network.

---

## Stage 2 Status

✅ PostgreSQL schema migrations (shared across 2 DBs per D0.15)  
✅ Redis Lua scripts (reserve/release/confirm/rate-limit)  
✅ Deterministic seed data (50K users, flash-sale events, 10K stock)  
✅ Reset/warmup/reconciliation scripts  
✅ Idempotency keys and inventory state model (I1–I5)

---

**Next Stage:** Stage 3 — Baseline API implementation
