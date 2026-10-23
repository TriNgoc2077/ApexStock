-- confirm_stock.lua
-- Remove reservation (stock already decremented, now permanent)
-- KEYS: [1]=reserved_key
-- ARGV: [1]=member

local reserved_key = KEYS[1]
local member       = ARGV[1]

-- 1. Check reservation exists
local score = redis.call('ZSCORE', reserved_key, member)
if score == false then
    return {-1, "RESERVATION_NOT_FOUND"}
end

-- 2. Remove reservation
redis.call('ZREM', reserved_key, member)

return {0, "CONFIRMED"}
