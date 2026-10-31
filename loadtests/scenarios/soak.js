import http from 'k6/http';
import { sleep } from 'k6';
import { getDeterministicUser, getDeterministicIdempotencyKey, SEEDED_CONSTANTS } from '../lib/generators.js';
import { classifyResponse } from '../lib/assertions.js';

// 10-minute moderate soak test to monitor memory/goroutine leak
export const options = {
  vus: 100,
  duration: __ENV.SOAK_DURATION || '2m',
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
