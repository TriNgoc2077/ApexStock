-- reserve_stock.lua
-- Atomic: check stock + check user limit + decrement + reserve
-- KEYS: [1]=stock_key, [2]=reserved_key, [3]=user_key
-- ARGV: [1]=quantity, [2]=max_per_user, [3]=member, [4]=expiry_ts

local stock_key      = KEYS[1]
local reserved_key   = KEYS[2]
local user_key       = KEYS[3]

local quantity       = tonumber(ARGV[1])
local max_per_user   = tonumber(ARGV[2])
local member         = ARGV[3]
local expiry_ts      = tonumber(ARGV[4])

-- 1. Check stock
local current_stock = tonumber(redis.call('GET', stock_key))
if current_stock == nil then
    return {-1, "ITEM_NOT_FOUND"}
end

if current_stock < quantity then
    return {-2, "OUT_OF_STOCK", current_stock}
end

-- 2. Check user limit
local user_purchased = tonumber(redis.call('GET', user_key) or "0")
if user_purchased + quantity > max_per_user then
    return {-3, "MAX_PER_USER_EXCEEDED", user_purchased, max_per_user}
end

-- 3. Atomic decrement stock
redis.call('DECRBY', stock_key, quantity)

-- 4. Add reservation
redis.call('ZADD', reserved_key, expiry_ts, member)

-- 5. Increment user counter
redis.call('INCRBY', user_key, quantity)

-- 6. Return success
local remaining = current_stock - quantity
return {0, "SUCCESS", remaining}
