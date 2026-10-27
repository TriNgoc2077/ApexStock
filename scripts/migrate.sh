#!/usr/bin/env bash
# Apply the same migration to both isolated databases (D0.15).
set -euo pipefail

: "${POSTGRES_HOST:=postgres}"
: "${POSTGRES_PORT:=5432}"
: "${POSTGRES_USER:=apexstock}"
: "${POSTGRES_PASSWORD:=apexstock}"
ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE_FILE="$ROOT_DIR/deploy/compose/docker-compose.yaml"

run_migration() {
    local db="$1"
    local sql_file="$2"
    if command -v psql >/dev/null 2>&1 && getent hosts "$POSTGRES_HOST" >/dev/null 2>&1; then
        PGPASSWORD="$POSTGRES_PASSWORD" psql \
          -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$db" \
          -v ON_ERROR_STOP=1 -f "$sql_file"
    elif command -v psql >/dev/null 2>&1 && [ "$POSTGRES_HOST" = "postgres" ] && (nc -z localhost 5432 2>/dev/null || (exec 3<>/dev/tcp/localhost/5432) 2>/dev/null); then
        PGPASSWORD="$POSTGRES_PASSWORD" psql \
          -h localhost -p 5432 -U "$POSTGRES_USER" -d "$db" \
          -v ON_ERROR_STOP=1 -f "$sql_file"
    elif command -v docker >/dev/null 2>&1 && docker compose -f "$COMPOSE_FILE" ps postgres -q 2>/dev/null | grep -q .; then
        docker compose -f "$COMPOSE_FILE" exec -T -e PGPASSWORD="$POSTGRES_PASSWORD" postgres psql -U "$POSTGRES_USER" -d "$db" -v ON_ERROR_STOP=1 < "$sql_file"
    else
        PGPASSWORD="$POSTGRES_PASSWORD" psql \
          -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$db" \
          -v ON_ERROR_STOP=1 -f "$sql_file"
    fi
}

for db in apexstock_baseline apexstock_optimized; do
  echo "==> Migrating $db"
  for f in $(ls "$ROOT_DIR"/db/migrations/*.up.sql | sort); do
    echo "    Applying $(basename "$f")"
    run_migration "$db" "$f"
  done
done

echo "==> Migration complete"
