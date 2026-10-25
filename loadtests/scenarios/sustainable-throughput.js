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
