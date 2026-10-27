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

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE_FILE="$ROOT_DIR/deploy/compose/docker-compose.yaml"

run_psql() {
    local db="$1"
    shift
    if command -v psql >/dev/null 2>&1 && getent hosts "$POSTGRES_HOST" >/dev/null 2>&1; then
        PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$db" "$@"
    elif command -v psql >/dev/null 2>&1 && [ "$POSTGRES_HOST" = "postgres" ] && (nc -z localhost 5432 2>/dev/null || (exec 3<>/dev/tcp/localhost/5432) 2>/dev/null); then
        PGPASSWORD="$POSTGRES_PASSWORD" psql -h localhost -p 5432 -U "$POSTGRES_USER" -d "$db" "$@"
    elif command -v docker >/dev/null 2>&1 && docker compose -f "$COMPOSE_FILE" ps postgres -q 2>/dev/null | grep -q .; then
        docker compose -f "$COMPOSE_FILE" exec -T -e PGPASSWORD="$POSTGRES_PASSWORD" postgres psql -U "$POSTGRES_USER" -d "$db" "$@"
    else
        PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$db" "$@"
    fi
}

run_redis() {
    if command -v redis-cli >/dev/null 2>&1 && getent hosts "$REDIS_HOST" >/dev/null 2>&1; then
        redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" "$@"
    elif command -v redis-cli >/dev/null 2>&1 && [ "$REDIS_HOST" = "redis" ] && (nc -z localhost 6379 2>/dev/null || (exec 3<>/dev/tcp/localhost/6379) 2>/dev/null); then
        redis-cli -h localhost -p 6379 "$@"
    elif command -v docker >/dev/null 2>&1 && docker compose -f "$COMPOSE_FILE" ps redis -q 2>/dev/null | grep -q .; then
        docker compose -f "$COMPOSE_FILE" exec -T redis redis-cli "$@"
    else
        redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" "$@"
    fi
}

DBS=("apexstock_baseline" "apexstock_optimized")
ITEM_ID="22222222-2222-2222-2222-222222222222"
INITIAL_STOCK=10000

echo "==> Resetting databases and Redis to baseline"

# Reset PostgreSQL
for DB in "${DBS[@]}"; do
    echo "==> Resetting database: $DB"
    run_psql "$DB" <<-EOSQL
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
run_redis DEL "flash_item:$ITEM_ID:stock" "flash_item:$ITEM_ID:reserved" >/dev/null
run_redis EVAL \
    "local keys = redis.call('keys', 'flash_item:$ITEM_ID:user:*'); for i=1,#keys,5000 do redis.call('del', unpack(keys, i, math.min(i+4999, #keys))) end; return #keys" 0 >/dev/null
run_redis EVAL \
    "local keys = redis.call('keys', 'idempotency:*'); for i=1,#keys,5000 do redis.call('del', unpack(keys, i, math.min(i+4999, #keys))) end; return #keys" 0 >/dev/null
run_redis SET "flash_item:$ITEM_ID:stock" "$INITIAL_STOCK" >/dev/null

echo "    ✓ Redis reset and stock initialized to $INITIAL_STOCK"

echo "==> Reset complete — both databases and Redis at baseline"
