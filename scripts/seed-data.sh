#!/usr/bin/env bash
# Seed deterministic data for ApexStock flash sale
# Uses IDs from benchmark-matrix.yaml
# Must be idempotent — safe to run multiple times

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

# Database names from D0.15
DBS=("apexstock_baseline" "apexstock_optimized")

# IDs from benchmark-matrix.yaml
FLASH_SALE_ID="11111111-1111-1111-1111-111111111111"
ITEM_ID="22222222-2222-2222-2222-222222222222"
PRODUCT_ID="33333333-3333-3333-3333-333333333333"
INITIAL_STOCK=10000
MAX_PER_USER=5
SEEDED_USERS=50000
USER_ID_PATTERN="33333333-3333-3333-3333-%012d"

# Event timing: start 1 hour ago, end 2 hours from now (always active)
START_TIME=$(date -u -d '1 hour ago' '+%Y-%m-%dT%H:%M:%SZ')
END_TIME=$(date -u -d '2 hours' '+%Y-%m-%dT%H:%M:%SZ')

echo "==> Seeding data to both databases: ${DBS[*]}"

for DB in "${DBS[@]}"; do
    echo "==> Database: $DB"

    # Product, Flash sale event, Flash sale item, Users
    run_psql "$DB" <<-EOSQL
        -- Idempotent inserts
        INSERT INTO products (id, name, slug, description, image_url, original_price)
        VALUES (
            '$PRODUCT_ID',
            'iPhone 15 Pro Max 256GB',
            'iphone-15-pro-max-256gb',
            'Apple iPhone 15 Pro Max - Titanium Design, A17 Pro chip, 256GB Storage',
            'https://via.placeholder.com/600x600.png?text=iPhone+15+Pro+Max',
            30000000
        )
        ON CONFLICT (id) DO NOTHING;

        -- Flash sale event (always active)
        INSERT INTO flash_sale_events (id, name, start_time, end_time, status)
        VALUES (
            '$FLASH_SALE_ID',
            'Demo Flash Sale — Benchmark',
            '$START_TIME',
            '$END_TIME',
            'active'
        )
        ON CONFLICT (id) DO UPDATE SET
            start_time = EXCLUDED.start_time,
            end_time = EXCLUDED.end_time,
            status = EXCLUDED.status;

        -- Flash sale item
        INSERT INTO flash_sale_items (id, event_id, product_id, sale_price, total_quantity, sold_quantity, max_per_user)
        VALUES (
            '$ITEM_ID',
            '$FLASH_SALE_ID',
            '$PRODUCT_ID',
            15000000,
            $INITIAL_STOCK,
            0,
            $MAX_PER_USER
        )
        ON CONFLICT (id) DO UPDATE SET
            total_quantity = EXCLUDED.total_quantity,
            sold_quantity = 0;

        -- Generate $SEEDED_USERS users using deterministic UUIDs
        -- User ID pattern: 33333333-3333-3333-3333-{n:012d}
        INSERT INTO users (id, email, password_hash, name, phone)
        SELECT
            ('33333333-3333-3333-3333-' || lpad(n::TEXT, 12, '0'))::UUID,
            'user' || n || '@apexstock.test',
            '\$2a\$10\$Dummy.Hash.For.Testing.Only.BCrypt.Cost10',
            'Test User ' || n,
            CASE WHEN n % 3 = 0 THEN '0901' || lpad((100000 + n)::TEXT, 6, '0') ELSE NULL END
        FROM generate_series(1, $SEEDED_USERS) AS n
        ON CONFLICT (id) DO NOTHING;
EOSQL

    echo "    ✓ Seeded $SEEDED_USERS users, 1 product, 1 active flash sale event, 1 item"
done

echo "==> Seed complete for both databases"
