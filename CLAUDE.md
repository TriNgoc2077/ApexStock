# ApexStock — Agent Context

High-concurrency flash sale benchmarking platform comparing **Baseline** (Go → PostgreSQL `SELECT ... FOR UPDATE`, `:8081`) and **Optimized** (Nginx → Go → Redis/Lua → NATS JetStream → Worker → PostgreSQL, `:8082`) on a single development laptop (i5-10500H 6C/12T, 16GB RAM, Docker Compose).

## Document Precedence Hierarchy (Higher Entries Override Lower Ones)

1. `docs/decisions/D0-decisions-summary.md` — Locked decisions (D0.1–D0.15).
2. `docs/contracts/openapi.yaml`, `docs/contracts/api-flash-sale-orders.md` — Common API contracts.
3. `docs/domain/inventory-state-model.md` — Inventory state machine & mathematical invariants.
4. `docs/benchmark/benchmark-matrix.yaml` — Resource budgets, seed IDs, benchmark scenarios.
5. `IMPLEMENTATION_PLAN.md` — Deliverables and acceptance criteria for each stage.
6. `docs/design/*.md` — **Reference lookup only** (schemas, Lua scripts, UI wireframes). These files were authored prior to D0 and contain superseded specifications: e.g., UI container-kill endpoints / `docker.sock` / `/api/v1/warroom/chaos/kill` (removed per D0.12), route `:id` (changed to `[id]` per D0.14), `allkeys-lru` (changed to `noeviction` per D0.13), 5,000 users (superseded by `seeded_users` in matrix), and preliminary RPS/latency numbers (strictly hypotheses per D0.10).

**Do NOT read `docs/design/` end-to-end** — grep or view only the specific snippets needed for the active stage.

## Execution State & Workflow

- Authoritative execution state is tracked in `IMPLEMENTATION_LEDGER.md` — read first before any action, and update immediately upon every phase transition.
- Stages execute strictly sequentially from Stage 1 → 13, guided by a single orchestrator dispatching one subagent at a time (see `.claude/agents/orchestrator.md`).
- Never alter locked domain semantics. For edge-case ambiguities that do not compromise Invariants I1–I5: issue a ruling, and record it under *Technical Rulings* in the ledger.

## Mandatory Invariants (Detailed in IMPLEMENTATION_PLAN.md §3.1)

- **I1:** `available >= 0`, `total = available + reserved + paid`; zero overselling under all conditions.
- **I2:** `reserve`, `release`, and `confirm` are idempotent; release after confirm and confirm after expired are no-ops.
- **I3:** Same `(user, idempotency_key)` + identical payload → identical response; differing payload → 409 `idempotency_conflict`.
- **I4:** Order status transitions exclusively follow: `pending_payment → paid | payment_failed | expired | cancelled`; late payments retain `expired` status and create a `webhook_logs.status='refund'` audit entry.
- **I5:** NATS at-least-once delivery requires idempotent worker processing via database unique constraints / `processed_events`.

## Verified Runtime Environment

- Local Go 1.27 (`go.mod` specifies `go 1.22`, module `apexstock`), Node 24 / npm 11, Docker 29 + Compose v5.
- **k6 is not installed on host** → run k6 using the `grafana/k6` Docker image inside Compose or via `docker run`.
- PostgreSQL: Single container hosting two isolated databases: `apexstock_baseline` and `apexstock_optimized` (D0.15), sharing identical migrations and seed IDs.
- Do not expose PostgreSQL or Redis ports to the host by default; never commit secrets (`.env` is gitignored; only commit `.env.example`).

## Standard Verification Suite

```bash
gofmt -l . && go vet ./... && go test -race ./...
docker compose -f deploy/compose/docker-compose.yaml config -q   # Standard Compose path
npm --prefix web run lint && npm --prefix web run build          # Frontend build verification
bash -n scripts/**/*.sh                                          # Shell script syntax validation
```

## Commit & Repository Hygiene

- Commit identity is populated from environment variables set by `scripts/run-sequential.sh` (`claude[bot] <209825114+claude[bot]@users.noreply.github.com>`). Do not modify Git global config.
- Two commits per stage: `wip(stage-X): implement ...` following IMPLEMENT and `feat(stage-X): ...` following VERIFY.
- Never commit `node_modules`, `.next`, build binaries, `.env`, or large raw k6 outputs (only commit summaries and metadata inside the evidence pack).
- No direct `git push`, no history rewriting (`--force`), and never use `--no-verify`.
