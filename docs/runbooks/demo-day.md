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
