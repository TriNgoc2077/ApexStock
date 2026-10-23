# ApexStock – API & Data Flow Design

> Phiên bản: 1.0 | Ngày: 2026-10-22

---

## 1. API Endpoints

### 1.1 Public APIs (không cần auth – dùng cho kịch bản kiểm thử tải)

| Method | Path | Mô tả |
|--------|------|--------|
| POST | `/api/v1/flash-sale/orders` | Đặt đơn flash sale (Baseline và Optimized dùng chung contract) |
| GET | `/api/v1/flash-sale/events/:id` | Lấy thông tin sự kiện + tồn kho thực từ Redis |
| GET | `/api/v1/health` | Health check |

#### POST `/api/v1/flash-sale/orders`

> Baseline chạy trên `:8081`, Optimized chạy trên `:8082`. Hai bên dùng chung schema và validation để benchmark công bằng.

**Request (mỗi order đúng 1 sản phẩm):**
```json
{
  "idempotency_key": "usr_42_item_7_ts_1792664379",
  "flash_sale_item_id": "fsi_7",
  "quantity": 1
}
```

**Response 201 (thành công):**
```json
{
  "order_id": "ord_8f3a",
  "status": "pending_payment",
  "expires_at": "2026-10-22T19:30:00Z",
  "total_amount": 19990000
}
```

**Response 409 (hết hàng):**
```json
{ "error": "out_of_stock", "message": "Sản phẩm đã hết hàng" }
```

**Response 429 (rate limit):**
```json
{ "error": "rate_limited", "retry_after": 1 }
```

**Response 201 (idempotent – đơn đã tồn tại):**
```json
{
  "order_id": "ord_8f3a",
  "status": "pending_payment",
  "expires_at": "2026-10-22T19:30:00Z",
  "idempotent": true
}
```

**Headers yêu cầu:**
- `Idempotency-Key: <uuid>` hoặc trong body
- Rate limit: 10 req/s per user_id, 5000 req/s global

---

### 1.2 Authenticated APIs (dùng cho giao diện người dùng)

#### Auth

| Method | Path | Mô tả |
|--------|------|--------|
| POST | `/api/v1/auth/register` | Đăng ký tài khoản |
| POST | `/api/v1/auth/login` | Đăng nhập, trả về JWT + refresh token |
| POST | `/api/v1/auth/refresh` | Làm mới access token |
| POST | `/api/v1/auth/logout` | Xoá session |
| GET | `/api/v1/auth/me` | Thông tin người dùng hiện tại |

**POST `/api/v1/auth/login` – Request:**
```json
{ "email": "user@example.com", "password": "secret123" }
```
**Response:**
```json
{
  "access_token": "eyJhbGci...",
  "refresh_token": "rt_abc123",
  "expires_in": 900
}
```

#### Sản phẩm & Sự kiện

| Method | Path | Mô tả |
|--------|------|--------|
| GET | `/api/v1/products` | Danh sách sản phẩm (phân trang) |
| GET | `/api/v1/products/:id` | Chi tiết sản phẩm |
| GET | `/api/v1/flash-sale/events` | Danh sách sự kiện (upcoming, active, ended) |
| GET | `/api/v1/flash-sale/events/:id` | Chi tiết sự kiện + tồn kho realtime từ Redis |

**GET `/api/v1/flash-sale/events/:id` – Response:**
```json
{
  "id": "fse_1",
  "name": "Flash Sale 12.12",
  "start_time": "2026-12-12T00:00:00Z",
  "end_time": "2026-12-12T02:00:00Z",
  "status": "active",
  "items": [
    {
      "id": "fsi_7",
      "product_id": "p_1",
      "product_name": "iPhone 17 Pro 256GB",
      "image_url": "https://...",
      "original_price": 32990000,
      "sale_price": 19990000,
      "total_quantity": 10000,
      "remaining_stock": 3421,
      "max_per_user": 1
    }
  ]
}
```

#### Đơn hàng (có auth)

| Method | Path | Mô tả |
|--------|------|--------|
| POST | `/api/v1/orders` | Tạo đơn hàng (từ UI, có auth) |
| GET | `/api/v1/orders` | Danh sách đơn của user |
| GET | `/api/v1/orders/:id` | Chi tiết đơn hàng |
| DELETE | `/api/v1/orders/:id` | Huỷ đơn hàng (nếu còn pending_payment) |

#### Thanh toán SePay

| Method | Path | Mô tả |
|--------|------|--------|
| POST | `/api/v1/payments/:order_id/initiate` | Tạo thông tin chuyển khoản + QR SePay |
| GET | `/api/v1/payments/:order_id/status` | Kiểm tra trạng thái thanh toán |
| POST | `/api/v1/webhooks/sepay` | Nhận webhook từ SePay (không cần auth, có signature) |

**POST `/api/v1/payments/:order_id/initiate` – Response:**
```json
{
  "bank_account": "9704229123456789",
  "bank_name": "Vietcombank",
  "account_name": "CONG TY APEXSTOCK",
  "amount": 19990000,
  "transfer_content": "APEXSTOCK ord8f3a",
  "qr_code_url": "https://img.vietqr.io/image/...",
  "expires_at": "2026-10-22T19:30:00Z"
}
```

---

### 1.3 War Room APIs

> Benchmark chạy tuần tự từng architecture. War Room dùng WebSocket cho live metrics. **Không có tính năng kill container từ UI**; operator kill container trực tiếp trong demo. Các trang người dùng dùng polling đơn giản.

| Method | Path | Mô tả |
|--------|------|--------|
| GET | `/api/v1/warroom/metrics` | Snapshot metrics cả 2 kiến trúc |
| GET | `/api/v1/warroom/health` | Trạng thái tất cả container |
| WS | `/api/v1/warroom/live` | Stream metrics realtime qua WebSocket |

**POST `/api/v1/warroom/chaos/kill` – Request:**
```json
{ "target": "apexstock-api-1", "auto_resume_after": 30 }
```

**WS `/api/v1/warroom/live` – Message mỗi 500ms:**
```json
{
  "ts": 1792664379,
  "baseline": {
    "rps": 312, "p50": 420, "p95": 1240, "p99": 3100,
    "error_rate": 0.032, "stock_remaining": 7821,
    "orders_success": 2179, "oversell": 0,
    "health": { "api": "up", "postgres": "up" }
  },
  "optimized": {
    "rps": 4821, "p50": 18, "p95": 62, "p99": 145,
    "error_rate": 0.001, "stock_remaining": 5124,
    "orders_success": 4876, "oversell": 0,
    "health": { "api": "up", "redis": "up", "queue": "up", "postgres": "up" }
  }
}
```

---

### 1.4 Admin APIs (nội bộ)

| Method | Path | Mô tả |
|--------|------|--------|
| POST | `/api/v1/admin/flash-sale/events` | Tạo sự kiện flash sale |
| POST | `/api/v1/admin/flash-sale/events/:id/items` | Thêm sản phẩm vào sự kiện |
| POST | `/api/v1/admin/flash-sale/events/:id/activate` | Kích hoạt sự kiện (warm up Redis) |
| GET | `/api/v1/admin/dashboard/stats` | Thống kê tổng quan |

---

## 2. Luồng đặt hàng Flash Sale (Optimized Architecture)

```mermaid
sequenceDiagram
    participant C as Client
    participant LB as Nginx LB
    participant API as Go API
    participant RC as Redis Cache
    participant RL as Redis Lua
    participant Q as NATS JetStream
    participant W as Worker
    participant DB as PostgreSQL

    C->>LB: POST /api/v1/flash-sale/orders {idempotency_key, flash_sale_item_id, quantity}
    LB->>API: Forward request (round-robin)

    Note over API: 1. Kiểm tra idempotency
    API->>RC: GET idempotency:{key}
    alt Đã tồn tại
        RC-->>API: {status, response}
        API-->>C: 201 Created (cached response, idempotent:true)
    end

    Note over API: 2. Rate limiting
    API->>RC: EVALSHA rate_limit.lua user_id
    alt Vượt rate limit
        RC-->>API: rejected
        API-->>C: 429 Too Many Requests
    end

    Note over API: 3. Đặt kho nguyên tử (Lua)
    API->>RL: EVALSHA reserve_stock.lua [item_id, user_id, qty, expiry_ts]
    alt Hết hàng
        RL-->>API: {ok:false, reason:"out_of_stock"}
        API-->>C: 409 Conflict
    else Vượt giới hạn mua
        RL-->>API: {ok:false, reason:"user_limit_exceeded"}
        API-->>C: 409 Conflict
    else Thành công
        RL-->>API: {ok:true, reserved:true}
    end

    Note over API: 4. Ghi idempotency key (NX)
    API->>RC: SET idempotency:{key} {pending} NX PX 86400000

    Note over API: 5. Publish message
    API->>Q: PUBLISH orders.created {order_id, user_id, flash_sale_item_id, quantity, reserved_at}
    Q-->>API: durable JetStream ack

    API-->>C: 201 Created {order_id, status:pending_payment, expires_at}

    Note over W,DB: Xử lý bất đồng bộ
    Q->>W: SUBSCRIBE orders.created
    W->>DB: INSERT INTO orders (status=pending_payment)
    W->>DB: INSERT INTO order_items
    W->>RC: UPDATE idempotency:{key} {status:created, order_id}

    Note over W: Sau expires_at (10 phút UI / 30s test)
    W->>RC: EVALSHA cleanup_expired.lua
    W->>Q: PUBLISH orders.expired {order_id}
    W->>DB: UPDATE orders SET status=expired
```

---

## 3. Luồng đặt hàng Baseline Architecture

```mermaid
sequenceDiagram
    participant C as Client
    participant API as Go API (single)
    participant DB as PostgreSQL

    C->>API: POST /flash-sale/order

    Note over API,DB: Tất cả trong 1 transaction
    API->>DB: BEGIN TRANSACTION
    API->>DB: SELECT * FROM flash_sale_items WHERE id=? FOR UPDATE
    
    alt Hết hàng
        API->>DB: ROLLBACK
        API-->>C: 409 Conflict
    end
    
    alt Vượt giới hạn user
        API->>DB: SELECT COUNT(*) FROM orders WHERE user_id=? AND item_id=?
        API->>DB: ROLLBACK
        API-->>C: 409 Conflict
    end

    API->>DB: UPDATE flash_sale_items SET sold_quantity = sold_quantity + ?
    API->>DB: INSERT INTO orders
    API->>DB: INSERT INTO order_items
    API->>DB: COMMIT

    API-->>C: 201 Created

    Note over API,DB: Bottleneck: Mỗi request giữ row lock trong suốt transaction
    Note over API,DB: 1 slow query = tất cả request sau phải chờ
```

---

## 4. Luồng thanh toán SePay

```mermaid
sequenceDiagram
    participant U as User (Browser)
    participant FE as Next.js Frontend
    participant API as Go API
    participant DB as PostgreSQL
    participant RC as Redis
    participant SP as SePay

    U->>FE: Bấm "Thanh toán ngay"
    FE->>API: POST /payments/:order_id/initiate
    API->>DB: SELECT order WHERE id=? AND status=pending_payment AND expires_at>NOW()
    alt Đơn đã hết hạn
        API-->>FE: 410 Gone
        FE-->>U: "Đơn hàng đã hết hạn"
    end
    API-->>FE: {bank_account, amount, qr_code_url, transfer_content}
    FE-->>U: Hiển thị QR + thông tin chuyển khoản + đồng hồ đếm ngược

    U->>SP: Chuyển khoản qua app ngân hàng
    SP->>API: POST /webhooks/sepay {transaction_ref, amount, content, checksum}

    Note over API: Xác thực webhook
    API->>API: HMAC verify checksum với SePay secret
    alt Checksum sai
        API-->>SP: 400 Bad Request
    end

    Note over API: Idempotency webhook
    API->>RC: SET webhook:{transaction_ref} NX EX 86400
    alt Đã xử lý
        API-->>SP: 200 OK (ack, skip duplicate)
    end

    Note over API: Đối soát đơn hàng
    API->>DB: SELECT order WHERE transfer_content LIKE '%ord_id%'
    alt Không tìm thấy đơn
        API->>DB: INSERT webhook_log (unmatched)
        API-->>SP: 200 OK
    end
    alt Đơn đã expired
        API->>DB: INSERT webhook_logs {order_id, status='refund', provider='simulator'}
        API-->>SP: 200 OK (admin xử lý refund sau)
    end
    alt Số tiền không khớp
        API->>DB: INSERT webhook_log (amount_mismatch)
        API-->>SP: 200 OK
    end

    Note over API: Cập nhật trạng thái chỉ khi đơn chưa expired
    API->>DB: UPDATE orders SET status=paid WHERE id=? AND status='pending_payment'
    API->>DB: INSERT INTO payments {transaction_ref, amount, status=completed}
    API->>RC: EVALSHA confirm_stock.lua [item_id] -- Xác nhận kho đã trừ
    API->>RC: SET webhook:{transaction_ref} "processed" EX 86400
    API-->>SP: 200 OK

    Note over FE,API: Polling hoặc WebSocket cập nhật UI
    FE->>API: GET /payments/:order_id/status (poll mỗi 3s)
    API-->>FE: {status: "paid"}
    FE-->>U: "Thanh toán thành công! 🎉"
```

---

## 5. Luồng Restock (Hoàn kho)

```mermaid
sequenceDiagram
    participant CR as Cron Job (mỗi 10s)
    participant RC as Redis
    participant Q as NATS JetStream
    participant W as Worker
    participant DB as PostgreSQL

    Note over CR: Quét đơn hàng hết hạn
    CR->>RC: EVALSHA cleanup_expired.lua
    RC-->>CR: [order_id_1, order_id_2, ...]

    loop Mỗi order hết hạn
        CR->>Q: PUBLISH orders.expired {order_id, item_id, qty}
    end

    Q->>W: SUBSCRIBE orders.expired
    W->>DB: SELECT order WHERE id=? AND status=pending_payment
    alt Order đã được xử lý (paid/cancelled)
        W-->>Q: ack (skip)
    end
    W->>DB: UPDATE orders SET status=expired WHERE id=? AND status=pending_payment
    W->>RC: EVALSHA release_stock.lua [item_id, user_id, qty]
    Note over RC: stock_remaining += qty
    Note over RC: user_purchase_count -= qty
    Note over RC: Remove from reserved sorted set
    W-->>Q: ack

    Note over W: Luồng thanh toán thất bại
    Q->>W: SUBSCRIBE orders.payment.failed
    W->>DB: UPDATE orders SET status=payment_failed
    W->>RC: EVALSHA release_stock.lua [item_id, user_id, qty]
    W-->>Q: ack
```

---

## 6. Luồng Resilience

### 6.1 Kịch bản: API instance bị kill

```mermaid
sequenceDiagram
    participant C as Clients
    participant LB as Nginx LB
    participant API1 as API Instance 1
    participant API2 as API Instance 2
    participant Q as NATS JetStream

    Note over C,API1: Trạng thái bình thường: traffic chia đều
    C->>LB: Requests
    LB->>API1: 50% requests
    LB->>API2: 50% requests

    Note over API1: docker kill apexstock-api-1
    API1--xLB: Connection lost
    LB->>LB: Health check fail → remove API1
    Note over LB: max_fails=3, fail_timeout=5s

    C->>LB: Requests tiếp tục
    LB->>API2: 100% requests (API2 vẫn hoạt động)

    Note over Q: Các message đang in-flight trên API1 KHÔNG mất
    Note over Q: NATS JetStream đảm bảo at-least-once delivery
    Note over Q: Worker consumer vẫn xử lý queue bình thường

    Note over LB: Sau restart, API1 vào lại pool
    API1->>LB: Health check pass
    LB->>API1: Resume routing
```

### 6.2 Kịch bản: Redis bị kill

```mermaid
sequenceDiagram
    participant API as Go API
    participant CB as Circuit Breaker
    participant RC as Redis
    participant Q as NATS JetStream
    participant W as Worker
    participant DB as PostgreSQL

    Note over RC: docker kill apexstock-redis
    API->>RC: Kết nối thất bại (timeout 50ms)
    CB->>CB: failure_count++ (threshold=5)
    CB->>CB: State: CLOSED → OPEN

    Note over API,CB: Trong thời gian Redis down
    API->>CB: Check circuit state
    CB-->>API: OPEN → reject fast
    API-->>API: Return 503 "Hệ thống đang bận, thử lại sau"

    Note over W: Worker vẫn xử lý queue (không cần Redis)
    Q->>W: SUBSCRIBE orders.*
    W->>DB: Write to PostgreSQL (không ảnh hưởng)

    Note over RC: Redis restart
    RC->>RC: Load từ persistence (AOF/RDB)
    
    Note over CB: Sau 30s, circuit thử HALF-OPEN
    CB->>CB: State: OPEN → HALF-OPEN
    API->>RC: Thử 1 request
    RC-->>API: Success
    CB->>CB: State: HALF-OPEN → CLOSED

    Note over API: Warm up Redis từ DB
    API->>DB: SELECT remaining stock per item
    API->>RC: SET flash_item:{id}:stock {value}
```

### 6.3 Kịch bản: PostgreSQL chậm

```mermaid
sequenceDiagram
    participant API as Go API
    participant Q as NATS JetStream
    participant W as Worker
    participant DB as PostgreSQL (slow)

    Note over DB: Giả lập: Toxiproxy thêm 2s latency
    API->>Q: PUBLISH orders.created (không block, <1ms)
    API-->>API: Return 201 ngay lập tức

    Q->>W: SUBSCRIBE orders.created
    W->>DB: INSERT orders (2s latency per query)
    DB-->>W: Timeout (3s limit)
    W->>W: Retry #1 (backoff 500ms)
    W->>DB: INSERT orders
    DB-->>W: Success (DB recovered)
    W-->>Q: ack

    Note over Q: Queue depth tăng trong lúc DB chậm
    Note over Q: Worker retry với exponential backoff (500ms, 1s, 2s, 4s, 8s)
    Note over Q: Sau MaxDeliver=5, message vào Dead Letter Queue
    Note over Q: Alert khi DLQ có messages
```

---

## 7. Idempotency – Thiết kế chi tiết

### 7.1 State machine

```mermaid
stateDiagram-v2
    [*] --> ABSENT: Lần đầu request
    ABSENT --> PROCESSING: SET NX thành công
    PROCESSING --> CREATED: Order tạo xong
    PROCESSING --> FAILED: Lỗi (stock, validation...)
    CREATED --> [*]: TTL 24h hết hạn
    FAILED --> [*]: TTL 24h hết hạn

    ABSENT --> RETURN_CACHED: Key đã tồn tại
    RETURN_CACHED --> [*]
```

### 7.2 Race condition: 2 request cùng idempotency key đến đồng thời

```
T=0ms: Request A và Request B cùng GET idempotency:{key} → cả hai thấy ABSENT
T=1ms: Request A: SET idempotency:{key} "processing" NX PX 30000 → OK (A thắng)
T=1ms: Request B: SET idempotency:{key} "processing" NX PX 30000 → FAIL (B thua)
T=2ms: Request B: GET idempotency:{key} → "processing"
T=2ms: Request B: Trả về 202 Accepted { message: "Đang xử lý" }
T=50ms: Request A: Xử lý xong → SET idempotency:{key} {status:created, order_id:...} PX 86400000
T=3s: Client retry Request B → GET idempotency:{key} → {status:created, order_id:...}
T=3s: Request B: Trả về 200 { order_id, status:pending_payment, idempotent:true }
```

### 7.3 Redis key schema cho idempotency

```
Key:   idempotency:{idempotency_key}
Type:  HASH
TTL:   86400s (24 giờ)

Fields:
  status        "processing" | "created" | "failed"
  order_id      "ord_8f3a" (sau khi tạo xong)
  error_code    "out_of_stock" (nếu failed)
  created_at    Unix timestamp
  user_id       "usr_42"
```

### 7.4 Cleanup

- TTL tự động xoá key sau 24h
- Cron job hàng tuần xoá bản ghi `idempotency_keys` trong PostgreSQL quá 30 ngày

---

## 8. SePay Webhook – Edge Cases

| Tình huống | Xử lý |
|-----------|--------|
| Webhook đến đúng giờ, số tiền đúng | Cập nhật `paid`, xác nhận kho |
| Webhook trùng (retry từ SePay) | Redis NX check → skip, trả 200 OK |
| Webhook đến sau khi đơn đã `expired` | Ghi `webhook_log` với `late_payment`, đánh dấu `refund_pending` |
| Số tiền không khớp (thiếu/thừa) | Ghi `amount_mismatch`, cần xử lý thủ công |
| Checksum sai | Trả 400, log security warning |
| Không tìm thấy đơn | Ghi `unmatched`, trả 200 OK |
| Thanh toán cho đơn đã `paid` (duplicate) | Skip, trả 200 OK |

---

## 9. Tóm tắt các quyết định thiết kế

| Quyết định | Lựa chọn | Lý do |
|-----------|---------|-------|
| Idempotency store | Redis HASH + PostgreSQL backup | Redis cho speed, PG cho audit |
| Stock deduction timing | Tại request (Redis Lua), trước khi tạo đơn | Ngăn oversell, không cần saga rollback phức tạp |
| Đồng bộ Redis–DB | Worker bất đồng bộ qua queue | API không block, DB writes có thể retry |
| Webhook idempotency | Redis SET NX | Tự xoá sau 24h, đủ cho SePay retry window |
| Payment timeout | Redis Sorted Set + Cron 10s | Chính xác tới 10s, đủ cho demo |
| Circuit breaker state | Redis shared | Tất cả API instance dùng chung state |
