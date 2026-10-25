# ApexStock Complete Context Handoff Document

## 1. Decision Gates Summary (D0.1 – D0.15)

| Gate | Title | Decision & Architectural Rationale |
| :--- | :--- | :--- |
| **D0.1** | Compose Topology | Khóa cứng ports 8081 (Baseline), 8082 (Optimized), 9000 (War Room), 3000 (Web). |
| **D0.2** | Event Payload | Gói event JetStream mang đủ thông tin đơn hàng; worker không query ngược Redis. |
| **D0.3** | Stock Increment Timing | Baseline tăng `sold_quantity` khi reserve; Optimized chỉ tăng DB khi `paid`. |
| **D0.4** | Single Item Constraint | Mỗi flash sale order bắt buộc đúng 1 sản phẩm (`quantity = 1`). |
| **D0.5** | Error Envelope Schema | Thống nhất chuẩn JSON `{error, message, correlation_id, details}` trên toàn hệ thống. |
| **D0.6** | Payment Isolation | K6 benchmark chỉ gọi Simulator; SePay adapter chỉ phục vụ UI demo bằng tay. |
| **D0.7** | Late Payment Policy | Đơn đã expired mà tiền về: giữ nguyên `expired`, ghi `webhook_logs` với `status='refund'`. |
| **D0.8** | Database Indexing | Tránh dùng non-deterministic `now()` trong index predicates. |
| **D0.9** | Metrics Cardinality | Bounded labels (arch, route, status); tuyệt đối không đưa user_id/order_id vào Prometheus. |
| **D0.10** | Resource Parity | Cấp phát tài nguyên tương đương (2.0 CPU, 1024MB RAM) giữa Baseline và Optimized. |
| **D0.11** | Sequential Execution | Benchmark chính thức bắt buộc chạy tuần tự từng kiến trúc để tránh tranh chấp CPU. |
| **D0.12** | War Room Chaos Boundary | Bỏ nút kill trên UI; operator kill container trực tiếp bằng Docker CLI. |
| **D0.13** | Redis Eviction Policy | Cấu hình `noeviction` để bảo vệ tính toàn vẹn của tồn kho flash sale. |
| **D0.14** | Frontend Realtime | War Room dùng WebSocket/SSE; trang người dùng dùng polling đơn giản. |
| **D0.15** | Data Isolation | Tách riêng hai database `apexstock_baseline` và `apexstock_optimized`. |

## 2. Verification Checklist For Maintainers

- [ ] `go build ./... && go vet ./...` (Exit code 0)
- [ ] `go test -race ./...` (All unit, contract, and integration tests pass)
- [ ] `./scripts/demo/rehearsal.sh` (Full rehearsal passes)
- [ ] `./scripts/reconcile.sql.sh` (Zero invariant discrepancies detected)
- [ ] `cd web && npm run lint && npm run build` (Next.js production build passes)

## 3. Repository Cleanliness
- Working tree is clean.
- All temporary benchmark logs reside in `loadtests/results/` and are tracked with `.gitkeep`.
