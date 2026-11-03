#!/usr/bin/env bash
# Displays order breakdown (Paid vs Cancelled/Restocked vs Pending) and inventory conservation table
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

echo "================================================================================"
echo " 📦 APEXSTOCK — BẢNG ĐỐI SOÁT ĐƠN HÀNG & HOÀN KHO (PAYMENT & RESTOCK SUMMARY)"
echo "================================================================================"

for db in apexstock_baseline apexstock_optimized; do
  echo ""
  echo ">>> Kiến trúc: $db"
  run_psql "$db" -c "
    WITH stats AS (
      SELECT
        fsi.id,
        fsi.total_quantity AS initial_stock,
        COUNT(o.id) AS total_orders,
        COALESCE(SUM(oi.quantity) FILTER (WHERE o.status = 'paid'), 0) AS paid_qty,
        COALESCE(SUM(oi.quantity) FILTER (WHERE o.status IN ('cancelled', 'expired', 'payment_failed')), 0) AS restocked_qty,
        COALESCE(SUM(oi.quantity) FILTER (WHERE o.status = 'pending_payment' AND o.expires_at > now()), 0) AS reserved_qty
      FROM flash_sale_items fsi
      LEFT JOIN order_items oi ON oi.flash_sale_item_id = fsi.id
      LEFT JOIN orders o ON o.id = oi.order_id
      GROUP BY fsi.id, fsi.total_quantity
    )
    SELECT
      initial_stock AS \"Kho Ban Đầu\",
      total_orders AS \"Tổng Đơn Đã Tạo\",
      paid_qty AS \"Thành Công (Paid ~50%)\",
      restocked_qty AS \"Hủy & Hoàn Kho (Restocked ~50%)\",
      reserved_qty AS \"Đang Giữ Chỗ (Pending)\",
      (initial_stock - paid_qty - reserved_qty) AS \"Kho Còn Lại (Available)\"
    FROM stats;
  "
done

echo "================================================================================"
echo " 🛡️ Kiểm tra bảo toàn tồn kho (Invariant Reconciliation):"
"$ROOT_DIR/scripts/reconcile.sql.sh"
echo "================================================================================"
