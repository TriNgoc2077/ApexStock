-- Atomic reservation and idempotency claim.
-- KEYS: stock, reserved zset, per-user count, idempotency hash, reservation hash
-- ARGV: qty, max_per_user, reservation_id, order_id, item_id, user_id, expiry epoch, fingerprint
local stock_key = KEYS[1]
local reserved_key = KEYS[2]
local user_key = KEYS[3]
local idempotency_key = KEYS[4]
local reservation_key = KEYS[5]
local qty = tonumber(ARGV[1])
local max_per_user = tonumber(ARGV[2])
local reservation_id = ARGV[3]
local order_id = ARGV[4]
local item_id = ARGV[5]
local user_id = ARGV[6]
local expires_at = tonumber(ARGV[7])
local fingerprint = ARGV[8]
local existing = redis.call('HGETALL', idempotency_key)
if #existing > 0 then
  local found = {}
  for i = 1, #existing, 2 do found[existing[i]] = existing[i + 1] end
  if found['fingerprint'] ~= fingerprint then return {-4, 'IDEMPOTENCY_CONFLICT'} end
  return {1, found['reservation_id'], found['order_id'], found['item_id'], found['user_id'], found['qty'], found['expires_at'], found['state']}
end
local stock = tonumber(redis.call('GET', stock_key) or '-1')
if stock < 0 then return {-1, 'ITEM_NOT_FOUND'} end
if stock < qty then return {-2, 'OUT_OF_STOCK', stock} end
local bought = tonumber(redis.call('GET', user_key) or '0')
if bought + qty > max_per_user then return {-3, 'MAX_PER_USER_REACHED', bought, max_per_user} end
redis.call('DECRBY', stock_key, qty)
redis.call('INCRBY', user_key, qty)
redis.call('ZADD', reserved_key, expires_at, reservation_id)
redis.call('HSET', reservation_key, 'reservation_id', reservation_id, 'order_id', order_id, 'item_id', item_id, 'user_id', user_id, 'qty', qty, 'state', 'reserved', 'expires_at', expires_at)
redis.call('EXPIRE', reservation_key, math.max(1, expires_at - tonumber(redis.call('TIME')[1]) + 86400))
redis.call('HSET', idempotency_key, 'fingerprint', fingerprint, 'reservation_id', reservation_id, 'order_id', order_id, 'item_id', item_id, 'user_id', user_id, 'qty', qty, 'expires_at', expires_at, 'state', 'reserved')
redis.call('EXPIRE', idempotency_key, math.max(1, expires_at - tonumber(redis.call('TIME')[1]) + 86400))
return {0, reservation_id, order_id, item_id, user_id, qty, expires_at, 'reserved'}
