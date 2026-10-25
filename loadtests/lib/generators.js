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
