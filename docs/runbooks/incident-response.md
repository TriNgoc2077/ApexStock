# Operational Incident Response Runbook

## 1. Alert: OversellDetected (`oversell_total > 0`)
- **Severity:** P0 — Critical Invariant Breach.
- **Triage Steps:**
  1. Freeze the flash sale item immediately:
     `UPDATE flash_sale_events SET status = 'paused' WHERE id = '<event_id>';`
  2. Run the reconciliation verification script:
     `go test -v ./test/reconciliation -run TestStockReconciliation`
  3. Inspect `processed_events` and `orders` for duplicate reservations:
     `SELECT id, COUNT(*) FROM orders GROUP BY id HAVING COUNT(*) > 1;`
  4. Compare Redis Lua atomic counters against database records.

## 2. Alert: HighHttp5xxRate (> 1% 5xx errors)
- **Severity:** P1 — Degraded Reliability.
- **Triage Steps:**
  1. Inspect recent JSON structured logs with level `ERROR`:
     `docker compose logs --tail=100 api-optimized | grep '"status":5'`
  2. Check database connection pool exhaustion in PostgreSQL.
  3. Verify Redis connection ping and NATS cluster availability.

## 3. Alert: NATSQueueBacklogHigh (> 1,000 lag)
- **Severity:** P2 — Asynchronous Ingestion Delay.
- **Triage Steps:**
  1. Check worker container health: `docker compose ps worker`
  2. Inspect worker container logs: `docker compose logs --tail=100 worker`
  3. If worker crashed, restart the worker container:
     `docker compose restart worker`
  4. NATS JetStream will safely redeliver unacknowledged messages due to idempotent database transactions.

## 4. Disaster Recovery: Container Crash / Kill Drill
- To simulate container failure and test automated recovery during demo:
  - Baseline API Kill: `docker stop apexstock-api-baseline-1`
  - Optimized NGINX Kill: `docker stop apexstock-nginx-1`
  - Worker Kill: `docker stop apexstock-worker-1`
- Observe Prometheus `ServiceDown` alert and Grafana War Room recovery indicators.
