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

PGPASSWORD="$POSTGRES_PASSWORD" psql -At -F '|' \
  -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d apexstock_optimized <<'SQL' |
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
  redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" SET "flash_item:$item:stock" "$stock" >/dev/null
  redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" DEL "flash_item:$item:reserved" >/dev/null
  echo "SET flash_item:$item:stock=$stock (reserved=$reserved)"
done

# Rebuild reservation members and per-user counters in one pass.
PGPASSWORD="$POSTGRES_PASSWORD" psql -At -F '|' \
  -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d apexstock_optimized <<'SQL' |
SELECT oi.flash_sale_item_id, EXTRACT(EPOCH FROM o.expires_at)::bigint,
       o.id::text || ':' || o.user_id::text || ':' || oi.quantity::text,
       o.user_id, oi.quantity
FROM orders o JOIN order_items oi ON oi.order_id = o.id
WHERE o.status = 'pending_payment' AND o.expires_at > now();
SQL
while IFS='|' read -r item expiry member user_id qty; do
  [ -z "$item" ] && continue
  redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ZADD "flash_item:$item:reserved" "$expiry" "$member" >/dev/null
  redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" INCRBY "flash_item:$item:user:$user_id" "$qty" >/dev/null
done

echo "Redis warm-up complete"
