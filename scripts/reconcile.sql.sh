#!/usr/bin/env bash
# SQL inventory reconciliation checker. Exits non-zero on mismatch.
set -euo pipefail

: "${POSTGRES_HOST:=postgres}"
: "${POSTGRES_PORT:=5432}"
: "${POSTGRES_USER:=apexstock}"
: "${POSTGRES_PASSWORD:=apexstock}"

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

failed=0
for db in apexstock_baseline apexstock_optimized; do
  echo "==> Reconciling $db"
  result=$(run_psql "$db" -At <<'SQL'
WITH pending AS (
  SELECT oi.flash_sale_item_id, COALESCE(SUM(oi.quantity), 0)::int AS reserved
  FROM orders o JOIN order_items oi ON oi.order_id = o.id
  WHERE o.status = 'pending_payment' AND o.expires_at > now()
  GROUP BY oi.flash_sale_item_id
), paid AS (
  SELECT oi.flash_sale_item_id, COALESCE(SUM(oi.quantity), 0)::int AS paid
  FROM orders o JOIN order_items oi ON oi.order_id = o.id
  WHERE o.status = 'paid'
  GROUP BY oi.flash_sale_item_id
)
SELECT fsi.id,
       fsi.total_quantity,
       fsi.sold_quantity,
       COALESCE(pending.reserved, 0),
       COALESCE(paid.paid, 0),
       fsi.total_quantity - fsi.sold_quantity - COALESCE(pending.reserved, 0)
FROM flash_sale_items fsi
LEFT JOIN pending ON pending.flash_sale_item_id = fsi.id
LEFT JOIN paid ON paid.flash_sale_item_id = fsi.id
WHERE fsi.sold_quantity < 0
   OR fsi.sold_quantity > fsi.total_quantity
   OR fsi.total_quantity - fsi.sold_quantity - COALESCE(pending.reserved, 0) < 0;
SQL
  )
  if [ -n "$result" ]; then
    echo "FAIL: invariant violation in $db"
    echo "$result"
    failed=1
  else
    echo "PASS: $db"
  fi
done

if [ "$failed" -ne 0 ]; then exit 1; fi
echo "Reconciliation PASS"
