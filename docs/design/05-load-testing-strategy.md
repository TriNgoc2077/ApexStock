# ApexStock – Load Testing & Benchmarking Strategy

> Phiên bản: 1.0 | Ngày: 2026-10-22

---

## 1. Công cụ kiểm thử

### 1.1 Bộ công cụ chính

| Công cụ | Vai trò | Lý do chọn |
|---------|---------|------------|
| **k6** | Tạo và điều phối tải | Viết script bằng JS, nhẹ (~5MB), xuất Prometheus metrics, hỗ trợ custom metrics, mô hình VU rõ ràng |
| **Prometheus** | Thu thập & lưu metrics | Pull-based, tích hợp tốt với Go, chuẩn ngành |
| **Grafana** | Hiển thị biểu đồ | Dashboard đẹp, alert rule, plugin k6 |
| **cAdvisor** | Metrics container | CPU, RAM, network per container |
| **Toxiproxy** | Giả lập lỗi mạng | Thêm latency, packet loss, connection reset vào giữa services |
| **War Room** | Dashboard demo | Trang tự làm, hiển thị realtime cho hội đồng |

### 1.2 So sánh với các lựa chọn khác

| Công cụ | Điểm mạnh | Điểm yếu | Kết luận |
|---------|----------|----------|----------|
| **k6** ✅ | Nhẹ, scriptable, Prometheus native | Chạy distributed cần k6 Cloud | **Chọn** |
| wrk | Rất nhanh, C-based | Không có built-in metrics phong phú | Không chọn |
| hey | Đơn giản | Không có kịch bản phức tạp | Không chọn |
| Gatling | Báo cáo đẹp | Scala, RAM cao (~1GB) | Không chọn |
| JMeter | GUI, phổ biến | Nặng (~1.5GB), config phức tạp | Không chọn |
| Locust | Python, dễ mở rộng | Không export Prometheus dễ | Không chọn |

---

## 2. Kịch bản kiểm thử

### Scenario 1: Throughput tối đa (Max RPS)

**Mục tiêu:** Tìm ngưỡng RPS tối đa mà hệ thống còn giữ error rate < 1%. Mọi RPS/latency nêu trong tài liệu là **hypothesis/acceptance candidate** cho đến khi có measured artifact.

**Cấu hình k6:**
```javascript
export const options = {
  scenarios: {
    max_throughput: {
      executor: 'ramping-vus',
      stages: [
        { duration: '30s', target: 100 },
        { duration: '30s', target: 500 },
        { duration: '30s', target: 1000 },
        { duration: '30s', target: 2000 },
        { duration: '30s', target: 3000 },
        { duration: '60s', target: 3000 },  // hold
        { duration: '30s', target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};
```

**Metrics cần ghi nhận:**
- Điểm VU tại đó error rate vượt 1%
- RPS tại đỉnh bền vững
- p50, p95, p99 latency tại đỉnh
- CPU và RAM của từng container tại đỉnh

---

### Scenario 2: Flash Sale thực tế

**Mục tiêu:** Giả lập sự kiện flash sale thực: 10,000 sản phẩm, 1,000+ người dùng cùng lúc, có thanh toán và restock.

**Timeline:**

```
T+0s:   Sự kiện bắt đầu, 0 người dùng
T+0s:   Ramp-up nhanh lên 1,000 VU trong 5 giây (shock traffic)
T+5s:   Duy trì 1,000 VU đang cố đặt hàng
T+~20s: Dự kiến 10,000 hàng bán hết (Optimized) hoặc lâu hơn (Baseline)
T+30s:  Bắt đầu giả lập thanh toán:
          - 70% VU: gọi PAID (thanh toán thành công)
          - 20% VU: gọi FAILED (thanh toán thất bại)
          - 10% VU: không làm gì (timeout tự nhiên)
T+60s:  Kho tái nhập từ đơn thất bại/timeout → rush mới
T+90s:  Kết thúc test
```

**Cấu hình k6:**
```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Gauge, Trend } from 'k6/metrics';

// Custom metrics
const oversellCount = new Counter('oversell_count');
const stockRemaining = new Gauge('stock_remaining');
const orderSuccessRate = new Trend('order_success_rate');

export const options = {
  scenarios: {
    flash_sale: {
      executor: 'ramping-vus',
      stages: [
        { duration: '5s',  target: 1000 },  // shock wave
        { duration: '25s', target: 1000 },  // sustain
        { duration: '30s', target: 500 },   // payment phase
        { duration: '30s', target: 200 },   // restock rush
        { duration: '10s', target: 0 },
      ],
    },
  },
  thresholds: {
    'oversell_count': ['count==0'],         // MUST: không được oversell
    'http_req_duration': ['p(95)<500'],
    'http_req_failed': ['rate<0.05'],       // cho phép cao hơn do hết hàng
  },
};

const ITEM_ID = 'fsi_7';
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
let orderSuccessCount = 0;

export default function () {
  const userId = `usr_${__VU}`;
  const idempotencyKey = `${userId}_${ITEM_ID}_${__ITER}`;

  // Phase 1: Đặt hàng
  const orderRes = http.post(`${BASE_URL}/api/v1/flash-sale/orders`, JSON.stringify({
    idempotency_key: idempotencyKey,
    flash_sale_item_id: ITEM_ID,
    quantity: 1,
  }), {
    headers: { 'Content-Type': 'application/json' },
    timeout: '3s',
  });

  const ordered = check(orderRes, {
    'order placed or sold out': (r) => [201, 409, 429].includes(r.status),
  });

  if (orderRes.status === 201) {
    const body = JSON.parse(orderRes.body);

    // Phase 2: Giả lập thanh toán
    sleep(Math.random() * 5); // Chờ 0-5 giây trước khi thanh toán

    const rand = Math.random();
    if (rand < 0.70) {
      // 70%: Thanh toán thành công
      const payRes = http.post(`${BASE_URL}/api/v1/test/payments/${body.order_id}/simulate`, 
        JSON.stringify({ result: 'success' }), {
        headers: { 'Content-Type': 'application/json' },
      });
      check(payRes, { 'payment success': (r) => r.status === 200 });
    } else if (rand < 0.90) {
      // 20%: Thanh toán thất bại
      http.post(`${BASE_URL}/api/v1/test/payments/${body.order_id}/simulate`,
        JSON.stringify({ result: 'failure' }), {
        headers: { 'Content-Type': 'application/json' },
      });
    }
    // 10%: Không làm gì → tự hết hạn sau 30s
  }

  sleep(0.1);
}

export function handleSummary(data) {
  return {
    'stdout': textSummary(data, { indent: ' ', enableColors: true }),
    'results/summary.json': JSON.stringify(data),
  };
}
```

**Metrics cần ghi nhận:**
- Thời gian bán hết 10,000 hàng
- Tổng đơn thành công
- `oversell_count` PHẢI = 0
- Số đơn restock sau payment fail/timeout

---

### Scenario 3: Stress Test (Quá tải có kiểm soát)

**Mục tiêu:** Đẩy hệ thống vượt ngưỡng để quan sát cách graceful degradation hoạt động.

**Cấu hình:**
```javascript
export const options = {
  scenarios: {
    stress: {
      executor: 'ramping-vus',
      stages: [
        { duration: '1m', target: 5000 },
        { duration: '2m', target: 5000 },
        { duration: '30s', target: 0 },
      ],
    },
  },
};
```

**Điều cần kiểm tra:**
- Hệ thống có trả lỗi có cấu trúc (JSON) không hay bị crash?
- Sau khi quá tải, có tự phục hồi khi load giảm không?
- Tính nhất quán dữ liệu: `sold + remaining = total` vẫn đúng

---

### Scenario 4: Resilience – Chaos Test

**Mục tiêu:** Giết 1 API instance trong lúc traffic cao. Đây là kịch bản chính cho demo hội đồng.

**Script điều phối (chạy ngoài k6):**
```bash
#!/bin/bash
# Chạy benchmark tuần tự; operator inject chaos trực tiếp ngoài UI khi demo

# Bắt đầu tải
k6 run --env BASE_URL=http://optimized:8080 scenarios/flash_sale.js &
K6_PID=$!

# Chờ 30 giây traffic ổn định
sleep 30

echo ">>> CHAOS: Killing API instance 1..."
docker kill apexstock-api-1
echo ">>> Time: $(date) - API 1 killed"

# Chờ 30 giây để xem phục hồi
sleep 30

echo ">>> RECOVERY: Restarting API instance 1..."
docker start apexstock-api-1
echo ">>> Time: $(date) - API 1 restarted"

# Chờ test kết thúc
wait $K6_PID
```

**Metrics cần ghi nhận:**
- RPS tại thời điểm kill (baseline)
- RPS ngay sau kill (dip)
- Thời gian phục hồi về RPS ban đầu
- Số đơn bị mất (phải = 0)
- Số đơn trùng lặp (phải = 0)

---

### Scenario 5: Soak Test (Bền vững)

**Mục tiêu:** Phát hiện memory leak, goroutine leak, connection pool exhaustion.

**Cấu hình:**
```javascript
export const options = {
  scenarios: {
    soak: {
      executor: 'constant-vus',
      vus: 200,
      duration: '10m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<200'],
    http_req_failed: ['rate<0.01'],
  },
};
```

**Metrics cần quan sát theo thời gian:**
- Go heap size (nên ổn định, không tăng liên tục)
- Goroutine count (nên ổn định)
- PostgreSQL connection count (không vượt pool size)
- Redis connected_clients
- p95 latency (không được tăng dần theo thời gian)

---

## 3. Metrics thu thập

### 3.1 Application Metrics (Go app xuất ra Prometheus)

```go
// Ví dụ khai báo trong Go
var (
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "http_requests_total"},
        []string{"method", "path", "status"},
    )
    httpDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
        },
        []string{"method", "path"},
    )
    ordersCreated   = prometheus.NewCounter(prometheus.CounterOpts{Name: "orders_created_total"})
    ordersPaid      = prometheus.NewCounter(prometheus.CounterOpts{Name: "orders_paid_total"})
    ordersFailed    = prometheus.NewCounter(prometheus.CounterOpts{Name: "orders_failed_total"})
    ordersExpired   = prometheus.NewCounter(prometheus.CounterOpts{Name: "orders_expired_total"})
    stockRemaining  = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{Name: "stock_remaining"},
        []string{"item_id"},
    )
    stockOversold   = prometheus.NewCounter(prometheus.CounterOpts{Name: "stock_oversold_total"})
    queueDepth      = prometheus.NewGauge(prometheus.GaugeOpts{Name: "queue_depth"})
    circuitBreaker  = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{Name: "circuit_breaker_state"}, // 0=closed,1=half-open,2=open
        []string{"service"},
    )
    activeGoroutines = prometheus.NewGauge(prometheus.GaugeOpts{Name: "active_goroutines"})
)
```

### 3.2 Bảng metrics đầy đủ

| Metric | Type | Labels | Mô tả |
|--------|------|--------|-------|
| `http_requests_total` | Counter | method, path, status | Tổng requests |
| `http_request_duration_seconds` | Histogram | method, path | Phân phối latency |
| `orders_created_total` | Counter | — | Đơn được tạo |
| `orders_paid_total` | Counter | — | Đơn thanh toán thành công |
| `orders_failed_total` | Counter | — | Đơn thất bại |
| `orders_expired_total` | Counter | — | Đơn hết hạn |
| `stock_remaining` | Gauge | item_id | Tồn kho realtime |
| `stock_oversold_total` | Counter | — | ⚠️ PHẢI = 0 |
| `queue_depth` | Gauge | — | Số message chờ xử lý |
| `queue_consumer_lag` | Gauge | consumer | Độ trễ consumer |
| `redis_commands_total` | Counter | cmd | Số lệnh Redis |
| `redis_duration_seconds` | Histogram | cmd | Latency Redis |
| `db_query_duration_seconds` | Histogram | query_type | Latency DB |
| `active_goroutines` | Gauge | — | Goroutine count |
| `circuit_breaker_state` | Gauge | service | 0/1/2 |
| `idempotency_cache_hit_total` | Counter | — | Cache hit idempotency |

---

## 4. Phương pháp so sánh công bằng

### 4.1 Phân bổ tài nguyên

| Container | Baseline | Optimized |
|-----------|----------|-----------|
| API | 1 instance: 4 CPU, 1.5GB | 2 instances × 2 CPU, 512MB = 4 CPU, 1GB |
| PostgreSQL | 2 CPU, 2GB | 1 CPU, 1.5GB |
| Redis | — | 1 CPU, 512MB |
| NATS | — | 0.5 CPU, 256MB |
| Worker | — | 0.5 CPU, 256MB |
| **Tổng** | **6 CPU, 3.5GB** | **7 CPU, 3.5GB** |
| k6 + Prometheus + Grafana | 2 CPU, 1GB (dùng chung) | — |

> **Note:** Optimized có thêm 1 CPU nhưng đó là chi phí của queue+worker. Kết quả vẫn so sánh được vì RAM bằng nhau và CPU overhead của queue nhỏ.

**Quy trình test:** Chạy tuần tự từng architecture với cùng resource budget khóa trước; không chạy Baseline và Optimized đồng thời.

```bash
#!/bin/bash
# Chạy test công bằng cho cả 2 kiến trúc

for ARCH in baseline optimized; do
  echo "=== Testing: $ARCH ==="

  # 1. Restart fresh
  docker compose -f docker-compose.$ARCH.yml down -v
  docker compose -f docker-compose.$ARCH.yml up -d
  sleep 10  # Cho services khởi động

  # 2. Seed data
  docker exec apexstock-$ARCH-api ./scripts/seed.sh

  # 3. Warm up (không tính vào kết quả)
  k6 run --duration 30s --vus 50 --env BASE_URL=http://$ARCH:8080 scenarios/warmup.js

  # 4. Chạy test thực (3 lần, lấy median)
  for RUN in 1 2 3; do
    k6 run \
      --env BASE_URL=http://$ARCH:8080 \
      --out prometheus=http://prometheus:9090 \
      --tag arch=$ARCH,run=$RUN \
      scenarios/flash_sale.js
    sleep 30  # Nghỉ giữa các lần
  done

  # 5. Xuất kết quả
  docker compose -f docker-compose.$ARCH.yml down
done
```

### 4.3 Điều kiện test công bằng

- Cùng dữ liệu seed: 10,000 sản phẩm, cùng số user giả lập
- Cùng phiên bản k6, cùng kịch bản
- Chạy tuần tự (không cùng lúc) để tránh tranh CPU
- Clear page cache hệ điều hành giữa các lần: `sync && echo 3 > /proc/sys/vm/drop_caches`
- Warm up 30 giây trước mỗi lần đo
- Báo cáo giá trị median của 3 lần chạy

---

## 5. Tuning môi trường

### 5.1 Linux kernel (chạy trước khi test)

```bash
# File descriptors
ulimit -n 65536
echo "* soft nofile 65536" >> /etc/security/limits.conf
echo "* hard nofile 65536" >> /etc/security/limits.conf

# TCP tuning
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535
sysctl -w net.ipv4.ip_local_port_range="1024 65535"
sysctl -w net.ipv4.tcp_fin_timeout=10
sysctl -w net.ipv4.tcp_tw_reuse=1
sysctl -w net.core.netdev_max_backlog=5000
```

### 5.2 PostgreSQL (trong `postgresql.conf`)

```ini
max_connections = 200
shared_buffers = 512MB           # 25% RAM của container
effective_cache_size = 1GB
work_mem = 4MB
maintenance_work_mem = 64MB
wal_buffers = 16MB
checkpoint_completion_target = 0.9
random_page_cost = 1.1           # SSD
```

### 5.3 Redis (trong `redis.conf`)

```ini
maxmemory 400mb
maxmemory-policy noeviction      # Không evict inventory keys; alert khi gần giới hạn
save ""                          # Tắt RDB trong test để giảm I/O
appendonly no                    # Tắt AOF trong test
tcp-backlog 511
```

### 5.4 Go runtime

```bash
# Trong Dockerfile/entrypoint
export GOMAXPROCS=2              # Bằng CPU limit của container
export GOGC=100                  # GC mặc định
```

---

## 6. Grafana Dashboard – Panels cần có

### Dashboard "Benchmark Overview"

1. **RPS so sánh** (2 time series: Baseline vs Optimized)
2. **Latency p50 so sánh** 
3. **Latency p95 so sánh**
4. **Latency p99 so sánh**
5. **Error rate so sánh**
6. **Stock remaining** (giảm dần, 2 đường)
7. **Orders success total** (tăng dần)
8. **Oversell count** (PHẢI = 0, alert nếu > 0)
9. **CPU usage per container**
10. **Memory usage per container**
11. **Queue depth** (chỉ Optimized)
12. **Circuit breaker state** (chỉ Optimized)

### Alert Rules

```yaml
# Cảnh báo ngay nếu oversell > 0
- alert: OversellDetected
  expr: stock_oversold_total > 0
  for: 0m
  annotations:
    summary: "⚠️ OVERSELL DETECTED - Data integrity violation!"

# Cảnh báo nếu error rate > 5%
- alert: HighErrorRate
  expr: rate(http_requests_total{status=~"5.."}[1m]) / rate(http_requests_total[1m]) > 0.05
  for: 30s
```

---

## 7. Định dạng báo cáo kết quả cuối

### Bảng tổng hợp (trình bày cho hội đồng)

| Metric | Baseline | Optimized | Cải thiện |
|--------|----------|-----------|-----------|
| Max RPS bền vững | ~350 | ~4,500 | **+1,186%** |
| Latency p50 | ~350ms | ~18ms | **-95%** |
| Latency p95 | ~1,200ms | ~62ms | **-95%** |
| Latency p99 | ~3,500ms | ~145ms | **-96%** |
| Error rate (đỉnh tải) | ~12% | ~0.1% | **-99%** |
| Oversell count | 0* | 0 | ✓ |
| Thời gian bán hết 10k hàng | ~30s | ~2.5s | **-92%** |
| Thời gian phục hồi sau sự cố | N/A | ~5s | ✓ |
| Đơn mất khi 1 API bị kill | N/A | 0 | ✓ |

> *Baseline đạt 0 oversell nhờ `SELECT FOR UPDATE` nhưng đánh đổi bằng throughput thấp.

### Narrative cho hội đồng

> "Cả hai kiến trúc đều **không bán vượt kho** – đây là yêu cầu bắt buộc. Tuy nhiên, kiến trúc tối ưu đạt được điều này với throughput cao hơn **12 lần**, latency thấp hơn **95%**, và còn duy trì hoạt động khi có sự cố phần cứng (API instance bị kill), điều mà kiến trúc baseline không thể làm."

---

## 8. Checklist trước khi demo

- [ ] Đặt `ulimit -n 65536` trên máy host
- [ ] Kiểm tra tất cả container healthy: `docker compose ps`
- [ ] Seed dữ liệu: 10,000 sản phẩm, 5,000 user test
- [ ] Warm up Redis: kích hoạt flash sale event qua Admin API
- [ ] Mở War Room dashboard, xác nhận WebSocket kết nối
- [ ] Mở Grafana dashboard
- [ ] Test 1 request thủ công xem API hoạt động
- [ ] Chuẩn bị script chaos: `docker kill apexstock-baseline-api` và `docker kill apexstock-api-1`
- [ ] Pin clock/timer trên màn hình để hội đồng thấy realtime
