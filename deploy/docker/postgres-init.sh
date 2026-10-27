#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE apexstock_baseline;
    CREATE DATABASE apexstock_optimized;
EOSQL

echo "PostgreSQL: Created databases apexstock_baseline and apexstock_optimized"

if [ -d /docker-entrypoint-initdb.d/migrations ]; then
    for db in apexstock_baseline apexstock_optimized; do
        echo "PostgreSQL: Applying migrations to $db..."
        for f in $(ls /docker-entrypoint-initdb.d/migrations/*.up.sql 2>/dev/null | sort); do
            echo "  Applying $(basename "$f")"
            psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$db" -f "$f"
        done
    done
fi
