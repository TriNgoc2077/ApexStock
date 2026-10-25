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
