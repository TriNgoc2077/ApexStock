# SePay Local Demo & Webhook Verification Runbook

## 1. Mục Đích & Nguyên Tắc Bảo Mật
- Cho phép demo tính năng thanh toán quét mã QR (VietQR) và nhận webhook thực tế từ cổng thanh toán SePay.
- **Cô lập bí mật (Decision Gate D0.6):** Không commit bất kỳ token, secret, số tài khoản thật vào Git repository. Toàn bộ cấu hình thông qua biến môi trường.
- **Cô lập tải:** Mọi bài test tải (k6 load testing) đều sử dụng Payment Simulator (`POST /api/v1/payments/simulate`), tuyệt đối KHÔNG gọi SePay trong benchmark.

## 2. Cấu Hình Môi Trường
Thiết lập các biến môi trường trước khi khởi động API:
```bash
export SEPAY_WEBHOOK_SECRET="local-dev-sepay-secret-2026"
export SEPAY_ACCOUNT_NUMBER="0388888888"
export SEPAY_BANK_CODE="MBBank"
```

## 3. Quy Trình Demo Qua Tunnel (Ngrok / Cloudflare Tunnel)
Để nhận webhook thật từ máy chủ SePay về localhost:
```bash
# Sử dụng Cloudflare Tunnel (khuyến nghị, không cần tài khoản)
cloudflared tunnel --url http://localhost:8082

# Hoặc sử dụng ngrok
ngrok http 8082
```
Cấu hình Webhook URL trên Dashboard SePay:
`https://<subdomain>.trycloudflare.com/api/v1/webhooks/sepay`
Authentication header: `Apikey local-dev-sepay-secret-2026`

## 4. Kiểm Thử Thủ Công Bằng cURL

### Bước 1: Tạo đơn hàng
```bash
ORDER_RESP=$(curl -s -X POST http://localhost:8082/api/v1/flash-sale/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: demo-sepay-$(date +%s)" \
  -d '{
    "user_id": "00000000-0000-0000-0000-000000000001",
    "flash_sale_item_id": "00000000-0000-0000-0000-000000000001",
    "quantity": 1
  }')
echo $ORDER_RESP
ORDER_ID=$(echo $ORDER_RESP | jq -r .order_id)
```

### Bước 2: Tạo mã QR thanh toán (Initiate)
```bash
curl -s -X POST http://localhost:8082/api/v1/payments/sepay/initiate \
  -H "Content-Type: application/json" \
  -d "{\"order_id\": \"$ORDER_ID\"}" | jq .
```
Kết quả trả về:
- `qr_url`: Link ảnh VietQR tương ứng với số tài khoản và nội dung chuyển khoản `DH <order_id>`.
- `transfer_content`: Cú pháp chuyển khoản chính xác để SePay webhook đối soát.

### Bước 3: Gửi Webhook thanh toán thành công
```bash
curl -s -X POST http://localhost:8082/api/v1/webhooks/sepay \
  -H "Content-Type: application/json" \
  -H "Authorization: Apikey local-dev-sepay-secret-2026" \
  -d "{
    \"id\": $(date +%s),
    \"gateway\": \"MBBank\",
    \"transactionDate\": \"$(date '+%Y-%m-%d %H:%M:%S')\",
    \"accountNumber\": \"0388888888\",
    \"content\": \"DH $ORDER_ID\",
    \"transferType\": \"in\",
    \"transferAmount\": 100000,
    \"referenceCode\": \"REF$(date +%s)\"
  }" | jq .
```
Kỳ vọng: HTTP 200 `{"status": "ok", "order_status": "paid", "payment_status": "success", "idempotent_replay": false}`.

### Bước 4: Kiểm tra tính Idempotent (Gửi lại Webhook trùng lặp)
Gửi lại cURL ở Bước 3.
Kỳ vọng: HTTP 200 `{"status": "ok", "idempotent_replay": true}`, không trừ thêm kho, không lỗi.

### Bước 5: Kiểm tra thanh toán muộn sau khi đơn hết hạn (Decision Gate D0.7)
1. Đơn hàng chuyển sang trạng thái `expired` (hoặc test với order đã expired).
2. Gửi webhook thanh toán thành công cho đơn đã expired.
3. Kỳ vọng:
   - Trạng thái trả về: `{"status": "refund_logged", "order_id": "$ORDER_ID", "reason": "order_already_expired"}`.
   - Database `orders.status` vẫn giữ nguyên là `expired`.
   - Bảng `webhook_logs` xuất hiện 1 bản ghi với `status = 'refund'` để CSKH/kế toán hoàn tiền thủ công.

### Bước 6: Kiểm tra bảo mật (Secret không hợp lệ)
```bash
curl -s -i -X POST http://localhost:8082/api/v1/webhooks/sepay \
  -H "Content-Type: application/json" \
  -H "Authorization: Apikey wrong-secret" \
  -d @test/fixtures/sepay_webhook_valid.json
```
Kỳ vọng: HTTP 401 Unauthorized, không thay đổi bất kỳ bản ghi DB nào.
