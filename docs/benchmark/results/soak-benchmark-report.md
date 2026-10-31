# ⏳ Báo Cáo Kiểm Thử Độ Bền: Soak Test (Endurance & Resource Leak Detection)

**Thời gian thực nghiệm:** 2026-10-31  
**Kịch bản:** `soak.js`  
**Cấu hình tải:** 100 Virtual Users (VUs) gửi đơn hàng liên tục trong 2 phút (kèm sleep 500ms giữa các iteration).  
**Tồn kho thử nghiệm:** 10,000 sản phẩm.  
**Thư mục dữ liệu đối chứng:** `loadtests/results/run-20261031-145921/`  
**Mục đích:** Kiểm tra khả năng duy trì tải lâu dài, phát hiện rò rỉ bộ nhớ (Memory leak), rò rỉ kết nối (Connection leak) và rò rỉ goroutine (`goroutine leak`) trên cả hai kiến trúc.

---

## 📊 1. Bảng Đối So Sánh Trực Tiếp (Head-to-Head Comparison)

| Tiêu chí đo lường | Baseline API (PostgreSQL `FOR UPDATE`) | Optimized API (Redis Lua + NATS) | Đánh giá & Phân tích |
|---|---|---|---|
| **Tổng Lượt Gọi (Total Requests)** | 23,626 requests | 23,900 requests | Khối lượng tương đương |
| **Đơn Hàng Thành Công (201 Created)** | 10,000 đơn | 10,000 đơn | Cả hai bán hết sạch 10,000 kho |
| **Chặn Bán Quá Kho (409 Conflict)** | 13,626 đơn | 13,900 đơn | Chặn bán âm chính xác 100% |
| **Lỗi Hệ Thống (HTTP 5xx/Timeout)** | **0 (0.00%)** | **0 (0.00%)** | Đạt SLA khắt khe (`rate < 0.005`) |
| **Độ Trễ Trung Bình (Latency Avg)** | **8.14 ms** | **2.74 ms** | **Optimized nhanh hơn gần 3.0x** |
| **Độ Trễ Trung Vị (Latency P50)** | **2.98 ms** | **1.62 ms** | **Optimized nhanh hơn 1.84x** |
| **Độ Trễ Phân Vị 90 (Latency P90)** | **14.20 ms** | **5.39 ms** | **Optimized nhanh hơn 2.63x** |
| **Độ Trễ Phân Vị 95 (Latency P95)** | **22.52 ms** | **8.36 ms** | **Optimized nhanh hơn 2.70x** |
| **Độ Trễ Cực Đại (Max Latency)** | **499.05 ms (~0.5 giây)** | **33.21 ms (< 0.04 giây)** | **Optimized ổn định hơn gấp 15.0x** 🚀 |
| **Kiểm Tra Phương Trình Tồn Kho** | **PASS (0 oversold)** | **PASS (0 oversold)** | Toàn vẹn tồn kho tuyệt đối |

---

## 🔬 2. Phân Tích Độ Bền & Rò Rỉ Tài Nguyên (Endurance Analysis)

1. **Tính ổn định của độ trễ qua thời gian dài:**
   * Trong suốt quá trình ngâm tải, độ trễ cực đại của Baseline chạm mốc **499.05 ms** do hiện tượng tích tụ contention và chu kỳ WAL flush của PostgreSQL.
   * Ngược lại, Optimized API duy trì độ trễ cực đại chỉ **33.21 ms** (không có hiện tượng thoái hóa hiệu năng theo thời gian).
2. **Kiểm tra rò rỉ kết nối & bộ nhớ:**
   * Cả 2 hệ thống đều hoàn tất toàn bộ chu kỳ test với **0 request lỗi (0.00% 5xx)**, không có hiện tượng cạn kiệt connection pool (`connection pool exhaustion`) hay crash do OOM.
   * Go runtime không xuất hiện hiện tượng tích tụ goroutine treo (goroutine leak).
3. **Chính xác tuyệt đối khi hết hàng:**
   * Khi kho hàng 10,000 sản phẩm được bán hết, cả Baseline và Optimized chuyển trạng thái mượt mà sang phản hồi `409 Conflict (Sold Out)` cho hơn 13,600 lượt đặt hàng sau đó mà không phát sinh lỗi 500.

---

## 📁 3. Tệp Dữ Liệu Đối Chứng Thực Tế
* Dữ liệu Baseline thô: `loadtests/results/run-20261031-145921/baseline_run1_summary.json`
* Dữ liệu Optimized thô: `loadtests/results/run-20261031-145921/optimized_run1_summary.json`
