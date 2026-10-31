import http from 'k6/http';
import { getDeterministicUser, getDeterministicIdempotencyKey, SEEDED_CONSTANTS } from '../lib/generators.js';
import { classifyResponse } from '../lib/assertions.js';

// Stepped arrival rate to identify the throughput plateau.
// Peak is set with `-e PEAK_RPS=<n>` (default 2000). Stages: 10% -> 25% -> 50% -> 100% -> 25% of peak.
// Total requests sent ~= 59 * PEAK_RPS (seed stock accordingly, see run-benchmark.sh).
const PEAK = parseInt(__ENV.PEAK_RPS || '2000', 10);

export const options = {
  scenarios: {
    stepped_rps: {
      executor: 'ramping-arrival-rate',
      startRate: Math.max(Math.round(PEAK * 0.05), 1),
      timeUnit: '1s',
      preAllocatedVUs: Math.min(Math.max(Math.round(PEAK / 10), 100), 2000),
      maxVUs: Math.min(Math.max(PEAK, 2000), 8000),
      stages: [
        { duration: '30s', target: Math.round(PEAK * 0.1) },
        { duration: '30s', target: Math.round(PEAK * 0.25) },
        { duration: '30s', target: Math.round(PEAK * 0.5) },
        { duration: '30s', target: PEAK },
        { duration: '30s', target: Math.round(PEAK * 0.25) },
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
