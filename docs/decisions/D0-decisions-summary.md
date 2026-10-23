# Decision Gate D0 — Tổng hợp các quyết định đã chốt

> **Ngày:** 2026-10-23  
> **Trạng thái:** Đã chốt tất cả 14 quyết định  
> **Người quyết định:** Product Owner/Tech Lead

---

## Tổng quan

Decision Gate D0 giải quyết 14 mâu thuẫn/chỗ thiếu contract trong tài liệu thiết kế. Tất cả các quyết định này đã được chốt và phải được tuân thủ trong toàn bộ quá trình triển khai.

---

## D0.1 — Order Durability và API Response

**Vấn đề:** Optimized được mô tả vừa sync DB vừa async publish; API lúc 201 lúc 202; có chỗ nói `<5ms`.

**Quyết định:**
- API trả **201 Created**
- Thanh toán ngay lập tức
- Dùng **NATS JetStream (persistent on disk)** để không mất order
- `<5ms` là **target hypothesis**, không phải kết quả nghiệm thu

**Lý do:** Đảm bảo durability với NATS persistence, response semantics nhất quán, và tránh over-promise về performance trước khi đo.

---

## D0.2 — API Contract chung

**Vấn đề:** Path khác nhau giữa `/flash-sale/order`, `/optimized/orders`, `/baseline/orders`.

**Quyết định:**
- Dùng chung **1 endpoint**: `POST /api/v1/flash-sale/orders`
- Chung **1 schema**, 2 bên validate như nhau
- Phân tách port: **`:8081`** cho Baseline, **`:8082`** cho Optimized

**Lý do:** Đảm bảo tính công bằng trong benchmark và k6 testing, cùng contract nghiệp vụ.

---

## D0.3 — Inventory State Model

**Vấn đề:** Baseline trừ `sold_quantity` ngay, Optimized reserve Redis nhưng sold_quantity chỉ tăng khi paid.

**Quyết định:**
- Tách rõ 3 state: **`available`**, **`reserved`**, **`paid`**
- Định nghĩa metric và DB projection riêng cho từng state
- Baseline giữ semantics direct DB, Optimized tách state rõ ràng
- Harness so sánh cùng business outcome

**Lý do:** Tránh nhầm lẫn trong reconciliation và công thức stock, đảm bảo metrics chính xác.

---

## D0.4 — Single Item per Flash Sale Order

**Vấn đề:** Multi-item order yêu cầu all-or-nothing nhưng Lua scripts chỉ mô tả một item.

**Quyết định:**
- Mỗi order flash sale chỉ được săn mua **đúng 1 sản phẩm duy nhất**
- Trong SDD ghi có thể mở rộng nhiều món trong tương lai, nhưng **nằm ngoài phạm vi hiện tại**

**Lý do:** Đơn giản hóa Lua transaction, tránh reserve dở dang và oversell, phù hợp scope demo.

---

## D0.5 — Reservation Cleanup

**Vấn đề:** `cleanup_expired.lua` parse member phức tạp, có thể cleanup sai key.

**Quyết định:**
- Chuẩn hóa và **confirm xóa reservation trước khi cleanup quét tới**
- Có thể làm **tối giản** vì hệ thống chủ yếu đánh vào khả năng chịu tải
- Reservation record phải chứa đủ: `reservation_id`, `item_id`, `user_id`, `qty`, `state`

**Lý do:** Đảm bảo cleanup an toàn, không double release hoặc leak stock.

---

## D0.6 — Payment Integration Strategy

**Vấn đề:** SePay docs nói HMAC/checksum nhưng chưa có payload/signature/config thực tế.

**Quyết định:**
- Triển khai **simulator trước**
- Sau đó sẽ xem xét tích hợp thật khi có tài liệu/config đầy đủ
- Adapter giữ verification provider-specific

**Lý do:** Tránh bịa contract, cho phép phát triển và testing độc lập với SePay thật.

---

## D0.7 — Late Payment Policy

**Vấn đề:** Late payment/refund_pending/webhook_log enum/schema không đầy đủ.

**Quyết định:**
- Khi đơn hàng đã **expired** mà người dùng mới thanh toán:
  - Giữ trạng thái đơn là **expired**
  - Xử lý như thường
  - Tạo thêm 1 record trong `webhook_logs` với trạng thái **`refund`**
  - Admin xử lý sau
- **Không chuyển thành paid** sau khi expired

**Lý do:** Đảm bảo state transition nhất quán, có audit trail, và tránh logic phức tạp về late payment.

---

## D0.8 — PostgreSQL Index

**Vấn đề:** Partial index `WHERE expires_at < now()` không hợp lệ PostgreSQL.

**Quyết định:**
- Dùng **index thường** trên `expires_at`
- Cleanup query có `WHERE` clause

**Lý do:** Tránh migration fail, index thường vẫn đủ hiệu quả cho scope hiện tại.

---

## D0.9 — Resource Budget và Test Mode

**Vấn đề:** Resource table và replica count khác nhau giữa các docs.

**Quyết định:**
- **Khóa budget RAM/CPU**
- **Chạy test tuần tự từng bên**
- Tách profiles: `dev`, `demo`, `benchmark`
- Khóa topology và budget trước run
- War Room hiển thị topology thật

**Lý do:** Đảm bảo tính công bằng trong benchmark, tránh tranh tài nguyên.

---

## D0.10 — Performance Metrics Labeling

**Vấn đề:** Docs nêu target RPS/latency cụ thể nhưng chưa có measurement.

**Quyết định:**
- Đổi label thành **hypothesis/acceptance candidate**
- UI/README chỉ hiển thị **measured artifact sau run**
- Không công bố target như kết quả

**Lý do:** Tránh biến hypothesis thành claim trước hội đồng, đảm bảo tính khoa học.

---

## D0.11 — Benchmark Execution Mode

**Vấn đề:** War Room đề xuất chạy k6 song song, fairness yêu cầu tuần tự.

**Quyết định:**
- **Chạy tuần tự** cho benchmark chính
- Demo visual có thể hiển thị hai panel từ cùng run
- Nếu chạy song song, đánh dấu là **demo không dùng để kết luận**

**Lý do:** Tránh tranh CPU làm sai kết quả benchmark.

---

## D0.12 — War Room Chaos Control

**Vấn đề:** War Room muốn gọi Docker socket trực tiếp từ browser.

**Quyết định:**
- **Bỏ tính năng kill từ UI**
- Chính operator sẽ **kill container trực tiếp** trong demo
- Browser gọi Go controller localhost-only/auth/allowlist cho các chức năng khác
- Socket chỉ mount vào controller profile demo nếu cần

**Lý do:** Giảm rủi ro bảo mật, đơn giản hóa demo flow, operator có kiểm soát trực tiếp.

---

## D0.13 — Redis Eviction Policy

**Vấn đề:** `allkeys-lru` policy có thể xóa inventory keys.

**Quyết định:**
- Cấu hình **`noeviction`**
- **Không eviction inventory keys**
- Alert memory nếu cần

**Lý do:** Đảm bảo inventory keys không bị mất, critical cho correctness.

---

## D0.14 — Frontend Realtime Strategy

**Vấn đề:** Frontend route và realtime contract chưa rõ.

**Quyết định:**
- Dùng Next **`[id]`** dynamic route
- Dùng **WebSocket cho War Room**
- Các trang người dùng dùng **polling đơn giản**

**Lý do:** War Room cần realtime cao, user pages có thể dùng polling đơn giản hơn để giảm complexity.

---

## Definition of Ready

**Tất cả D0.1–D0.14 đã được chốt.** Có thể triển khai stage 1–13 theo thứ tự dependency trong IMPLEMENTATION_PLAN.md.

---

## Áp dụng

- Mọi agent/developer phải tuân thủ các quyết định này
- Không được tự đoán semantics từ code cũ nếu contract đã chốt
- Thay đổi contract phải qua approval và cập nhật file này

---

**Ngày tạo:** 2026-10-23  
**Người maintain:** Tech Lead
