# API Contract — Flash Sale Orders

> **Phiên bản:** 1.0  
> **Ngày:** 2026-10-23  
> **Quyết định liên quan:** D0.1, D0.2, D0.3, D0.4

---

## Tổng quan

Baseline và Optimized dùng **chung 1 endpoint và schema** để đảm bảo tính công bằng trong benchmark.

---

## Endpoint chung

```
POST /api/v1/flash-sale/orders
```

**Phân tách port:**
- **Baseline:** `:8081`
- **Optimized:** `:8082`

k6 và frontend sử dụng base URL khác nhau để chọn implementation.

---

## Request Schema

```json
{
  "flash_sale_item_id": 123,
  "quantity": 1,
  "idempotency_key": "user123_item456_20261023T120000"
}
```

**Validation:**
- `flash_sale_item_id`: required, integer > 0
- `quantity`: required, **phải bằng 1** (D0.4 — mỗi order chỉ mua 1 sản phẩm)
- `idempotency_key`: required, string, max 255 chars

**Idempotency:**
- Cùng key + cùng payload → trả cùng response (201 hoặc 409)
- Cùng key + khác payload → 409 Conflict với error khác biệt
- TTL: 24h

---

## Response — Success

**Status:** `201 Created` (D0.1)

```json
{
  "order_id": 789,
  "status": "pending_payment",
  "payment": {
    "payment_id": 101,
    "status": "pending",
    "amount": 100000,
    "expires_at": "2026-10-23T12:15:00Z",
    "qr_code_url": null,
    "payment_url": null
  },
  "reserved_quantity": 1,
  "reserved_until": "2026-10-23T12:15:00Z"
}
```

**Durability (D0.1):**
- Optimized sử dụng **NATS JetStream (persistent on disk)**
- Order không bị mất khi crash
- Thanh toán ngay lập tức (không async 202)

---

## Response — Errors

### 409 Conflict — Out of Stock

```json
{
  "error": "out_of_stock",
  "message": "Sản phẩm đã hết hàng",
  "details": {
    "flash_sale_item_id": 123,
    "available": 0
  }
}
```

### 409 Conflict — Max Per User Reached

```json
{
  "error": "max_per_user_reached",
  "message": "Bạn đã đạt giới hạn mua cho sản phẩm này",
  "details": {
    "flash_sale_item_id": 123,
    "max_per_user": 2,
    "user_purchased": 2
  }
}
```

### 409 Conflict — Idempotency Mismatch

```json
{
  "error": "idempotency_conflict",
  "message": "Idempotency key đã được dùng với payload khác"
}
```

### 429 Too Many Requests

```json
{
  "error": "rate_limit_exceeded",
  "message": "Quá nhiều request, vui lòng thử lại sau",
  "retry_after": 5
}
```

### 400 Bad Request

```json
{
  "error": "validation_error",
  "message": "Dữ liệu không hợp lệ",
  "details": {
    "quantity": "quantity phải bằng 1"
  }
}
```

### 5xx Server Error

```json
{
  "error": "internal_server_error",
  "message": "Lỗi hệ thống, vui lòng thử lại"
}
```

---

## Inventory State Model (D0.3)

Tách rõ 3 state:

1. **`available`** — Số lượng còn lại có thể reserve
2. **`reserved`** — Đã reserve, chờ thanh toán
3. **`paid`** — Đã thanh toán thành công

**Công thức:**
```
total_quantity = available + reserved + paid
```

**Baseline:**
- Trừ trực tiếp DB khi order thành công
- `sold_quantity` tăng ngay

**Optimized:**
- Reserve trong Redis
- `sold_quantity` DB chỉ tăng khi paid
- Worker đồng bộ từ NATS

---

## Performance Targets (D0.1)

- `<5ms` response time là **hypothesis**, không phải acceptance criteria
- Sẽ được đo trong benchmark thực tế
- Report chỉ dùng measured values

---

## Validation Rules

Cả Baseline và Optimized validate như nhau:

- Flash sale phải đang active (`start_time <= now() < end_time`)
- User đã authenticate (JWT valid)
- `quantity` phải bằng 1
- `max_per_user` không vượt quá
- Stock available đủ

---

## Single Item Constraint (D0.4)

**Quy tắc:**
- Mỗi order flash sale chỉ được mua **đúng 1 sản phẩm**
- Nếu `quantity != 1` → 400 Bad Request
- Multi-item order nằm ngoài phạm vi hiện tại
- Có thể mở rộng trong tương lai (giỏ hàng nhiều món bình thường + 1 món flash sale)

**Lý do:**
- Đơn giản hóa Lua transaction (all-or-nothing)
- Tránh reserve dở dang
- Phù hợp scope demo

---

## Testing

**Contract tests phải cover:**
- Happy path → 201
- Out of stock → 409
- Max per user → 409
- Idempotency same payload → 201 same response
- Idempotency different payload → 409
- Invalid quantity (!=1) → 400
- Rate limit → 429
- Concurrent same item → no oversell
- Concurrent same key → đúng 1 order

---

**Người maintain:** Backend Team  
**Review:** Tech Lead, QA
