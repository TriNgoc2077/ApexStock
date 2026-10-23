#!/usr/bin/env bash
# Reset both databases and Redis to baseline state
# Idempotent — safe to run between benchmark runs

set -euo pipefail

: "${POSTGRES_HOST:=postgres}"
: "${POSTGRES_PORT:=5432}"
: "${POSTGRES_USER:=apexstock}"
: "${POSTGRES_PASSWORD:=apexstock}"
: "${REDIS_HOST:=redis}"
: "${REDIS_PORT:=6379}"

DBS=("apexstock_baseline" "apexstock_optimized")
ITEM_ID="22222222-2222-2222-2222-222222222222"
INITIAL_STOCK=10000

echo "==> Resetting databases and Redis to baseline"

# Reset PostgreSQL
for DB in "${DBS[@]}"; do
    echo "==> Resetting database: $DB"
    PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$DB" <<-EOSQL
        -- Delete transactional data
        TRUNCATE TABLE webhook_logs CASCADE;
        TRUNCATE TABLE payments CASCADE;
        TRUNCATE TABLE order_items CASCADE;
        TRUNCATE TABLE orders CASCADE;
        TRUNCATE TABLE idempotency_keys CASCADE;

        -- Reset flash sale item stock to initial
        UPDATE flash_sale_items
        SET sold_quantity = 0
        WHERE id = '$ITEM_ID';

        -- Keep users, products, flash_sale_events, flash_sale_items
EOSQL
    echo "    ✓ Reset $DB"
done

# Reset Redis (optimized only)
echo "==> Flushing Redis inventory/idempotency keys"
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" DEL "flash_item:$ITEM_ID:stock" "flash_item:$ITEM_ID:reserved" >/dev/null
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" EVAL \
    "local keys = redis.call('keys', 'flash_item:$ITEM_ID:user:*'); for i=1,#keys,5000 do redis.call('del', unpack(keys, i, math.min(i+4999, #keys))) end; return #keys" 0 >/dev/null
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" EVAL \
    "local keys = redis.call('keys', 'idempotency:*'); for i=1,#keys,5000 do redis.call('del', unpack(keys, i, math.min(i+4999, #keys))) end; return #keys" 0 >/dev/null
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" SET "flash_item:$ITEM_ID:stock" "$INITIAL_STOCK" >/dev/null

echo "    ✓ Redis reset and stock initialized to $INITIAL_STOCK"

echo "==> Reset complete — both databases and Redis at baseline"
