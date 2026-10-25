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
