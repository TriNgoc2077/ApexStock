# ⚡ ApexStock — High-Concurrency Flash Sale Benchmark Platform

> **Báo Cáo Thực Tập Tốt Nghiệp**  
> So sánh thực nghiệm hiệu năng giữa Kiến Trúc Truyền Thống (**Baseline**: PostgreSQL Row Locking) và Kiến Trúc Tối Ưu (**Optimized**: Redis Lua Atomic Reservation + NATS JetStream Durable Queue) dưới tải cao và kịch bản Chaos Engineering.

---

## 🚀 Quick Start (Khởi Động Nhanh Trong 1 Phút)

### 1. Khởi động toàn bộ hệ thống
```bash
# Khởi động cơ sở hạ tầng, 2 phiên bản API, Worker, War Room và Web UI
docker compose -f deploy/compose/docker-compose.yaml --profile demo up -d
```

### 2. Khởi tạo dữ liệu mẫu (Seed & Warm-up)
```bash
./scripts/reset-data.sh && ./scripts/seed-data.sh && ./scripts/warmup-redis.sh
```

### 3. Kiểm tra tự động toàn hệ thống (Rehearsal)
```bash
./scripts/demo/rehearsal.sh
```

---

## 🌐 Các Cổng Truy Cập Dịch Vụ

| Dịch Vụ | URL | Mục Đích |
| :--- | :--- | :--- |
| **Web UI (Next.js)** | `http://localhost:3000` | Giao diện mua hàng Flash Sale, quét mã VietQR |
| **War Room Dashboard** | `http://localhost:3000/war-room` | Trung tâm điều khiển so sánh RPS, Latency, Invariant real-time |
| **War Room Controller** | `http://localhost:9000` | API & SSE Stream tổng hợp metrics |
| **Optimized API (NGINX)** | `http://localhost:8082` | Cổng phân tán tải (Redis Lua + NATS) |
| **Baseline API** | `http://localhost:8081` | Cổng truyền thống (PostgreSQL Row Lock) |
| **Prometheus** | `http://localhost:9090` | Thu thập metrics và cảnh báo vi phạm |
| **Grafana** | `http://localhost:3001` | Dashboard trực quan hóa hiệu năng |

---

## 🛡️ Chứng Nhận Invariant & Không Bị Oversell

Hệ thống cam kết 5 Invariant bất biến toán học:
- **I1:** `Kho khả dụng + Đơn đang giữ + Đơn đã thanh toán == Tổng số lượng ban đầu` (Tuyệt đối không oversell).
- **I2:** Mọi yêu cầu giữ hàng đều kết thúc ở trạng thái `confirmed` hoặc `released`.
- **I3:** Mỗi `idempotency_key` tạo ra tối đa đúng một đơn hàng.
- **I4:** Trạng thái đơn hàng và thanh toán tuân thủ nghiêm ngặt State Machine.
- **I5:** Worker xử lý mỗi sự kiện JetStream tối đa một lần (`processed_events`).

Kiểm tra Invariant bất kỳ lúc nào bằng lệnh:
```bash
./scripts/reconcile.sql.sh
```

---

## 📊 Kịch Bản Chạy Benchmark (k6)

Chạy benchmark so sánh tuần tự công bằng (Tuân thủ Decision Gate D0.11):
```bash
# Đo đột biến tranh chấp kho (Flash-sale Shock Contention)
./scripts/benchmark/run-benchmark.sh flash-sale-shock 3

# Đo thông lượng tối đa bền vững (Plateau Stepped RPS)
./scripts/benchmark/run-benchmark.sh sustainable-throughput 3
```

---

## 💥 Thao Tác Kịch Bản Chaos (Dành Cho Operator)

Tuân thủ Decision Gate D0.12 (không có nút bấm nguy hiểm trên Web):
```bash
# Mô phỏng sập 1 instance API Optimized
./scripts/chaos/kill-optimized-api.sh

# Phục hồi instance và quan sát thời gian tự phục hồi trên War Room
./scripts/chaos/restore-optimized-api.sh
```
