# API Contract — Flash Sale Orders

> **Phiên bản:** 1.1 (Đồng bộ UUID theo `02-database-and-data.md` & `benchmark-matrix.yaml`)  
> **Ngày:** 2026-10-23  
> **Quyết định liên quan:** D0.1, D0.2, D0.3, D0.4, D0.6, D0.7

---

## 1. Endpoint chung (D0.2)

```http
POST /api/v1/flash-sale/orders
```

**Phân tách port:**
- **Baseline:** `:8081` (`http://localhost:8081`)
- **Optimized:** `:8082` (`http://localhost:8082`)

k6 và frontend sử dụng base URL khác nhau để chọn implementation nhưng gửi cùng một request schema.

---

## 2. Request Schema

**Headers hỗ trợ:**
- `Idempotency-Key: <string>` (có thể truyền qua header hoặc trường `idempotency_key` trong body; nếu truyền cả hai thì phải khớp nhau)
- `X-User-ID: <uuid>` hoặc `Authorization: Bearer <jwt>` (hoặc truyền `user_id` trong body khi chạy k6 benchmark)
- `X-Correlation-ID: <string>` (tùy chọn; nếu thiếu server tự sinh UUID)

**JSON Body:**
```json
{
  "user_id": "33333333-3333-3333-3333-000000000001",
  "flash_sale_item_id": "22222222-2222-2222-2222-222222222222",
  "quantity": 1,
  "idempotency_key": "usr_1_item_2222_attempt_1"
}
```

**Validation Rules (cả Baseline và Optimized kiểm tra giống hệt nhau):**
- `user_id`: required (từ body, `X-User-ID` header hoặc JWT claim), định dạng UUID hợp lệ.
- `flash_sale_item_id`: required, định dạng UUID hợp lệ (hỗ trợ alias `item_id` để tương thích ngược).
- `quantity`: required, **bắt buộc bằng `1`** (D0.4 — nếu `quantity != 1` trả về `400 Bad Request` với `error: "validation_error"`).
- `idempotency_key`: required (từ header `Idempotency-Key` hoặc body `idempotency_key`), string độ dài `1..255`.

---

## 3. Response — Success (`201 Created` theo D0.1)

```json
{
  "order_id": "99999999-9999-4999-8999-000000000001",
  "reservation_id": "88888888-8888-4888-8888-000000000001",
  "user_id": "33333333-3333-3333-3333-000000000001",
  "flash_sale_item_id": "22222222-2222-2222-2222-222222222222",
  "quantity": 1,
  "total_amount": 19990000,
  "status": "pending_payment",
  "payment_status": "pending",
  "expires_at": "2026-10-23T12:15:00Z",
  "created_at": "2026-10-23T12:10:00Z",
  "idempotent": false
}
```
*(Nếu là request retry cùng `idempotency_key` + cùng payload đã thành công trước đó: trả về `201 Created` hoặc `200 OK` cùng payload đơn hàng với `"idempotent": true`).*

---

## 4. Response — Errors (Chuẩn hóa cho k6 & Frontend)

### `400 Bad Request` — Validation Error (`quantity != 1`, thiếu trường, sai UUID)
```json
{
  "error": "validation_error",
  "message": "Dữ liệu không hợp lệ: quantity phải bằng 1",
  "details": {
    "field": "quantity"
  }
}
```

### `404 Not Found` — Không tìm thấy Flash Sale Item
```json
{
  "error": "item_not_found",
  "message": "Không tìm thấy sản phẩm flash sale"
}
```

### `409 Conflict` — Hết hàng (`out_of_stock`)
```json
{
  "error": "out_of_stock",
  "message": "Sản phẩm đã hết hàng",
  "details": {
    "flash_sale_item_id": "22222222-2222-2222-2222-222222222222",
    "available": 0
  }
}
```

### `409 Conflict` — Vượt giới hạn mua mỗi người (`max_per_user_reached`)
```json
{
  "error": "max_per_user_reached",
  "message": "Bạn đã đạt giới hạn mua cho sản phẩm này"
}
```

### `409 Conflict` — Idempotency Key trùng nhưng khác Payload (`idempotency_conflict`)
```json
{
  "error": "idempotency_conflict",
  "message": "Idempotency key đã được dùng với payload khác"
}
```

### `409 Conflict` — Request cùng Key đang được xử lý đồng thời (`request_in_progress`)
```json
{
  "error": "request_in_progress",
  "message": "Request với cùng idempotency key đang được xử lý"
}
```

### `429 Too Many Requests` — Rate Limit
```json
{
  "error": "rate_limit_exceeded",
  "message": "Quá nhiều request, vui lòng thử lại sau",
  "retry_after": 1
}
```

### `503 Service Unavailable` / `500 Internal Server Error`
```json
{
  "error": "service_unavailable",
  "message": "Hệ thống tạm thời gián đoạn, vui lòng thử lại"
}
```
