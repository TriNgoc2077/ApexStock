# Inventory State Model

> **Phiên bản:** 1.0  
> **Ngày:** 2026-10-23  
> **Quyết định liên quan:** D0.3

---

## Tổng quan

Inventory state được tách rõ thành 3 trạng thái để tránh nhầm lẫn giữa Baseline và Optimized, đảm bảo reconciliation chính xác.

---

## Ba trạng thái

### 1. Available (Còn lại)
- Số lượng **có thể reserve** ngay lúc này
- Chưa được reserve bởi order nào

### 2. Reserved (Đã giữ chỗ)
- Đã được reserve bởi order
- Đang chờ thanh toán
- Có TTL (thường 5–15 phút)

### 3. Paid (Đã bán)
- Đã thanh toán thành công
- Không thể release lại

---

## Công thức Invariant

```
total_quantity = available + reserved + paid
```

**Luôn phải đúng** tại mọi thời điểm.

---

## Baseline Implementation

### Schema
```sql
CREATE TABLE flash_sale_items (
  id BIGSERIAL PRIMARY KEY,
  flash_sale_id BIGINT REFERENCES flash_sales(id),
  product_id BIGINT REFERENCES products(id),
  total_quantity INT NOT NULL,
  sold_quantity INT NOT NULL DEFAULT 0,  -- tăng ngay khi order
  max_per_user INT NOT NULL DEFAULT 1
);
```

### Semantics
- `available = total_quantity - sold_quantity`
- `sold_quantity` **tăng ngay** khi order thành công (pending_payment)
- Transaction: `SELECT ... FOR UPDATE` → kiểm tra stock → trừ `sold_quantity` → insert order

### Projection
```sql
-- Available
SELECT total_quantity - sold_quantity AS available
FROM flash_sale_items
WHERE id = ?;

-- Reserved (from orders table)
SELECT COALESCE(SUM(quantity), 0) AS reserved
FROM orders
WHERE flash_sale_item_id = ?
  AND status = 'pending_payment';

-- Paid
SELECT COALESCE(SUM(quantity), 0) AS paid
FROM orders
WHERE flash_sale_item_id = ?
  AND status = 'paid';
```

---

## Optimized Implementation

### Redis Keys
```
stock:item:{item_id}            → available (integer)
reservations:item:{item_id}     → sorted set (score = expires_at)
confirmed:item:{item_id}        → integer (paid count)
```

### Semantics
- Reserve trong Redis atomic (Lua script)
- Publish event to NATS JetStream
- Worker consume và ghi DB
- `sold_quantity` DB **chỉ tăng khi paid** (sau worker confirm)

### Lua Script — Reserve
```lua
-- reserve.lua
local item_key = KEYS[1]
local reservation_key = KEYS[2]
local available = tonumber(redis.call('GET', item_key) or '0')
local quantity = tonumber(ARGV[1])
local reservation_id = ARGV[2]
local expires_at = tonumber(ARGV[3])

if available < quantity then
  return {err = 'out_of_stock', available = available}
end

redis.call('DECRBY', item_key, quantity)
redis.call('ZADD', reservation_key, expires_at, reservation_id)

return {ok = 'reserved', new_available = available - quantity}
```

### Projection
```
available   = GET stock:item:{item_id}
reserved    = ZCOUNT reservations:item:{item_id} -inf +inf
paid        = GET confirmed:item:{item_id}
```

---

## State Transitions

### Reserve
```
available → reserved
```
- Baseline: transaction DB
- Optimized: Lua atomic

### Confirm (Payment Success)
```
reserved → paid
```
- Release từ reservations
- Tăng confirmed/paid count
- Idempotent

### Release (Payment Failed/Timeout/Cancel)
```
reserved → available
```
- Hoàn lại stock
- Idempotent (không hoàn 2 lần)

### Expired Cleanup
```
reserved (expired) → available
```
- Background job quét expired reservations
- Release tự động

---

## Reconciliation

### Query Baseline
```sql
SELECT 
  fsi.id AS item_id,
  fsi.total_quantity,
  fsi.sold_quantity AS baseline_sold,
  COALESCE(SUM(CASE WHEN o.status = 'pending_payment' THEN o.quantity ELSE 0 END), 0) AS reserved,
  COALESCE(SUM(CASE WHEN o.status = 'paid' THEN o.quantity ELSE 0 END), 0) AS paid,
  (fsi.total_quantity - fsi.sold_quantity) AS available
FROM flash_sale_items fsi
LEFT JOIN orders o ON o.flash_sale_item_id = fsi.id
WHERE fsi.id = ?
GROUP BY fsi.id;
```

### Query Optimized
```redis
GET stock:item:{item_id}                     -- available
ZCOUNT reservations:item:{item_id} -inf +inf -- reserved
GET confirmed:item:{item_id}                 -- paid
```

```sql
-- Cross-check với DB
SELECT 
  fsi.id,
  fsi.total_quantity,
  COUNT(*) FILTER (WHERE o.status = 'pending_payment') AS db_reserved,
  COUNT(*) FILTER (WHERE o.status = 'paid') AS db_paid
FROM flash_sale_items fsi
LEFT JOIN orders o ON o.flash_sale_item_id = fsi.id
WHERE fsi.id = ?
GROUP BY fsi.id;
```

### Invariant Check
```python
# Baseline
assert total == baseline_sold + available
assert baseline_sold == reserved + paid

# Optimized
assert total == available + reserved + paid
assert redis_available == db_total - db_reserved - db_paid
```

---

## Metrics

### Prometheus Metrics
```go
// Baseline
flash_sale_stock_available{arch="baseline",item_id="123"}
flash_sale_stock_reserved{arch="baseline",item_id="123"}
flash_sale_stock_paid{arch="baseline",item_id="123"}

// Optimized
flash_sale_stock_available{arch="optimized",item_id="123"}
flash_sale_stock_reserved{arch="optimized",item_id="123"}
flash_sale_stock_paid{arch="optimized",item_id="123"}

// Invariant violation
flash_sale_stock_oversold_total{arch="baseline"} 0
flash_sale_stock_oversold_total{arch="optimized"} 0
```

---

## Testing

### Unit Tests
- Reserve đủ stock → success
- Reserve không đủ stock → out_of_stock
- Concurrent reserve → no oversell

### Integration Tests
- Reserve + confirm → paid tăng, reserved giảm
- Reserve + release → available phục hồi
- Reserve + timeout → cleanup tự động

### Concurrency Tests
- 100 concurrent reserve cùng item
- Check invariant sau test
- Không stock âm

---

## Known Limitations

### Baseline
- Lock contention cao khi concurrent
- Không scale horizontal dễ dàng

### Optimized
- Eventual consistency giữa Redis và DB
- Cần reconciliation định kỳ
- Redis down → service unavailable (circuit breaker)

---

**Người maintain:** Backend Team  
**Review:** Tech Lead
