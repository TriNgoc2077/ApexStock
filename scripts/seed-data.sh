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
INITIAL_STOCK="${INITIAL_STOCK:-10000}"
MAX_PER_USER="${MAX_PER_USER:-5}"
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
        -- Idempotent inserts for products
        INSERT INTO products (id, name, slug, description, image_url, original_price)
        VALUES 
        (
            '$PRODUCT_ID',
            'iPhone 15 Pro Max 256GB',
            'iphone-15-pro-max-256gb',
            'Apple iPhone 15 Pro Max - Titanium Design, Chip A17 Pro siêu mạnh mẽ, 256GB Storage, Camera 48MP zoom 5x',
            '/images/products/iphone15.jpg',
            30000000
        ),
        (
            '33333333-3333-3333-3333-333333333334',
            'MacBook Pro 14 M3 Pro (18GB/512GB)',
            'macbook-pro-14-m3-pro',
            'Apple MacBook Pro 14 inch chip Apple M3 Pro, 18GB Unified Memory, 512GB SSD, màn hình Liquid Retina XDR 120Hz',
            '/images/products/macbook.jpg',
            49990000
        ),
        (
            '33333333-3333-3333-3333-333333333335',
            'Tai Nghe Sony WH-1000XM5 Hi-Res Audio',
            'sony-wh-1000xm5',
            'Tai nghe chống ồn đỉnh cao Sony WH-1000XM5 với bộ xử lý chống ồn V1/QN1, 30 giờ pin, đàm thoại AI sắc nét',
            '/images/products/sony-xm5.jpg',
            8490000
        ),
        (
            '33333333-3333-3333-3333-333333333336',
            'Apple Watch Ultra 2 GPS + Cellular 49mm',
            'apple-watch-ultra-2',
            'Đồng hồ thể thao chuyên nghiệp vỏ Titan siêu bền, màn hình 3000 nits, định vị GPS kép chính xác, pin 72 giờ',
            '/images/products/apple-watch.jpg',
            21990000
        ),
        (
            '33333333-3333-3333-3333-333333333337',
            'iPad Pro M4 11 inch 256GB Wi-Fi',
            'ipad-pro-m4-11',
            'iPad Pro thế hệ mới siêu mỏng 5.3mm, chip Apple M4 đột phá, màn hình Ultra Retina XDR OLED kép siêu sáng',
            '/images/products/ipad.jpg',
            28990000
        ),
        (
            '33333333-3333-3333-3333-333333333338',
            'Bàn Phím Cơ Custom Keychron Q1 Pro',
            'keychron-q1-pro-wireless',
            'Bàn phím cơ vỏ nhôm CNC nguyên khối, gasket mount kép, kết nối Bluetooth 5.1/Type-C, hotswap switch, keycap PVD',
            '/images/products/keyboard.jpg',
            4690000
        ),
        (
            '33333333-3333-3333-3333-333333333339',
            'Tai Nghe Apple AirPods Pro 2 (USB-C)',
            'airpods-pro-2-usbc',
            'AirPods Pro thế hệ 2 hộp sạc MagSafe USB-C, chip H2 khử tiếng ồn chủ động gấp 2 lần, âm thanh thích ứng',
            '/images/products/airpods.jpg',
            6190000
        ),
        (
            '33333333-3333-3333-3333-333333333340',
            'Samsung Galaxy S24 Ultra 256GB AI Phone',
            'samsung-galaxy-s24-ultra',
            'Khung viền Titan sang trọng, Galaxy AI dịch trực tiếp cuộc gọi, khoanh tròn tìm kiếm, Snapdragon 8 Gen 3 for Galaxy',
            '/images/products/samsung.jpg',
            31990000
        )
        ON CONFLICT (id) DO UPDATE SET
            image_url = EXCLUDED.image_url,
            description = EXCLUDED.description,
            original_price = EXCLUDED.original_price;

        -- Flash sale event (always active)
        INSERT INTO flash_sale_events (id, name, start_time, end_time, status)
        VALUES (
            '$FLASH_SALE_ID',
            'Siêu Giảm Giá Công Nghệ 2026 — Flash Sale Deal Chớp Nhoáng',
            '$START_TIME',
            '$END_TIME',
            'active'
        )
        ON CONFLICT (id) DO UPDATE SET
            name = EXCLUDED.name,
            start_time = EXCLUDED.start_time,
            end_time = EXCLUDED.end_time,
            status = EXCLUDED.status;

        -- Flash sale items
        INSERT INTO flash_sale_items (id, event_id, product_id, sale_price, total_quantity, sold_quantity, max_per_user)
        VALUES 
        (
            '$ITEM_ID',
            '$FLASH_SALE_ID',
            '$PRODUCT_ID',
            15000000,
            $INITIAL_STOCK,
            0,
            $MAX_PER_USER
        ),
        (
            '22222222-2222-2222-2222-222222222223',
            '$FLASH_SALE_ID',
            '33333333-3333-3333-3333-333333333334',
            38990000,
            500,
            0,
            2
        ),
        (
            '22222222-2222-2222-2222-222222222224',
            '$FLASH_SALE_ID',
            '33333333-3333-3333-3333-333333333335',
            6490000,
            1000,
            0,
            3
        ),
        (
            '22222222-2222-2222-2222-222222222225',
            '$FLASH_SALE_ID',
            '33333333-3333-3333-3333-333333333336',
            18290000,
            800,
            0,
            2
        ),
        (
            '22222222-2222-2222-2222-222222222226',
            '$FLASH_SALE_ID',
            '33333333-3333-3333-3333-333333333337',
            25490000,
            600,
            0,
            2
        ),
        (
            '22222222-2222-2222-2222-222222222227',
            '$FLASH_SALE_ID',
            '33333333-3333-3333-3333-333333333338',
            3890000,
            1200,
            0,
            5
        ),
        (
            '22222222-2222-2222-2222-222222222228',
            '$FLASH_SALE_ID',
            '33333333-3333-3333-3333-333333333339',
            4890000,
            1500,
            0,
            3
        ),
        (
            '22222222-2222-2222-2222-222222222229',
            '$FLASH_SALE_ID',
            '33333333-3333-3333-3333-333333333340',
            24990000,
            800,
            0,
            2
        )
        ON CONFLICT (id) DO UPDATE SET
            sale_price = EXCLUDED.sale_price,
            total_quantity = EXCLUDED.total_quantity,
            max_per_user = EXCLUDED.max_per_user;

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
