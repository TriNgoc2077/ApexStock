-- rate_limit.lua
-- Atomic fixed-window rate limiter
-- KEYS: [1]=rate_limit_key
-- ARGV: [1]=limit, [2]=window_seconds

local key     = KEYS[1]
local limit   = tonumber(ARGV[1])
local window  = tonumber(ARGV[2])

local current = redis.call('INCR', key)
if current == 1 then
    redis.call('EXPIRE', key, window)
end

if current > limit then
    local ttl = redis.call('TTL', key)
    return {-1, "RATE_LIMITED", current, ttl}
end

return {0, "OK", current, limit - current}
