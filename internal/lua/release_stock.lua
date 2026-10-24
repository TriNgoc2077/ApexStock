-- release_stock.lua
-- Atomic: remove reservation + increment stock + decrement user counter
-- KEYS: [1]=stock_key, [2]=reserved_key, [3]=user_key
-- ARGV: [1]=quantity, [2]=member

local stock_key    = KEYS[1]
local reserved_key = KEYS[2]
local user_key     = KEYS[3]

local quantity     = tonumber(ARGV[1])
local member       = ARGV[2]

-- 1. Check reservation exists (idempotency)
local score = redis.call('ZSCORE', reserved_key, member)
if score == false then
    return {-1, "RESERVATION_NOT_FOUND"}
end

-- 2. Remove from reserved set
redis.call('ZREM', reserved_key, member)

-- 3. Return stock
redis.call('INCRBY', stock_key, quantity)

-- 4. Decrement user counter
local user_count = tonumber(redis.call('GET', user_key) or "0")
if user_count > 0 then
    local new_count = user_count - quantity
    if new_count <= 0 then
        redis.call('DEL', user_key)
    else
        redis.call('DECRBY', user_key, quantity)
    end
end

-- 5. Return new stock
local new_stock = tonumber(redis.call('GET', stock_key))
return {0, "RELEASED", new_stock}
