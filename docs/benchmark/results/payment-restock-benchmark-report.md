# 🔄 Báo Cáo Kiểm Thử Thanh Toán & Hoàn Kho: Payment Restock (Consistency & Compensation)

**Thời gian thực nghiệm:** 2026-10-31  
**Kịch bản:** `payment-restock.js`  
**Cấu hình tải:** 50 Virtual Users (VUs) gửi đơn hàng liên tục trong 60 giây.  
**Cơ chế thử nghiệm (Compensation Simulation):**
1. Gửi yêu cầu đặt hàng `POST /api/v1/flash-sale/orders` với số lượng 1.
2. Nếu đặt hàng thành công (`201 Created`), client tiếp tục gọi ngay `POST /api/v1/payments/simulate`:
   * **50% tỷ lệ:** Giả lập thanh toán thành công (`outcome: 'success'`).
   * **50% tỷ lệ:** Giả lập người dùng hủy đơn hoặc hết hạn thanh toán (`outcome: 'cancel'`).
3. Khi đơn bị hủy (`cancel`), hệ thống bắt buộc phải kích hoạt cơ chế hoàn kho tự động (Restock), bù trừ số lượng hàng lại vào kho và không để xảy ra sai lệch số lượng.
**Thư mục dữ liệu đối chứng:** `loadtests/results/run-20261031-145535/`  

---

## 📊 1. Bảng Đối So Sánh Trực Tiếp (Head-to-Head Comparison)

| Tiêu chí đo lường | Baseline API (PostgreSQL `FOR UPDATE`) | Optimized API (Redis Lua + NATS + Worker) | Đánh giá & Phân tích |
|---|---|---|---|
| **Độ Trễ Đặt Hàng Trung Bình (Order Latency Avg)** | **63.64 ms** | **1.55 ms** | **Optimized nhanh hơn 41.0 lần** 🚀 |
| **Độ Trễ Đặt Hàng Trung Vị (Order Latency P50)** | **59.52 ms** | **0.75 ms (750 µs)** | **Optimized nhanh hơn 79.3 lần** 🚀 |
| **Độ Trễ Đặt Hàng P95 (Order Latency P95)** | **133.12 ms** | **3.66 ms** | **Optimized nhanh hơn 36.4 lần** 🚀 |
| **Độ Trễ Đặt Hàng Cực Đại (Order Max Latency)** | **309.60 ms** | **39.50 ms** | **Optimized ổn định hơn gấp 7.8 lần** |
| **Tỷ Lệ Lỗi Hệ Thống (HTTP 5xx)** | **0.00%** | **0.00%** | Chuẩn SLA tuyệt đối |
| **Tỷ Lệ Giữ Kho Thành Công** | 100% trong số đơn hợp lệ | 100% trong số đơn hợp lệ | Cả hai xử lý đúng logic |
| **Kiểm Tra Phương Trình Tồn Kho (`Stock Equation`)** | **PASS (0 oversold)** | **PASS (0 oversold)** | **Toàn vẹn kho hàng tuyệt đối** |

---

## 🔬 2. Phân Tích Cơ Chế Bù Trừ & Hoàn Kho (Saga / Compensation Pattern)

### 1. Luồng xử lý hoàn kho của Baseline:
* Khi nhận request cancel thanh toán, Baseline thực thi cập nhật trực tiếp trong transaction PostgreSQL:
  ```sql
  UPDATE flash_sale_items SET stock = stock + 1 WHERE id = $1;
  UPDATE orders SET status = 'CANCELLED' WHERE id = $2;
  ```
* Vì mọi thao tác đều ghi đĩa đồng bộ (Synchronous Transaction), độ trễ trung bình của chu trình đặt hàng và thanh toán duy trì ở mức **~63.6 ms**.

### 2. Luồng xử lý hoàn kho của Optimized (Event-Driven Restock):
* Khi hủy đơn, API bắn event `OrderCancelledEvent` vào **NATS JetStream**.
* Worker chạy ngầm (Asynchronous Background Consumer) nhận event:
  1. Gọi Lua script hoàn trả số lượng vào Redis: `INCRBY flash_item:{id}:stock 1` và xóa reservation key.
  2. Ghi nhận trạng thái `CANCELLED` vào PostgreSQL.
* Nhờ cơ chế tách rời bất đồng bộ (Decoupling):
  * Người dùng đặt hàng nhận kết quả chỉ trong **1.55 ms** (nhanh gấp 41 lần).
  * 50% số đơn bị hủy được worker hoàn kho tự động và chính xác trong nền.

### 3. Kiểm chứng phương trình toàn vẹn tồn kho (`Stock Equation`):
Ngay sau khi kết thúc bài test, công cụ kiểm chứng độc lập [TestInvariantVerifier_StockEquation_Calculation](file:///home/trisngocj/Projects/Báo%20cáo%20thực%20tập/ApexStock/test/system) được thực thi và trả về **PASS**:
$$\text{Tồn kho khởi tạo (Initial Stock)} = \text{Tồn kho thực tế (Current Stock)} + \text{Số đơn thành công (Paid Orders)} + \text{Đơn đang giữ hàng (Pending)}$$
$$\text{Số lượng đơn đã Cancel} \times 1 \text{ đã được hoàn trả chính xác 100\% vào kho.}$$

---

## 📁 3. Tệp Dữ Liệu Đối Chứng Thực Tế
* Dữ liệu Baseline thô: `loadtests/results/run-20261031-145535/baseline_run1_summary.json`
* Dữ liệu Optimized thô: `loadtests/results/run-20261031-145535/optimized_run1_summary.json`
