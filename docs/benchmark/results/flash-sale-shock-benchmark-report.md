# ⚡ Báo Cáo Kiểm Thử Sốc Tải Tức Thời: Flash Sale Shock (Hot-Spot Contention)

**Thời gian thực nghiệm:** 2026-10-31  
**Kịch bản:** `flash-sale-shock.js`  
**Cấu hình tải:** 500 Virtual Users (VUs) đồng loạt ập vào bấm mua cùng lúc (không làm nóng), mỗi VU thực hiện 20 lần đặt hàng (Tổng cộng 10,000 requests).  
**Sản phẩm thử nghiệm:** 1 Hot-spot Item duy nhất (`22222222-2222-2222-2222-222222222222`).  
**Tồn kho thử nghiệm:** 10,000 sản phẩm; Giới hạn mỗi người dùng: 5 sản phẩm.  
**Thư mục dữ liệu đối chứng:** `loadtests/results/run-20261031-145339/`  

---

## 📊 1. Bảng Đối So Sánh Trực Tiếp (Head-to-Head Comparison)

| Tiêu chí đo lường | Baseline API (PostgreSQL `FOR UPDATE`) | Optimized API (Redis Lua + NATS) | Tỷ lệ cải thiện (Speedup) |
|---|---|---|---|
| **Tổng Lượt Yêu Cầu (Total Requests)** | 10,000 requests | 10,000 requests | Khối lượng ngang bằng tuyệt đối |
| **Thời Gian Xử Lý Hết 10,000 Đơn** | **27.5 giây** | **0.8 giây** | **Optimized nhanh hơn 34.4 lần** 🚀 |
| **Thông Lượng (Throughput)** | **363.9 requests/giây** | **12,755.7 requests/giây** | **Optimized gấp 35.0 lần** 🚀 |
| **Độ Trễ Trung Bình (Latency Avg)** | **1,293.9 ms (1.29 giây)** | **30.64 ms** | **Nhanh hơn 42.2 lần** |
| **Độ Trễ Trung Vị (Latency P50)** | **732.67 ms (0.73 giây)** | **20.92 ms** | **Nhanh hơn 35.0 lần** |
| **Độ Trễ Phân Vị 90 (Latency P90)** | **3,014.7 ms (3.01 giây)** | **43.96 ms** | **Nhanh hơn 68.6 lần** |
| **Độ Trễ Phân Vị 95 (Latency P95)** | **5,593.3 ms (5.59 giây)** | **74.54 ms (< 0.08 giây)** | **Nhanh hơn 75.0 lần** 🚀 |
| **Độ Trễ Cực Đại (Max Latency)** | **12,116.8 ms (12.1 giây)** | **488.21 ms (< 0.5 giây)** | **Nhanh hơn 24.8 lần** |
| **Đơn Hàng Thành Công (HTTP 201)** | 5,000 đơn (50.00%) | 5,000 đơn (50.00%) | Đạt tối đa theo hạn mức user |
| **Từ Chối Nghiệp Vụ (HTTP 409)** | 5,000 đơn (50.00%) | 5,000 đơn (50.00%) | Chặn vượt hạn mức chính xác 100% |
| **Lỗi Hệ Thống (HTTP 5xx/Timeout)** | **0 (0.00%)** | **0 (0.00%)** | Cả hai không rớt kết nối |
| **Kiểm Tra Toàn Vẹn Tồn Kho** | **PASS (0 oversold)** | **PASS (0 oversold)** | Bảo toàn tồn kho tuyệt đối |

---

## 🔬 2. Phân Tích Hiện Tượng Kỹ Thuật

Kịch bản **Flash Sale Shock** mô phỏng chính xác khoảnh khắc đồng hồ vừa điểm 00:00:00 của đợt mở bán: hàng trăm người dùng nhấn nút *"Mua ngay"* trong cùng một phần nghìn giây.

### Điểm nghẽn nghiêm trọng ở Baseline:
1. Khi 500 kết nối đồng thời gửi `SELECT ... FOR UPDATE` lên cùng 1 dòng trong bảng `flash_sale_items`, PostgreSQL buộc phải cấp khóa độc quyền hàng (exclusive row lock).
2. Hàng trăm transaction bị xếp hàng dài chờ đợi khóa giải phóng (Lock Wait Queue). 
3. Hậu quả: Độ trễ P95 vọt lên **5.59 giây**, độ trễ cực đại lên tới **12.1 giây**. Người mua phải chờ đợi màn hình loading hơn 10 giây để biết mình có mua được hàng hay không.
4. Toàn bộ 10,000 đơn hàng mất tới **27.5 giây** mới hoàn tất.

### Sức mạnh bứt phá của Optimized:
1. **Khử hoàn toàn hàng đợi khóa trên RDBMS**: Lua script của Redis thực thi nguyên tử trong RAM chỉ mất microsecond.
2. **Xử lý siêu tốc**: Hệ thống giải quyết sạch sẽ 10,000 đơn hàng dồn dập chỉ trong **0.8 giây**!
3. **Trải nghiệm người dùng vượt bậc**: Độ trễ P95 chỉ **74.5 ms** (nhanh hơn 75 lần so với Baseline). 95% khách hàng nhận kết quả đặt hàng ngay tức thì dưới 0.1 giây.
4. **Kiểm soát hạn mức mua hoàn hảo**: Cả 2 hệ thống đều từ chối chính xác 5,000 đơn vượt hạn mức (`max_per_user = 5`) mà không làm sai lệch tồn kho.

---

## 📁 3. Tệp Dữ Liệu Đối Chứng Thực Tế
* Dữ liệu Baseline thô: `loadtests/results/run-20261031-145339/baseline_run1_summary.json`
* Dữ liệu Optimized thô: `loadtests/results/run-20261031-145339/optimized_run1_summary.json`
