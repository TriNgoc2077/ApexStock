# 💨 Báo Cáo Kiểm Thử Khởi Động: Smoke Test (Contract & Health Parity)

**Thời gian thực nghiệm:** 2026-10-31  
**Kịch bản:** `smoke.js`  
**Cấu hình tải:** 10 Virtual Users (VUs) chạy liên tục trong 30 giây (kèm sleep 100ms giữa các iteration).  
**Thư mục dữ liệu đối chứng:** `loadtests/results/run-20261031-145107/`  
**Mục đích:** Xác minh tính sẵn sàng của toàn bộ chu trình đặt hàng, contract API, tính toàn vẹn tồn kho cơ bản và độ ổn định của cả 2 kiến trúc (Baseline vs Optimized) trước khi tiến hành các bài đo tải cực hạn.

---

## 📊 1. Bảng Đối So Sánh Trực Tiếp (Head-to-Head Comparison)

| Chỉ số đo lường | Baseline API (PostgreSQL `FOR UPDATE`) | Optimized API (Redis Lua + NATS) | Đánh giá & Cải thiện |
|---|---|---|---|
| **Tổng Lượt Gọi (Total Requests)** | 2,800 requests | 2,880 requests | Tương đương khối lượng |
| **Số Đơn Thành Công (HTTP 201)** | 2,800 (100.00%) | 2,880 (100.00%) | Cả hai đều đạt 100% |
| **Lỗi Hệ Thống (HTTP 5xx/Timeout)** | 0 (0.00%) | 0 (0.00%) | Đạt chuẩn SLA (`rate < 0.01`) |
| **Thông Lượng (Throughput)** | 93.06 req/s | 95.89 req/s | Tương đương do bị giới hạn bởi sleep 100ms |
| **Độ Trễ Trung Bình (Latency Avg)** | **6.76 ms** | **3.38 ms** | **Optimized nhanh hơn 2.0x** |
| **Độ Trễ Trung Vị (Latency P50)** | **6.60 ms** | **3.40 ms** | **Optimized nhanh hơn 1.94x** |
| **Độ Trễ Phân Vị 90 (Latency P90)** | **9.06 ms** | **4.87 ms** | **Optimized nhanh hơn 1.86x** |
| **Độ Trễ Phân Vị 95 (Latency P95)** | **10.38 ms** | **5.41 ms** | **Optimized nhanh hơn 1.92x** |
| **Độ Trễ Tối Đa (Max Latency)** | **63.72 ms** | **20.85 ms** | **Optimized ổn định hơn gấp 3 lần** |
| **Kiểm tra Phương trình Tồn kho** | **PASS (0 oversold)** | **PASS (0 oversold)** | Toàn vẹn dữ liệu tuyệt đối |

---

## 🔬 2. Phân Tích Kỹ Thuật

1. **Hiệu năng ở tải thấp (Low Load Latency Baseline):**
   * Ngay cả ở mức tải rất nhẹ (10 người dùng ảo), Optimized API với Redis Lua Script In-Memory vẫn cho độ trễ trung bình chỉ **3.38 ms**, giảm một nửa so với Baseline (**6.76 ms**) khi phải mở transaction và lock row trong PostgreSQL.
2. **Độ ổn định đường truyền (Jitter / Latency Spike):**
   * Độ trễ cực đại của Baseline ghi nhận **63.72 ms** do chu kỳ checkpoint và lock acquisition của PostgreSQL.
   * Optimized API duy trì độ trễ cực đại dưới **20.85 ms** nhờ Redis phản hồi microsecond và NATS JetStream bất đồng bộ hóa ghi đĩa.
3. **Tính sẵn sàng cho Demo:**
   * Kịch bản này được tích hợp trong `./scripts/demo/rehearsal.sh`. Kết quả 100% thành công chứng minh toàn bộ các thành phần (Nginx, Go API, Worker, Redis, NATS, PostgreSQL) đã sẵn sàng phục vụ biểu diễn trước Hội đồng.

---

## 📁 3. Tệp Dữ Liệu Đối Chứng Thực Tế
* Dữ liệu Baseline thô: `loadtests/results/run-20261031-145107/baseline_run1_summary.json`
* Dữ liệu Optimized thô: `loadtests/results/run-20261031-145107/optimized_run1_summary.json`
