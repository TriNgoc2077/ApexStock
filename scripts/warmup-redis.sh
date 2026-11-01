#!/usr/bin/env bash
# Rebuild optimized Redis inventory snapshot from PostgreSQL.
# Stock = total - sold - all active pending reservations.
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

run_psql apexstock_optimized -At -F '|' <<'SQL' |
WITH active_items AS (
  SELECT fsi.id, fsi.total_quantity, fsi.sold_quantity
  FROM flash_sale_items fsi
  JOIN flash_sale_events fse ON fse.id = fsi.event_id
  WHERE fse.status = 'active'
), reservations AS (
  SELECT oi.flash_sale_item_id, SUM(oi.quantity)::int AS quantity
  FROM orders o JOIN order_items oi ON oi.order_id = o.id
  WHERE o.status = 'pending_payment' AND o.expires_at > now()
  GROUP BY oi.flash_sale_item_id
)
SELECT ai.id, ai.total_quantity, ai.sold_quantity, COALESCE(r.quantity, 0)
FROM active_items ai LEFT JOIN reservations r ON r.flash_sale_item_id = ai.id
ORDER BY ai.id;
SQL
while IFS='|' read -r item total sold reserved; do
  [ -z "$item" ] && continue
  stock=$((total - sold - reserved))
  if [ "$stock" -lt 0 ]; then
    echo "ERROR: negative reconstructed stock for item $item" >&2
    exit 1
  fi
  run_redis SET "flash_item:$item:stock" "$stock" </dev/null >/dev/null
  run_redis DEL "flash_item:$item:reserved" </dev/null >/dev/null
  echo "SET flash_item:$item:stock=$stock (reserved=$reserved)"
done

# Rebuild reservation members and per-user counters in one pass.
run_psql apexstock_optimized -At -F '|' <<'SQL' |
SELECT oi.flash_sale_item_id, EXTRACT(EPOCH FROM o.expires_at)::bigint,
       o.id::text || ':' || o.user_id::text || ':' || oi.quantity::text,
       o.user_id, oi.quantity
FROM orders o JOIN order_items oi ON oi.order_id = o.id
WHERE o.status = 'pending_payment' AND o.expires_at > now();
SQL
while IFS='|' read -r item expiry member user_id qty; do
  [ -z "$item" ] && continue
  run_redis ZADD "flash_item:$item:reserved" "$expiry" "$member" </dev/null >/dev/null
  run_redis INCRBY "flash_item:$item:user:$user_id" "$qty" </dev/null >/dev/null
done

echo "Redis warm-up complete"
