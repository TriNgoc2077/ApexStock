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
