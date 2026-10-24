-- Idempotent confirm. KEYS: reserved zset, reservation hash; ARGV: reservation id
local state = redis.call('HGET', KEYS[2], 'state')
if state == 'confirmed' then return {1, 'confirmed'} end
if state ~= 'reserved' then return {1, state or 'missing'} end
redis.call('ZREM', KEYS[1], ARGV[1])
redis.call('HSET', KEYS[2], 'state', 'confirmed')
return {0, 'confirmed'}
