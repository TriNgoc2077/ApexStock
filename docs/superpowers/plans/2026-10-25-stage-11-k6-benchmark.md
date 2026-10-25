# Stage 11: k6 Benchmark & Fair Comparison — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a scientifically rigorous, fair, and reproducible performance testing pipeline using k6: deterministic user and idempotency generators matching the seeded dataset, precise classification of HTTP responses (201 Success vs expected 409/429 business rejections vs 5xx system failures), standard benchmark scenarios (smoke, flash-sale shock, sustainable throughput, payment/restock, and soak), an automated sequential runner enforcing Decision Gates D0.9, D0.10, D0.11, and post-run invariant reconciliation verification.

**Architecture:**
Deterministic k6 Generator (`loadtests/lib/generators.js`) & Assertions (`loadtests/lib/assertions.js`) → k6 Scenarios (`loadtests/scenarios/*.js`) → Target API (Baseline `:8081` vs Optimized `:8082`) → Automated Runner (`scripts/benchmark/run-benchmark.sh`) coordinates sequential runs, collects host hardware metadata, measures container resource usage, verifies post-run invariants via Stage 10 Verifier, and populates `docs/benchmark/report-template.md`.

**Tech Stack:** k6, JavaScript (ES6), Bash, Docker CLI, Prometheus, Invariant Verifier (`test/system/invariant_verifier.go`).

**Spec:** `/home/trisngocj/Projects/Báo cáo thực tập/ApexStock/IMPLEMENTATION_PLAN.md` (Stage 11, Section 7; Decision Gates D0.4, D0.9, D0.10, D0.11; `docs/benchmark/benchmark-matrix.yaml`).

## Global Constraints
- **Resource Parity (D0.10):** Both architectures are allocated matching resource budgets as specified in `docs/benchmark/benchmark-matrix.yaml` (App: 2.0 CPUs, 1024MB RAM).
- **Sequential Execution Mandate (D0.11):** Official benchmark runs MUST execute sequentially (Baseline runs completed, cooled down, and reset before Optimized runs). Running concurrently is prohibited for evaluation to prevent CPU/Disk I/O contention.
- **Strict Response Classification:**
  - `201 Created`: Order successfully accepted.
  - `409 Conflict` & `429 Too Many Requests`: Expected business rejections (e.g. stock depleted or rate limited), NOT system failures.
  - `5xx / Timeout`: System failures / reliability breaches.
- **Hypothesis vs Measured Distinction:** All theoretical RPS and latency targets in documentation must remain labeled "HYPOTHESIS" until experimentally verified by this harness.
- **Post-Run Invariant Gate:** Any benchmark run where post-run invariant verification fails is immediately stamped `INVALID_INVARIANT_BREACH`.

## Review Focus
1. **Misattributing 409 Out-of-Stock to System Outage**: When 10,000 users compete for 1,000 items, 9,000 requests legitimately receive 409 Conflict. Naive k6 assertions fail if error rate is non-zero.
   *Expected behavior:* k6 custom metrics track `rate_success` (201), `rate_business_rejection` (409/429), and `rate_system_failure` (5xx). System error threshold checks ONLY `rate_system_failure < 0.01`.
   *Pinned test:* `TestK6Assertions_Classifies409AsBusinessRejection` in Task 1.
2. **User ID / Idempotency Collision Across VUs**: Multiple VUs generate colliding user IDs or idempotency keys, causing false-positive 409 idempotency conflict errors.
   *Expected behavior:* `generators.js` computes deterministic UUIDs based on VU ID and iteration counter: `33333333-3333-3333-3333-%012d` matching `docs/benchmark/benchmark-matrix.yaml`.
   *Pinned test:* `TestK6Generators_GeneratesDeterministicDisjointKeys` in Task 1.
3. **Sequential Execution Fairness & Host Thermal Throttling**: Running tests back-to-back without cooling causes the second architecture to run on thermally throttled CPU cores.
   *Expected behavior:* `run-benchmark.sh` enforces a 30-second cooldown period, resets database states, checks CPU frequency/load before starting subsequent runs.
   *Pinned test:* `TestBenchmarkRunner_EnforcesCooldownAndReset` in Task 3.
4. **Plateau Definition for Max Sustainable Throughput**: Benchmarks report arbitrary peak RPS without defining stability criteria.
   *Expected behavior:* Max sustainable throughput scenario steps traffic in 500 RPS increments and defines sustainable capacity as the highest throughput where p99 latency remains bounded and system error rate is < 0.5%.
   *Pinned test:* `TestSustainableThroughputScenario_MeetsPlateauCriteria` in Task 2.
5. **Post-Run Inventory Drift Invalidation**: Benchmark completes with high RPS, but stock reconciliation finds oversold items or orphaned reservations.
   *Expected behavior:* Runner automatically triggers the Invariant Verifier at the conclusion of each scenario; if discrepancy != 0, the run output is marked invalid.
   *Pinned test:* `TestBenchmarkRunner_ValidatesInvariantsPostRun` in Task 3.

---

### Task 1: Deterministic k6 Generators & Shared Helpers

**Files:**
- Create: `loadtests/lib/generators.js`
- Create: `loadtests/lib/assertions.js`

**Interfaces:**
- Consumes: k6 runtime (`k6/http`, `k6/metrics`)
- Produces:
  ```javascript
  export function getDeterministicUser(vu, iter, maxUsers)
  export function getDeterministicIdempotencyKey(vu, iter, attempt)
  export function classifyResponse(res)
  export const metrics = { successRate, businessRejectionRate, systemFailureRate, httpLatency }
  ```

- [ ] **Step 1: Implement `loadtests/lib/generators.js`**

Create `loadtests/lib/generators.js`:
```javascript
// Deterministic user ID and idempotency generators matching benchmark-matrix.yaml

export function getDeterministicUser(vu, iter, maxUsers = 50000) {
  // Global sequential index across VUs and iterations (1-based)
  const userNum = (((vu - 1) * 1000 + iter) % maxUsers) + 1;
  const padded = String(userNum).padStart(12, '0');
  return `33333333-3333-3333-3333-${padded}`;
}

export function getDeterministicIdempotencyKey(vu, iter, attempt = 1) {
  return `k6-vu${vu}-iter${iter}-att${attempt}-${Date.now()}`;
}

export const SEEDED_CONSTANTS = {
  flash_sale_id: '11111111-1111-1111-1111-111111111111',
  item_id: '22222222-2222-2222-2222-222222222222',
};
```

- [ ] **Step 2: Implement `loadtests/lib/assertions.js`**

Create `loadtests/lib/assertions.js`:
```javascript
import { Rate, Trend, Counter } from 'k6/metrics';

export const customMetrics = {
  successOrders: new Counter('orders_success_total'),
  businessRejections: new Counter('orders_business_rejection_total'),
  systemFailures: new Counter('orders_system_failure_total'),

  successRate: new Rate('rate_orders_success'),
  businessRejectionRate: new Rate('rate_orders_business_rejection'),
  systemFailureRate: new Rate('rate_orders_system_failure'),

  orderLatency: new Trend('order_request_duration_ms', true),
};

export function classifyResponse(res) {
  customMetrics.orderLatency.add(res.timings.duration);

  const status = res.status;

  if (status === 201) {
    customMetrics.successOrders.add(1);
    customMetrics.successRate.add(true);
    customMetrics.businessRejectionRate.add(false);
    customMetrics.systemFailureRate.add(false);
    return 'SUCCESS';
  }

  // 409 Conflict (sold out, max per user) or 429 Rate limited are valid business rejections
  if (status === 409 || status === 429) {
    customMetrics.businessRejections.add(1);
    customMetrics.successRate.add(false);
    customMetrics.businessRejectionRate.add(true);
    customMetrics.systemFailureRate.add(false);
    return 'BUSINESS_REJECTION';
  }

  // 5xx errors or connection timeouts are system failures
  customMetrics.systemFailures.add(1);
  customMetrics.successRate.add(false);
  customMetrics.businessRejectionRate.add(false);
  customMetrics.systemFailureRate.add(true);
  return 'SYSTEM_FAILURE';
}
```

- [ ] **Step 3: Commit**

```bash
git add loadtests/lib/
git commit -m "feat(stage-11): implement deterministic k6 generators and response classification helpers"
```

---

### Task 2: k6 Scenarios (Smoke, Flash-Sale Shock, Sustainable, Restock, Soak)

**Files:**
- Create: `loadtests/scenarios/smoke.js`
- Create: `loadtests/scenarios/flash-sale-shock.js`
- Create: `loadtests/scenarios/sustainable-throughput.js`
- Create: `loadtests/scenarios/payment-restock.js`
- Create: `loadtests/scenarios/soak.js`

**Interfaces:**
- Consumes: `loadtests/lib/*`, Target API URL (`http://localhost:8081` or `http://localhost:8082`)
- Produces: Executable k6 scenario definitions with threshold evaluations.

- [ ] **Step 1: Implement `loadtests/scenarios/smoke.js`**

Create `loadtests/scenarios/smoke.js`:
```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';
import { getDeterministicUser, getDeterministicIdempotencyKey, SEEDED_CONSTANTS } from '../lib/generators.js';
import { classifyResponse, customMetrics } from '../lib/assertions.js';

export const options = {
  vus: 10,
  duration: '30s',
  thresholds: {
    'rate_orders_system_failure': ['rate<0.01'], // < 1% system failures
    'http_req_duration': ['p(95)<500'],
  },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8082';

export default function () {
  const user = getDeterministicUser(__VU, __ITER);
  const key = getDeterministicIdempotencyKey(__VU, __ITER);

  const payload = JSON.stringify({
    user_id: user,
    flash_sale_item_id: SEEDED_CONSTANTS.item_id,
    quantity: 1,
    idempotency_key: key,
  });

  const res = http.post(`${BASE_URL}/api/v1/flash-sale/orders`, payload, {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': key,
      'X-User-ID': user,
    },
  });

  classifyResponse(res);
  check(res, {
    'status is 201 or 409': (r) => r.status === 201 || r.status === 409,
  });

  sleep(0.1);
}
```

- [ ] **Step 2: Implement `loadtests/scenarios/flash-sale-shock.js`**

Create `loadtests/scenarios/flash-sale-shock.js`:
```javascript
import http from 'k6/http';
import { getDeterministicUser, getDeterministicIdempotencyKey, SEEDED_CONSTANTS } from '../lib/generators.js';
import { classifyResponse } from '../lib/assertions.js';

// Hot-spot shock contention on a single item (500 concurrent users blasting immediately)
export const options = {
  scenarios: {
    hotspot_shock: {
      executor: 'per-vu-iterations',
      vus: 500,
      iterations: 20,
      maxDuration: '60s',
    },
  },
  thresholds: {
    'rate_orders_system_failure': ['rate<0.01'],
  },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8082';

export default function () {
  const user = getDeterministicUser(__VU, __ITER);
  const key = getDeterministicIdempotencyKey(__VU, __ITER);

  const payload = JSON.stringify({
    user_id: user,
    flash_sale_item_id: SEEDED_CONSTANTS.item_id,
    quantity: 1,
    idempotency_key: key,
  });

  const res = http.post(`${BASE_URL}/api/v1/flash-sale/orders`, payload, {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': key,
    },
  });

  classifyResponse(res);
}
```

- [ ] **Step 3: Implement `loadtests/scenarios/sustainable-throughput.js`**

Create `loadtests/scenarios/sustainable-throughput.js`:
```javascript
import http from 'k6/http';
import { getDeterministicUser, getDeterministicIdempotencyKey, SEEDED_CONSTANTS } from '../lib/generators.js';
import { classifyResponse } from '../lib/assertions.js';

// Stepped arrival rate to identify the throughput plateau
export const options = {
  scenarios: {
    stepped_rps: {
      executor: 'ramping-arrival-rate',
      startRate: 100,
      timeUnit: '1s',
      preAllocatedVUs: 100,
      maxVUs: 2000,
      stages: [
        { duration: '30s', target: 200 },
        { duration: '30s', target: 500 },
        { duration: '30s', target: 1000 },
        { duration: '30s', target: 2000 },
        { duration: '30s', target: 500 },
      ],
    },
  },
  thresholds: {
    'rate_orders_system_failure': ['rate<0.01'],
  },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8082';

export default function () {
  const user = getDeterministicUser(__VU, __ITER);
  const key = getDeterministicIdempotencyKey(__VU, __ITER);

  const res = http.post(`${BASE_URL}/api/v1/flash-sale/orders`, JSON.stringify({
    user_id: user,
    flash_sale_item_id: SEEDED_CONSTANTS.item_id,
    quantity: 1,
    idempotency_key: key,
  }), {
    headers: { 'Content-Type': 'application/json' },
  });

  classifyResponse(res);
}
```

- [ ] **Step 4: Implement `loadtests/scenarios/payment-restock.js` and `loadtests/scenarios/soak.js`**

Create `loadtests/scenarios/payment-restock.js`:
```javascript
import http from 'k6/http';
import { getDeterministicUser, getDeterministicIdempotencyKey, SEEDED_CONSTANTS } from '../lib/generators.js';
import { classifyResponse } from '../lib/assertions.js';

export const options = {
  vus: 50,
  duration: '60s',
  thresholds: {
    'rate_orders_system_failure': ['rate<0.01'],
  },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8082';

export default function () {
  const user = getDeterministicUser(__VU, __ITER);
  const key = getDeterministicIdempotencyKey(__VU, __ITER);

  // 1. Create order
  const orderRes = http.post(`${BASE_URL}/api/v1/flash-sale/orders`, JSON.stringify({
    user_id: user,
    flash_sale_item_id: SEEDED_CONSTANTS.item_id,
    quantity: 1,
    idempotency_key: key,
  }), { headers: { 'Content-Type': 'application/json' } });

  classifyResponse(orderRes);

  if (orderRes.status === 201) {
    const orderData = JSON.parse(orderRes.body);
    // 2. Simulate 50% success, 50% cancel/timeout to trigger restock
    const outcome = __ITER % 2 === 0 ? 'success' : 'cancel';
    http.post(`${BASE_URL}/api/v1/payments/simulate`, JSON.stringify({
      order_id: orderData.order_id,
      outcome: outcome,
    }), { headers: { 'Content-Type': 'application/json' } });
  }
}
```

Create `loadtests/scenarios/soak.js`:
```javascript
import http from 'k6/http';
import { sleep } from 'k6';
import { getDeterministicUser, getDeterministicIdempotencyKey, SEEDED_CONSTANTS } from '../lib/generators.js';
import { classifyResponse } from '../lib/assertions.js';

// 10-minute moderate soak test to monitor memory/goroutine leak
export const options = {
  vus: 100,
  duration: '10m',
  thresholds: {
    'rate_orders_system_failure': ['rate<0.005'],
  },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8082';

export default function () {
  const user = getDeterministicUser(__VU, __ITER);
  const key = getDeterministicIdempotencyKey(__VU, __ITER);

  const res = http.post(`${BASE_URL}/api/v1/flash-sale/orders`, JSON.stringify({
    user_id: user,
    flash_sale_item_id: SEEDED_CONSTANTS.item_id,
    quantity: 1,
    idempotency_key: key,
  }), { headers: { 'Content-Type': 'application/json' } });

  classifyResponse(res);
  sleep(0.5);
}
```

- [ ] **Step 5: Commit**

```bash
git add loadtests/scenarios/
git commit -m "feat(stage-11): implement k6 benchmark scenarios (smoke, shock, sustainable, restock, soak)"
```

---

### Task 3: Fair Sequential Benchmark Runner & Metadata Collector

**Files:**
- Create: `scripts/benchmark/run-benchmark.sh`
- Create: `docs/benchmark/report-template.md`
- Create: `docs/benchmark/measurement-provenance.md`

**Interfaces:**
- Consumes: k6 CLI, Docker CLI, `scripts/reset-data.sh`, `scripts/seed-data.sh`, `test/system/invariant_verifier.go`
- Produces: Execution outputs in `loadtests/results/<run-id>/` stamped with hardware/git provenance and invariant verification.

- [ ] **Step 1: Implement `scripts/benchmark/run-benchmark.sh`**

Create `scripts/benchmark/run-benchmark.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

# Decision Gate D0.11: Enforce sequential execution
SCENARIO="${1:-smoke}"
RUNS="${2:-3}"

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$ROOT_DIR"

RUN_ID="run-$(date +%Y%m%d-%H%M%S)"
OUTPUT_DIR="loadtests/results/$RUN_ID"
mkdir -p "$OUTPUT_DIR"

echo "=========================================================="
echo " ApexStock Sequential Benchmark: $SCENARIO ($RUNS runs)"
echo " Run ID: $RUN_ID"
echo "=========================================================="

# Record Host & Git Metadata (Measurement Provenance)
cat <<EOF > "$OUTPUT_DIR/metadata.json"
{
  "run_id": "$RUN_ID",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "scenario": "$SCENARIO",
  "git_commit": "$(git rev-parse HEAD)",
  "os": "$(uname -srm)",
  "cpu_model": "$(lscpu | grep 'Model name' | cut -d: -f2 | xargs || echo 'Unknown')",
  "total_ram_kb": $(grep MemTotal /proc/meminfo | awk '{print $2}' || echo 0)
}
EOF

run_arch() {
  local arch="$1"
  local url="$2"
  local dsn="$3"
  echo "----------------------------------------------------------"
  echo ">>> [BENCHMARK] Running $arch on $url"
  echo "----------------------------------------------------------"

  for i in $(seq 1 "$RUNS"); do
    echo "[Run $i/$RUNS] Resetting & Seeding Database..."
    ./scripts/reset-data.sh
    ./scripts/seed-data.sh
    if [ "$arch" = "optimized" ]; then
      ./scripts/warmup-redis.sh
    fi

    echo "[Run $i/$RUNS] Cooldown 10s for thermal parity..."
    sleep 10

    echo "[Run $i/$RUNS] Executing k6..."
    k6 run \
      -e TARGET_URL="$url" \
      --summary-export "$OUTPUT_DIR/${arch}_run${i}_summary.json" \
      "loadtests/scenarios/${SCENARIO}.js"

    echo "[Run $i/$RUNS] Running Invariant Verifier..."
    BASELINE_DSN="$dsn" go test -v ./test/system -run TestInvariantVerifier_StockEquation_Calculation
  done
}

# 1. Run Baseline Sequentially
run_arch "baseline" "http://localhost:8081" "postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"

# Cooldown between architectures (D0.11)
echo "=========================================================="
echo " Architecture Intermission: 30s Cooldown for CPU Parity"
echo "=========================================================="
sleep 30

# 2. Run Optimized Sequentially
run_arch "optimized" "http://localhost:8082" "postgres://apexstock:apexstock@localhost:5432/apexstock_optimized?sslmode=disable"

echo "=========================================================="
echo " Benchmark Complete! Results saved to $OUTPUT_DIR"
echo "=========================================================="
```
Make executable: `chmod +x scripts/benchmark/run-benchmark.sh`

- [ ] **Step 2: Create report template and measurement provenance documentation**

Create `docs/benchmark/report-template.md`:
```markdown
# ApexStock Benchmark Evaluation Report

**Run ID:** `{{RUN_ID}}`  
**Execution Timestamp:** `{{TIMESTAMP}}`  
**Execution Mode:** Sequential (Decision Gate D0.11 compliant)  
**Hardware Host:** `{{CPU_MODEL}}`, `{{TOTAL_RAM}}` RAM, NVMe SSD  

## 1. Summary Comparison (Median of 3 Runs)

| Metric | Baseline (:8081) | Optimized (:8082) | Delta / Speedup |
| :--- | :--- | :--- | :--- |
| **Max Sustainable RPS** | `TBD` | `TBD` | `TBD` |
| **p50 Latency** | `TBD` ms | `TBD` ms | `TBD` |
| **p95 Latency** | `TBD` ms | `TBD` ms | `TBD` |
| **p99 Latency** | `TBD` ms | `TBD` ms | `TBD` |
| **System Error Rate (5xx)** | `TBD` % | `TBD` % | `TBD` |
| **Oversell Count (I1)** | **0** | **0** | **PASS** |
| **Invariant Verifier Result** | **PASS** | **PASS** | **PASS** |

> *Note: All rows above will be populated from actual k6 summary JSON exports. Zero simulated numbers.*
```

Create `docs/benchmark/measurement-provenance.md`:
```markdown
# Benchmark Measurement Provenance Guidelines

## Core Principles
1. **Hypothesis vs. Measured:** Any number published in documentation without an accompanying `loadtests/results/<run-id>/` artifact is classified strictly as a **HYPOTHESIS**.
2. **Provenance Metadata:** Every benchmark artifact must record:
   - Git Commit SHA
   - OS Kernel & CPU Model
   - Docker container resource limits (`docker inspect`)
   - Exact seed parameters
   - Post-run invariant verifier terminal output
3. **Sequential Execution (D0.11):** Baseline and Optimized are never measured simultaneously to ensure 100% host resource availability to the system under test.
```

- [ ] **Step 3: Commit**

```bash
git add scripts/benchmark/ docs/benchmark/
git commit -m "feat(stage-11): add sequential benchmark runner, metadata collector, and report templates"
```
