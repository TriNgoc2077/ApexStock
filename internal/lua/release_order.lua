-- Idempotent release. KEYS: stock, reserved zset, user count, reservation hash
-- ARGV: reservation id
local state = redis.call('HGET', KEYS[4], 'state')
if state == false or state == 'released' or state == 'confirmed' then return {1, state or 'missing'} end
local qty = tonumber(redis.call('HGET', KEYS[4], 'qty') or '0')
redis.call('INCRBY', KEYS[1], qty)
redis.call('DECRBY', KEYS[3], qty)
redis.call('ZREM', KEYS[2], ARGV[1])
redis.call('HSET', KEYS[4], 'state', 'released')
return {0, 'released', qty}
