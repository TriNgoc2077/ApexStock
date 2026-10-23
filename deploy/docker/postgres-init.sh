#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE apexstock_baseline;
    CREATE DATABASE apexstock_optimized;
EOSQL

echo "PostgreSQL: Created databases apexstock_baseline and apexstock_optimized"
