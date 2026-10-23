#!/usr/bin/env bash
# Apply the same migration to both isolated databases (D0.15).
set -euo pipefail

: "${POSTGRES_HOST:=postgres}"
: "${POSTGRES_PORT:=5432}"
: "${POSTGRES_USER:=apexstock}"
: "${POSTGRES_PASSWORD:=apexstock}"
ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
MIGRATION="$ROOT_DIR/db/migrations/001_initial_schema.up.sql"

for db in apexstock_baseline apexstock_optimized; do
  echo "==> Migrating $db"
  PGPASSWORD="$POSTGRES_PASSWORD" psql \
    -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$db" \
    -v ON_ERROR_STOP=1 -f "$MIGRATION"
done

echo "==> Migration complete"
