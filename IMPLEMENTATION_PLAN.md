# ApexStock — Implementation Plan

> **Ngôn ngữ:** Tiếng Việt  
> **Trạng thái:** Kế hoạch triển khai, chưa phải kết quả benchmark  
> **Phạm vi:** Backend Go, Baseline, Optimized, PostgreSQL, Redis/Lua, NATS JetStream, frontend Next.js, payment SePay, observability, War Room, QA, load/concurrency/chaos/system testing  
> **Môi trường mục tiêu:** Một laptop Intel i5-10500H, 6C/12T, RAM 16GB, NVMe, Docker Compose  
> **Nguyên tắc quan trọng:** Không công bố RPS, latency hay recovery time như kết quả thật trước khi benchmark được chạy và lưu raw result.

---

## 1. Cách đọc và phạm vi của kế hoạch

Repository hiện mới có tài liệu thiết kế và hồ sơ agent; chưa có `README`, mã nguồn, `go.mod`, `package.json`, Docker Compose hay cấu hình runtime. Vì vậy các đường dẫn trong kế hoạch là **đường dẫn dự kiến** cho implementation, không phải khẳng định rằng file đã tồn tại.

Kế hoạch này giữ các quyết định đã nêu trong SDD/spec, nhưng không âm thầm giải quyết các điểm mâu thuẫn. Các điểm có thể làm thay đổi contract hoặc tính đúng đắn được gom vào **Decision Gate D0** và các decision bổ sung trong mục 4. Mọi stage có thể bắt đầu sau D0 chỉ khi contract liên quan đã được ghi lại trong `docs/decisions/` hoặc phần quyết định của SDD.

Mục tiêu triển khai là một **vertical slice có thể chạy được sớm**, theo thứ tự:

```text
health → migration/seed → Baseline order → Optimized reservation
→ payment simulator/restock → metrics → frontend → War Room
→ contract/concurrency/system tests → benchmark → chaos/soak/demo
```

Không triển khai distributed production thật. Nhiều process/container chỉ dùng để mô phỏng các vai trò cần so sánh trên cùng một host; NATS, Redis, PostgreSQL và mọi replica đều là single-node theo giới hạn phần cứng hiện tại.

---

[Phần còn lại quá dài, đã lưu vào file]

## 2. Mục tiêu, non-goals và tiêu chí thành công

### 2.1. Mục tiêu triển khai

1. Chạy được Baseline và Optimized trên cùng host bằng Docker Compose.
2. Dùng cùng workload, dataset, thời gian chạy, giới hạn tài nguyên và k6 harness để so sánh.
3. Có order flow từ request đến trạng thái thanh toán, hết hạn, thất bại và hoàn kho.
4. Giữ các invariant cứng:
   - Không oversell.
   - `reserve`, `release`, `confirm` idempotent.
   - Không tạo order trùng khi retry/cùng idempotency key.
   - Chỉ cho phép các payment/order transition hợp lệ.
   - Message redelivery không làm trừ/hoàn kho hai lần.
5. Có metrics có provenance, log có correlation ID, dashboard hiển thị trạng thái stale/mất kết nối thay vì số liệu giả.
6. Có test chứng minh correctness trước khi chạy tải; benchmark chỉ được xem là hợp lệ khi invariant và reconciliation pass.
7. Có UI trình bày theo design đã chốt: nền tối giản kiểu A, aurora hero kiểu M4, spotlight/spinning border/gradient fill kiểu M3, dark/light theme.
8. Có kịch bản War Room và chaos an toàn, mặc định ưu tiên kill một API Optimized thay vì phá Redis/PostgreSQL.

### 2.2. Non-goals

- Không xây shipping, delivery, review, mobile app, multi-tenant.
- Không dựng Kubernetes, Redis Cluster, PostgreSQL replication, multi-host hay cloud production.
- Không gọi SePay thật trong k6; load test dùng simulator nội bộ.
- Không dùng số RPS/latency dự kiến trong docs làm acceptance result.
- Không coi việc UI đẹp là bằng chứng hệ thống chịu tải.

### 2.3. Tiêu chí thành công cấp dự án

| Nhóm | Điều kiện bắt buộc |
|---|---|
| Khởi động | Compose khởi động được, health/readiness phân biệt đúng, migration/seed lặp lại được |
| Baseline | `SELECT ... FOR UPDATE` nằm trong transaction; không dùng Redis/queue để làm sai baseline; order thành công và 409 hết hàng đúng |
| Optimized | Reserve atomic bằng Lua; publish/ghi order theo contract đã chốt; worker redelivery không duplicate side effect |
| Inventory | Không stock âm/oversell; release/confirm lặp lại không đổi kết quả; reconciliation cuối run pass |
| Payment | Simulator và SePay adapter dùng chung state machine; duplicate/out-of-order/late/mismatch được xử lý có audit |
| Observability | Metrics phân biệt 2xx/409/429/5xx, có p50/p95/p99, queue lag, stock và invariant counters |
| Frontend | Không hardcode benchmark; có stale/error/loading/reconnect; reduced-motion và keyboard flow hoạt động |
| War Room | Có snapshot/live stream, start/stop, health, activity log, summary; chaos không gọi Docker trực tiếp từ browser |
| Benchmark | Hai kiến trúc chạy tuần tự với parity; có raw summary, metadata môi trường, median/variance; không claim nếu invariant fail |
| Reliability | API kill có cleanup/recovery; Redis/DB/queue failure có runbook và test riêng trong môi trường local |


---

## 3. Nguyên tắc kỹ thuật không được phá vỡ

### 3.1. Invariant tồn kho và order/payment

Mọi stage chạm inventory phải kiểm tra các invariant sau, cả ở unit test, integration test, concurrency test và post-run reconciliation.

**Invariant I1 — tồn kho không âm/không oversell**

```text
stock_available >= 0
successful_paid_quantity + active_reserved_quantity + stock_available
    = total_quantity
```

Biểu thức trên cần được điều chỉnh theo semantics đã chốt ở D0 nếu `sold_quantity` trong PostgreSQL chỉ cập nhật bất đồng bộ. Không được dùng một query sai semantics rồi tuyên bố pass.

**Invariant I2 — reservation idempotency**

- Một `reservation_id/order_id` chỉ reserve một lần.
- `release` lần đầu hoàn đúng quantity; lần thứ hai là no-op có audit/metric.
- `confirm` lần đầu chuyển reservation sang confirmed; gọi lại không hoàn kho và không làm thay đổi stock.
- `release` sau `confirm` không được hoàn kho.
- `confirm` sau `release/expired` không được biến đơn đã hết hạn thành paid tự động.

**Invariant I3 — idempotency request**

- Cùng `(scope/user, idempotency_key)` và cùng request fingerprint trả cùng kết quả.
- Cùng key nhưng payload khác phải trả lỗi conflict, không xử lý như request mới.
- Hai request đồng thời cùng key chỉ một request được phép reserve.
- Crash giữa các bước phải có trạng thái có thể retry/reconcile, không tạo order thứ hai.

**Invariant I4 — state transition**

Order tối thiểu:

```text
pending_payment → paid
pending_payment → payment_failed
pending_payment → expired
pending_payment → cancelled
```

Không cho `paid → pending_payment`, `paid → expired` hay `payment_failed → paid` nếu chưa có policy late-payment được duyệt. Payment event duplicate/out-of-order phải không làm lùi state.

**Invariant I5 — durable processing**

- NATS at-least-once nghĩa là consumer phải idempotent, không phải message chỉ đến một lần.
- DB unique constraint và/hoặc inbox/processed-event record phải bảo vệ side effect.
- Không dùng từ "eventual consistency" để che việc reserve thành công nhưng order/message bị mất.

### 3.2. Fairness của Baseline và Optimized

Hai implementation phải dùng:

- Cùng API contract/harness hoặc adapter được chứng minh tương đương.
- Cùng một product/item, cùng `total_quantity`, `max_per_user`, user generator, idempotency pattern.
- Cùng tỷ lệ request hợp lệ, hết hàng, rate-limited, retry và payment outcome.
- Cùng warm-up, duration, reset data, số run, collection window.
- Chạy tuần tự để k6, Prometheus, Grafana và ứng dụng không tranh tài nguyên giữa hai run.
- Cùng tổng resource budget theo **đoạn cấu hình đã khóa**; không so sánh một kiến trúc được cấp thêm CPU/RAM rồi gọi là công bằng.

### 3.3. Single-host resource discipline

- Giai đoạn phát triển chạy profile tối thiểu: PostgreSQL, Redis, NATS, một API, một worker, k6 khi cần.
- Profile observability bật Prometheus/Grafana/cAdvisor khi đo hoặc demo; không bắt mọi developer run cùng toàn bộ stack.
- Không bật Baseline và Optimized benchmark đồng thời.
- Redis inventory không được dùng eviction policy có thể xóa key stock. Phải chốt `noeviction` hoặc cơ chế bảo vệ tương đương và để lại headroom.
- Resource limits là budget kiểm thử, không phải kết quả năng lực. Mọi thay đổi phải ghi trong benchmark metadata.

---

## 4. Decision Gate D0 — các điểm phải chốt trước implementation

Đây là các mâu thuẫn hoặc chỗ thiếu contract trong tài liệu hiện có. Không được chọn ngầm trong code.

| ID | Mâu thuẫn/câu hỏi | Tác động | Khuyến nghị để triển khai | Người/artefact cần chốt |
|---|---|---|---|---|
| D0.1 | Optimized được mô tả vừa `Redis → NATS → worker → PostgreSQL`, vừa ghi order PostgreSQL đồng bộ trước khi publish. API lúc thì 201, lúc 202; có chỗ nói `<5ms`. | Quyết định durability và semantics response; ảnh hưởng lost order/crash recovery. | **CHỐT:** API trả 201 Created, thanh toán ngay lập tức, dùng NATS JetStream (persistent on disk) để không mất order. `<5ms` là target hypothesis, không phải kết quả nghiệm thu. | ADR `order-durability-and-response.md`, API contract |
| D0.2 | Path khác nhau: `/flash-sale/order`, `/optimized/orders`, `/baseline/orders`; SDD yêu cầu cùng workload. | k6 và frontend có thể đo hai nghiệp vụ khác nhau. | **CHỐT:** Dùng chung 1 endpoint `POST /api/v1/flash-sale/orders`, chung 1 schema, 2 bên validate như nhau nhưng phân tách port `:8081` cho Baseline, `:8082` cho Optimized để đảm bảo tính công bằng. | OpenAPI/contract doc |
| D0.3 | Baseline trừ `sold_quantity` ngay khi order; Optimized docs nói chỉ tăng sold khi paid, nhưng Redis stock đã reserve từ lúc order. | Công thức stock/reconciliation và "orders success" không thống nhất. | **CHỐT:** Tách rõ `available`, `reserved`, `paid`. Định nghĩa metric và DB projection riêng. Baseline giữ semantics direct DB, Optimized tách state rõ ràng; harness so sánh cùng business outcome. | Inventory state model |
| D0.4 | Multi-item order yêu cầu "tất cả hoặc không", nhưng Lua scripts chỉ mô tả một item/key. | Có thể reserve dở dang và oversell/khó release. | **CHỐT:** Mỗi order flash sale chỉ được săn mua đúng 1 sản phẩm duy nhất (trong SDD ghi có thể mở rộng nhiều món trong tương lai, nhưng nằm ngoài phạm vi hiện tại). | Lua contract + test matrix |
| D0.5 | `cleanup_expired.lua` parse member để suy ra user key, trong khi reservation có thể nhiều item và retry. | Cleanup sai key, double release hoặc leak stock. | **CHỐT:** Có thể chuẩn hóa và confirm xóa reservation trước khi cleanup quét tới (có thể làm tối giản vì hệ thống chủ yếu đánh vào khả năng chịu tải). Reservation record phải chứa đủ `reservation_id`, `item_id`, `user_id`, `qty`, state. | Redis key schema |
| D0.6 | SePay docs nói HMAC/checksum, nhưng chưa có payload/signature/config thực tế; agent rule cấm tự bịa. | Webhook có thể reject payment thật hoặc tạo lỗ hổng. | **CHỐT:** Triển khai simulator trước. Sau đó sẽ xem xét tích hợp thật khi có tài liệu/config đầy đủ. Adapter giữ verification provider-specific. | Payment decision + secret contract |
| D0.7 | `refund_pending`/`late_payment`/`webhook_log` xuất hiện trong flow nhưng enum/schema không đầy đủ. | Late payment có thể làm state sai hoặc mất audit. | **CHỐT:** Khi đơn hàng đã expired mà người dùng mới thanh toán, chuyển trạng thái đơn thành expired và xử lý như thường, tạo thêm 1 `webhook_logs` để đánh trạng thái là `refund` để admin xử lý sau. Không chuyển thành paid sau khi expired. | Payment policy ADR |
| D0.8 | `idempotency_keys` có partial index `WHERE expires_at < now()`; PostgreSQL không cho predicate dùng hàm không immutable. | Migration có thể fail ngay. | **CHỐT:** Dùng index thường trên `expires_at`, cleanup query có `WHERE`. | Migration review |
| D0.9 | Resource table và replica count khác nhau: 2/3 API, baseline một API hay hai API; tổng CPU/RAM khác. | Benchmark không công bằng, số target trong War Room sai. | **CHỐT:** Khóa budget RAM/CPU, chạy test tuần tự từng bên. Tách `dev`, `demo`, `benchmark` profiles; khóa topology và budget trước run. War Room hiển thị topology thật. | `benchmark-matrix.yaml` |
| D0.10 | Docs nêu target/range như 200–500, 3.000–8.000, 4.500 RPS và p99 cụ thể; chưa có measurement. | Dễ biến hypothesis thành claim trước hội đồng. | **CHỐT:** Đổi label thành hypothesis/acceptance candidate; UI/README chỉ hiển thị measured artifact sau run. | Benchmark report policy |
| D0.11 | War Room docs đề xuất chạy k6 đến cả hai kiến trúc song song, còn fairness yêu cầu chạy tuần tự. | Tranh CPU làm sai kết quả. | **CHỐT:** Chạy tuần tự. Demo visual có thể hiển thị hai panel từ cùng run khi đã chứng minh parity; benchmark chính chạy tuần tự. Nếu chạy song song, đánh dấu là demo không dùng để kết luận. | War Room run mode |
| D0.12 | War Room muốn gọi Docker API/socket; public frontend không được gọi socket trực tiếp. | Rủi ro kill tùy ý và lộ quyền host. | **CHỐT:** Bỏ tính năng kill từ UI, chính operator sẽ kill container trực tiếp trong demo. Browser gọi Go controller localhost-only/auth/allowlist target/action cho các chức năng khác; socket chỉ mount vào controller profile demo nếu cần. | Security review |
| D0.13 | `maxmemory-policy allkeys-lru` mâu thuẫn với yêu cầu giữ inventory key. | Redis có thể tự xóa stock/idempotency. | **CHỐT:** Cấu hình `noeviction` và không eviction inventory. Alert memory nếu cần. | Redis config review |
| D0.14 | Frontend route dùng `:id` theo kiểu Express trong khi Next App Router dùng segment `[id]`; realtime stock có WebSocket nhưng contract chưa có endpoint riêng. | Scaffold route/API mismatch. | **CHỐT:** Dùng Next `[id]`; dùng WebSocket cho War Room, các trang người dùng dùng polling đơn giản. | Frontend/API contract |

**Definition of ready sau D0:** Tất cả D0.1–D0.14 đã được chốt. Có thể triển khai stage 1–13 theo thứ tự dependency.


---

## 5. Cấu trúc implementation dự kiến

Đây là cấu trúc đề xuất để giảm coupling giữa hai kiến trúc và giúp cùng harness.

```text
.
├── cmd/
│   ├── api-baseline/
│   ├── api-optimized/
│   ├── worker/
│   ├── scheduler/
│   ├── warroom-controller/
│   └── migrate-seed/
├── internal/
│   ├── domain/                 # order, payment, inventory state machine
│   ├── httpapi/                # shared DTO, validation, error envelope
│   ├── baseline/               # direct PostgreSQL implementation
│   ├── optimized/              # Redis/Lua/NATS implementation
│   ├── inventory/
│   ├── payments/
│   ├── idempotency/
│   ├── persistence/
│   ├── messaging/
│   ├── observability/
│   └── chaos/
├── db/
│   ├── migrations/
│   ├── queries/
│   └── seed/
├── scripts/
│   ├── compose/
│   ├── reset/
│   ├── seed/
│   └── benchmark/
├── deploy/
│   ├── compose/
│   ├── nginx/
│   ├── postgres/
│   ├── redis/
│   ├── nats/
│   ├── prometheus/
│   └── grafana/
├── web/
│   ├── app/
│   ├── components/
│   ├── hooks/
│   ├── lib/
│   └── stores/
├── loadtests/
│   ├── scenarios/
│   ├── fixtures/
│   └── results/
├── test/
│   ├── contract/
│   ├── integration/
│   ├── concurrency/
│   ├── e2e/
│   └── system/
├── docs/
│   ├── decisions/
│   ├── runbooks/
│   └── benchmark/
└── Makefile / Taskfile / README.md
```

Tên thư mục có thể thay đổi nếu implementation chọn layout Go khác, nhưng domain/state machine, implementation-specific adapter và common test harness phải được tách rõ.

---

## 6. Bảng stages và dependency

| Stage | Tên | Mục tiêu chính | Phụ thuộc | Kết quả vertical slice |
|---:|---|---|---|---|
| 0 | Contract & decision gate | Chốt các mâu thuẫn, API/state/inventory semantics, resource matrix | Không | Không code nghiệp vụ; có contract để code không lệch |
| 1 | Host/Compose foundation | Dựng stack tối thiểu, network, health, config/secrets | 0 | `health` trả đúng cho service tối thiểu |
| 2 | Schema, migration, seed | Tạo DB schema an toàn và dataset tái lập | 1 | Fresh stack có event/item/user và reset được |
| 3 | Baseline order slice | Hoàn chỉnh request → DB lock → order, cùng error contract | 2 | Đặt hàng Baseline chạy end-to-end |
| 4 | Optimized reservation slice | Redis Lua, idempotency claim, NATS/worker, durability contract | 2, 3, D0.1–D0.5 | Đặt hàng Optimized và reconciliation chạy |
| 5 | Payment simulator & restock | Confirm/fail/timeout/cancel, release/confirm idempotency | 3, 4 | Đủ lifecycle order/payment ở cả hai implementation |
| 6 | SePay adapter thật cho UI | Initiate QR, verify webhook theo evidence, audit/late policy | 5, D0.6–D0.7 | UI có thể thanh toán thật an toàn khi có credentials/tunnel |
| 7 | Common observability & reliability | Metrics/logs/health/SLO/runbook, labels arch | 3, 4, 5 | Số liệu có provenance, alert invariant |
| 8 | Frontend product flow | UI flash sale, auth/cart/checkout/order, theme/animation | 5, API contracts | Demo product flow không ảnh hưởng load harness |
| 9 | War Room | Live metrics, summary, controls, activity log | 7, 8, D0.11–D0.12 | Demo quan sát được và chaos có guardrail |
| 10 | QA test pyramid & invariant verifier | Unit/contract/integration/concurrency/e2e | 3–9 theo phần | Không merge khi correctness fail |
| 11 | Benchmark harness & fair runs | k6 scenario, reset, metadata, median/variance | 7, 10, D0.9–D0.10 | Có raw benchmark artifact, chưa claim nếu fail |
| 12 | Chaos, soak, system & recovery | Fault injection, replay, recovery, resource/long-run | 7, 9, 10, 11 smoke | Có evidence về failure mode và runbook |
| 13 | Demo hardening & handoff | Freeze config, rehearsal, evidence pack, rollback | 11, 12 | Bộ demo lặp lại được và handoff rõ |

Các stage 6 và 8 có thể triển khai song song sau khi stage 5 cung cấp contract; stage 7 có thể bắt đầu phần instrumentation từ stage 3–4 nhưng chỉ hoàn thiện sau khi flows ổn định. Stage 11 không được bắt đầu trước khi stage 10 pass các concurrency/invariant test nền tảng.


---

# 7. Chi tiết từng stage (tóm tắt deliverables/acceptance/handoff)

## Stage 0 — Contract, decision gate và skeleton documentation

**Mục tiêu:** Biến SDD thành các contract triển khai có thể kiểm thử; khóa các mâu thuẫn có thể làm sai benchmark hoặc mất order.

**Deliverables:**
- Decision records D0.1–D0.14 trong `docs/decisions/`.
- API/OpenAPI contract chung, state machine doc trong `docs/domain/`.
- Benchmark matrix YAML: topology, CPU, memory, seed, scenarios.
- Acceptance checklist và reconciliation query template.

**Files:** `docs/decisions/*.md`, `docs/contracts/openapi.yaml`, `docs/domain/*.md`, `docs/benchmark/benchmark-matrix.yaml`

**Acceptance:** Không còn blocker mâu thuẫn chưa chốt; có request contract mà cả hai kiến trúc đều thực hiện; các số RPS/latency trong docs được đánh dấu hypothesis; resource matrix và run mode được lock.

**Handoff:** Stage 1–5 nhận contract, state machine, invariant list và benchmark matrix. Không agent nào tự suy ra semantics từ code cũ.

---

## Stage 1 — Host foundation, Docker Compose và health slice

**Mục tiêu:** Dựng môi trường single-host tối thiểu, reproducible và có thể kiểm tra health trước khi đưa business logic vào.

**Deliverables:**
- Go module baseline, `cmd/api-baseline`, `cmd/api-optimized`, `cmd/worker`, `cmd/warroom-controller`.
- Next.js app shell.
- Dockerfile multi-stage.
- Compose profiles: `dev`, `test`, `observability`, `demo`, `benchmark`.
- Liveness/readiness contract.
- `.env.example`, `Makefile`, `README.md` tối thiểu.

**Files:** `go.mod`, `cmd/*/main.go`, `Dockerfile`, `web/Dockerfile`, `deploy/compose/*.yaml`, `internal/httpapi/health.go`, `internal/config/config.go`

**Acceptance:** Fresh checkout build được; Compose up không cần secret thật; PostgreSQL/Redis/NATS healthcheck pass; readiness chỉ pass khi dependency ready; không expose DB/Redis ra host mặc định; idle RAM được ghi lại và không vượt budget.

**Handoff:** Stage 2 dùng service names/URL/endpoints đã khóa, không hardcode localhost trong container.

---

## Stage 2 — PostgreSQL migration, Redis keyspace và seed/reset

**Mục tiêu:** Có schema và dataset deterministic để mọi flow và mọi benchmark bắt đầu từ cùng trạng thái.

**Deliverables:**
- Migration users, products, flash_sale events/items, orders/order_items, payments, idempotency/audit.
- Repository interface/queries nền.
- Seed deterministic và reset script.
- Redis warm-up từ DB, inventory snapshot.
- SQL reconciliation checker.

**Files:** `db/migrations/*.sql`, `internal/persistence/postgres/*.go`, `cmd/migrate-seed/main.go`, `db/seed/*.json`, `scripts/seed/seed.sh`, `scripts/reset/reset.sh`, `test/system/reconcile.sql`

**Acceptance:** Database chạy migration thành công; seed cùng config cho cùng logical IDs; có event active và stock từ DB/Redis; constraints không chứa predicate không hợp lệ; reset đưa cả DB và Redis về baseline; reconciliation pass.

**Handoff:** Stage 3–4 không tự query các cột ngoài schema contract; seed manifest là input bắt buộc của k6 và reconciliation.

---

## Stage 3 — Baseline order vertical slice

**Mục tiêu:** Baseline từ health/seed đến order thành công, hết hàng, idempotency và payment-pending bằng đúng mô hình `Go API → PostgreSQL SELECT ... FOR UPDATE`.

**Deliverables:**
- Baseline order endpoint theo common contract.
- Repository transaction và order state service.
- Error mapping `400/404/409/429/5xx`.
- Payment simulator contract tối thiểu.
- Metrics request/order/inventory cơ bản.

**Files:** `cmd/api-baseline/main.go`, `internal/domain/*.go`, `internal/httpapi/order_handler.go`, `internal/baseline/order_service.go`, `internal/persistence/postgres/order_repository.go`, `internal/payments/simulator.go`, test files

**Acceptance:** Request hợp lệ tạo đúng một order/pending payment và trừ inventory; concurrent không oversell/stock âm; duplicate key không tạo order thứ hai; cùng key khác payload bị reject; transaction rollback không để dở dang; race detector pass.

**Handoff:** Stage 4 reuse DTO/error/metrics naming nhưng không reuse Baseline persistence implementation. Stage 10 lấy Baseline test vector làm oracle.

---

## Stage 4 — Optimized reservation, idempotency, NATS và worker

**Mục tiêu:** Implement đường Optimized với Redis/Lua atomic inventory, queue/worker durable theo D0.1 và không mất order khi crash.

**Deliverables:**
- Optimized API order slice.
- Lua scripts: claim-and-reserve/reserve/release/confirm/cleanup.
- NATS JetStream stream/consumer bootstrap.
- Worker order persistence/retry/DLQ.
- Recovery/reconciliation command.

**Files:** `cmd/api-optimized/main.go`, `cmd/worker/main.go`, `internal/optimized/order_service.go`, `internal/inventory/redis_repository.go`, `internal/inventory/lua/*.lua`, `internal/messaging/*.go`, tests

**Acceptance:** Concurrent reserve không làm stock âm; cùng idempotency key chỉ một reserve; multi-item all-or-nothing; publish/worker retry không duplicate order; release/confirm lặp lại no-op; Redis down fail fast 503; NATS restart giữ message; DB chậm không mất durable command; race/integration tests pass.

**Handoff:** Stage 5 chỉ gọi public use case; không tự thao tác Redis key. Stage 7 dùng metrics names đã ổn định. Stage 10 dùng event fixtures kiểm tra at-least-once.

---

## Stage 5 — Payment simulator, confirm/fail/timeout/cancel và restock

**Mục tiêu:** Hoàn thành lifecycle nghiệp vụ cho cả hai implementation, đặc biệt hoàn kho đúng một lần khi payment fail/timeout/cancel.

**Deliverables:**
- Payment state machine/service chung cho simulator và provider adapter.
- Simulator endpoint/fixture: success/failure/timeout/duplicate/out-of-order.
- Expiry scheduler/worker.
- Release/confirm handlers.
- Reconciliation report.

**Files:** `internal/payments/service.go`, `state_machine.go`, `simulator.go`, `cmd/scheduler/main.go`, `internal/messaging/payment_handlers.go`, test files

**Acceptance:** Simulator ratios cấu hình và tái lập; mỗi terminal outcome chỉ áp dụng một lần; payment failure và timeout hoàn stock đúng một lần; confirm không hoàn stock; late payment sau expiry không tự chuyển paid; state transition invalid bị reject; reconciliation pass.

**Handoff:** Stage 6 dùng payment service contract và fixtures; Stage 8 dùng initiate/status contract; Stage 10 dùng lifecycle vectors làm invariant oracle.

---

## Stage 6 — SePay adapter thật cho UI

**Mục tiêu:** Cho phép người dùng trong UI tạo QR/chuyển khoản và nhận webhook SePay thật, nhưng cô lập credentials và không ảnh hưởng load test.

**Deliverables:**
- SePay adapter và env contract.
- Webhook audit/test fixtures.
- Runbook tunnel/demo.
- Manual-review/late-payment handling.

**Files:** `internal/payments/sepay/*.go`, `internal/httpapi/sepay_webhook_handler.go`, `docs/runbooks/sepay-local-demo.md`, test fixtures

**Acceptance:** Không có credentials thật trong Git; valid fixture cập nhật đúng payment/order một lần; invalid signature không thay đổi stock; duplicate webhook không lặp side effect; late payment tuân thủ policy; k6 không gọi SePay thật.

**Handoff:** Stage 8 chỉ cần provider-neutral API; không nhúng logic SePay vào component frontend. Stage 12 không dùng tài khoản thật.

---

## Stage 7 — Observability, metrics, logs và reliability baseline

**Mục tiêu:** Đảm bảo mọi số liệu War Room/benchmark có nguồn, label thấp cardinality và phân biệt business rejection với system failure.

**Deliverables:**
- Metric catalog, Prometheus metrics: HTTP, order, payment, stock, Redis, NATS, DB, goroutines, circuit breaker.
- Structured logs, correlation ID middleware.
- Prometheus/Grafana config/dashboard/alert rules.
- Runbook API kill, Redis down, DB slow, queue backlog.

**Files:** `internal/observability/*.go`, `deploy/prometheus/*.yml`, `deploy/grafana/provisioning/**/*.{json,yaml}`, `docs/observability/metric-catalog.md`, `docs/runbooks/*.md`

**Acceptance:** Truy nguyên order qua correlation ID; 409/429 không là 5xx outage; p50/p95/p99 từ histogram đúng route/arch; oversell metric không reset vô tình; dashboard hiển thị stale khi mất dữ liệu; alert có synthetic test; overhead được đo dưới benchmark profile.

**Handoff:** Stage 9 dùng metric schema xây live stream; Stage 11 dùng metric labels làm benchmark metadata; Stage 12 dùng runbook làm expected recovery.

---

## Stage 8 — Frontend product flow và design system

**Mục tiêu:** Xây UI trình bày hiện đại nhưng không chen vào harness; đủ luồng auth/cart/checkout/payment/order cho demo bằng tay.

**Deliverables:**
- Next.js App Router + TypeScript + Tailwind + Framer Motion.
- Design tokens: nền/aurora/spotlight/spin border/gradient fill/theme.
- Pages/routes: home, flash-sale `[id]`, auth, cart, checkout, orders, order detail.
- Auth/cart/order screens, payment QR/status polling.
- Theme/animation/reduced-motion implementation.

**Files:** `web/package.json`, `web/app/**/*.tsx`, `web/components/**/*`, `web/lib/*.ts`, `web/hooks/*.ts`, `web/stores/*.ts`, tests

**Acceptance:** User demo được từ flash sale → order → initiate payment → xem status; UI chỉ render metrics/order/stock từ API; duplicate click dùng idempotency key ổn định; stale/disconnect báo rõ; theme switch, reduced-motion, keyboard navigation và contrast pass; animation không rerender toàn cây mỗi mousemove; lint/typecheck/build pass.

**Handoff:** Stage 9 có thể tái sử dụng design system và types; Stage 11 không chạy browser UI làm load generator trừ scenario được ghi rõ.

---

## Stage 9 — War Room dashboard và chaos controller

**Mục tiêu:** Cho hội đồng nhìn thấy hai architecture, health, metrics, activity và recovery; cung cấp chaos có allowlist và safety boundary.

**Deliverables:**
- War Room API/WebSocket: snapshot metrics, live stream, test lifecycle, activity log, stale/reconnect.
- Test runner controller chỉ gọi allowlist profile/script, tuần tự benchmark, parallel demo phải gắn nhãn.
- Chaos controller: browser không gọi socket; Go controller localhost-only/auth/allowlist; confirm/cooldown/audit; mặc định chỉ kill API Optimized; Redis/DB advanced có cảnh báo.
- Frontend `/war-room`: header aurora, control bar, Baseline/Optimized panel, charts, oversell indicator, summary, activity log, stale badge, reconnect.

**Files:** `cmd/warroom-controller/main.go`, `internal/warroom/*.go`, `internal/chaos/*.go`, `web/app/war-room/**/*`, tests

**Acceptance:** Idle screen hiển thị Unknown/healthy đúng; start/stop có state machine; live stream reconnect được; mất Prometheus/WS có stale marker; Baseline/Optimized panel phản ánh topology thật; kill API qua controller có audit, target ngoài allowlist bị reject; recovery time đo từ event injection; summary từ run result có run ID; browser performance/accessibility pass.

**Handoff:** Stage 12 dùng controller inject failure có kiểm soát. Stage 13 dùng runbook và layout đã rehearsal, không sửa metric semantics vì lý do trình bày.

---

## Stage 10 — QA test pyramid, concurrency và invariant verifier

**Mục tiêu:** Xác minh correctness trước performance; tạo cùng test harness cho Baseline/Optimized và fail fast khi dữ liệu sai.

**Deliverables:**
- Unit tests state machines, validation/error mapping, Lua, backoff/circuit breaker, payment provider mapping.
- Contract tests common order API, error/status/body, simulator/payment/webhook, War Room protocol.
- Integration tests với PostgreSQL/Redis/NATS.
- Concurrency tests: same item shock, same key, different users max-per-user, multi-item all-or-nothing, payment vs expiry vs cancel, worker redelivery/crash before ACK, Redis restart/rebuild.
- Invariant verifier chạy sau mỗi scenario: stock equation, no negative/oversell, no duplicate order/event, all reservation terminally confirmed/released, order/payment terminal state valid.
- E2E browser simulator flow.
- Security checks secrets/authz/webhook/chaos endpoint/SQL injection.
- Race detector, leak checks, goroutine/connection pool checks.

**Files:** `test/unit/*`, `test/contract/*`, `test/integration/*`, `test/concurrency/*`, `test/e2e/*`, `test/system/invariant_verifier.go`, `test/fixtures/*`, scripts, docs test matrix

**Acceptance:** Tất cả invariant tests pass ở cả hai architecture; concurrent same key đúng một order; concurrent stock không oversell; release/confirm duplicate không lệch stock; NATS redelivery không duplicate side effect; expected 409/429 không fail như outage; race detector pass; không được đi benchmark nếu invariant verifier fail.

**Handoff:** Stage 11 chỉ dùng script/harness đã pass stage 10, cùng seed/reset và verifier; không copy k6 script mới làm lệch contract.

---

## Stage 11 — k6 benchmark và fair comparison

**Mục tiêu:** Đo throughput/latency/error/resource thực tế trên single host, không biến target trong docs thành lời hứa.

**Deliverables:**
- k6 scenarios: smoke, max sustainable throughput, flash-sale shock, stress, payment/restock, resilience orchestration, soak.
- User/idempotency generator deterministic theo VU/iteration.
- Phân loại kết quả: success/accepted, expected business rejection 409/429, system failure 5xx/timeout, client/harness error.
- Chạy tuần tự: clean/reset, seed/warm-up, measurement, raw summary/Prometheus/Grafana, invariant verification, teardown.
- Tối thiểu ba run; report median/spread/variance/outlier.
- Metadata: revision, OS/kernel, CPU/RAM, Docker/Compose, Go/Node/k6 versions, profile, limits, seed, scenario, time, thermal note nếu có.
- CPU/RAM host/container, k6 overhead, Prometheus overhead, queue lag.
- Report template chưa điền số giả.

**Files:** `loadtests/scenarios/*.js`, `loadtests/lib/*.js`, `scripts/benchmark/*.sh`, `loadtests/results/.gitkeep`, `docs/benchmark/report-template.md`, `docs/benchmark/measurement-provenance.md`, Grafana benchmark dashboard

**Acceptance:** Same script trỏ Baseline/Optimized không đổi payload; run invalid nếu seed/reset/warm-up/limits/metadata thiếu hoặc verifier fail; report phân biệt measured với hypothesis; có RPS, p50/p95/p99, expected rejection rate, 5xx/timeout, queue lag, resource usage và invariant result; không công bố Max RPS là số duy nhất nếu plateau chưa định nghĩa; kết quả lặp lại được.

**Handoff:** Stage 12 dùng baseline run/smoke làm reference và chỉ inject failure trong profile local có cleanup. Stage 13 chỉ dùng measured artifacts đã verify.

---

## Stage 12 — Chaos, resilience, soak và system testing

**Mục tiêu:** Chứng minh failure behavior bằng evidence thực tế; kiểm tra recovery và invariant sau sự cố.

**Deliverables:**
- API instance kill trong Optimized khi tải cao: đo loss/duplicate, Nginx health removal/reroute, worker/message persistence, restart readiness.
- Baseline API kill để minh họa SPOF, tách khỏi claim Optimized recovery.
- Redis down/restart: reject khi circuit open, persistence/rebuild path, block until reconciliation pass.
- PostgreSQL slow/down qua Toxiproxy: queue/backpressure/retry/DLQ, không mất durable order.
- NATS restart/worker kill trước ACK: redelivery, idempotent side effect, DLQ alert.
- Payment/expiry/cancel race chaos.
- Soak test moderate load 10 phút, theo dõi heap/goroutine/pool/queue/memory.
- System E2E từ compose up → migration/seed → order/payment/restock → metrics → reset.
- Cleanup sau mọi chaos; chỉ chạy advanced Redis/DB chaos khi có backup/recovery command và operator xác nhận.

**Files:** `scripts/chaos/*.sh`, `scripts/system/*.sh`, `test/system/chaos_*_test.go`, `docs/benchmark/chaos-report-template.md`, `docs/runbooks/recovery-checklist.md`

**Acceptance:** API kill được phát hiện, traffic không tiếp tục gửi vào instance down sau health interval, recovery time measured; request đã durable không mất; Redis down không oversell sau restart; DB slow không báo thành công sai contract; worker redelivery không duplicate; sau chaos invariant verifier pass; soak không có tăng không kiểm soát heap/goroutine/connection/queue và inventory drift.

**Handoff:** Stage 13 nhận recovery times/evidence thực tế, known limitations và lệnh cleanup; không thay số docs nếu chưa có artifact.

---

## Stage 13 — Demo hardening, evidence pack và context handoff

**Mục tiêu:** Đóng gói demo lặp lại được và giúp người tiếp quản biết đã làm gì, chưa làm gì, giới hạn còn lại.

**Deliverables:**
- Freeze topology, image tags, resource matrix, seed config và benchmark command.
- Full rehearsal: health, seed, Baseline/Optimized smoke/benchmark, payment simulator/restock, War Room live, API kill/recovery, UI SePay, teardown/reset.
- Evidence pack không chứa secret: raw k6 summary, Prometheus/Grafana export/screenshot có timestamp/run ID, compose config digest/resource limits, invariant/reconciliation output, chaos timeline, environment metadata.
- Report chỉ dùng measured values, có confidence/variance/limitation.
- README one-command và troubleshooting.
- Review cuối theo code-reviewer checklist.
- Handoff note trong docs/PR description; không commit theo yêu cầu hiện tại.

**Files:** `docs/runbooks/demo-day.md`, `docs/benchmark/results/<run-id>/{summary.json,metadata.json,invariants.txt,README.md}`, `docs/known-limitations.md`, `README.md`, `scripts/demo/*.sh`

**Acceptance:** Người khác đọc README/runbook có thể khởi động, seed, smoke và reset; demo không dùng target/hypothesis như measured result; mọi chart/table có run ID và timestamp; không có secret/socket exposure trong default profile; full rehearsal pass hoặc failure được ghi rõ; working tree không commit ngoài yêu cầu; artifact/generated files có policy.

**Handoff:** Phải gồm D0 decisions và còn mở; cách chạy từng profile; seed/reset/reconcile commands; contract/state/invariant summary; test commands và pass/fail/known flaky; benchmark metadata/artifact locations; failure modes và recovery steps; các phần chưa implement ghi cụ thể.

---

## 8. Critical path ngắn gọn

```text
D0 contract/decisions
  ↓
Compose health/readiness
  ↓
migration + deterministic seed/reset
  ↓
Baseline order (DB lock)
  ↓
Optimized reserve/idempotency/durability + NATS worker
  ↓
payment simulator + expiry/fail/cancel + restock
  ↓
metrics/logs/health/reconciliation
  ├── frontend product flow
  └── War Room + controlled chaos
  ↓
unit/contract/integration/concurrency/invariant tests
  ↓
fair sequential benchmark
  ↓
chaos/soak/system/recovery
  ↓
demo hardening + measured evidence handoff
```

Đường ngắn nhất để có demo kỹ thuật đáng tin cậy là dừng sau Stage 5/7/10 với UI tối thiểu, rồi mới làm polish frontend/War Room. Đường ngắn nhất để có demo trước hội đồng vẫn phải đi qua invariant verifier; không được bỏ qua correctness để lấy RPS.

---

## 9. Ma trận test tổng hợp

| Tầng | Baseline | Optimized | Khi chạy | Gate |
|---|---|---|---|---|
| Unit/domain | state/order/payment/validation | state/order/payment/Lua mapping | Mỗi thay đổi | Pass |
| Contract | common order/payment/error | common order/payment/error/NATS | Mỗi API change | Pass |
| Integration | PostgreSQL lock/transaction | Redis/NATS/DB/worker | Trước merge | Pass |
| Concurrency | cùng item/key, lock contention | reserve/key/release/worker redelivery | Trước benchmark | Không oversell/duplicate |
| Lifecycle | payment/expiry/cancel | payment/expiry/cancel/reconcile | Stage 5 trở đi | State/inventory pass |
| E2E browser | simulator/UI | simulator/UI/War Room | Smoke/rehearsal | Pass hoặc limitation |
| Load | common k6 | common k6 | Stage 11 | Metadata + invariant pass |
| Chaos | baseline SPOF observation | API/Redis/DB/NATS failure | Stage 12 | Recovery/evidence |
| Soak | resource baseline | worker/queue/Redis/DB resource | Stage 12 | Không drift/leak |
| Security | input/auth/secret | webhook/chaos/secret/Redis boundary | Trước demo | No blocker |

---

## 10. Checklist merge/review cho từng stage

- [ ] Đã đọc contract/decision liên quan, không tự đổi semantics.
- [ ] Có test cho happy path và failure path.
- [ ] Nếu chạm inventory: đã kiểm tra I1–I5.
- [ ] Nếu chạm async: đã kiểm tra duplicate delivery, retry, ACK ordering và crash window.
- [ ] Nếu chạm payment: đã kiểm tra duplicate, late, mismatch, out-of-order và audit.
- [ ] Nếu chạm metric: label cardinality thấp, query phân biệt business rejection/system failure.
- [ ] Nếu chạm frontend: không hardcode benchmark, có stale/error/reduced-motion.
- [ ] Nếu chạm chaos: allowlist/auth/confirmation/audit/cleanup.
- [ ] Có lệnh verification cụ thể và output được lưu khi cần.
- [ ] Có rollback hoặc feature flag; không phá đường chạy Baseline/seed/reset.
- [ ] Handoff đã nêu file, config, known limitation và bước kế tiếp.

---

## 11. Các giả định đang dùng trong kế hoạch

1. Chỉ một host; Docker Compose đủ cho mục tiêu trình diễn.
2. PostgreSQL là durable source cho order/payment, Redis là hot inventory/idempotency state theo semantics được D0 chốt.
3. NATS JetStream là queue được ưu tiên trong SDD vì footprint phù hợp; thay đổi queue phải cập nhật resource matrix và contract.
4. k6 là load generator chính; chạy cùng host nên mọi kết quả là host-local measurement, không đại diện production.
5. SePay thật chỉ có trong UI demo khi user cung cấp config hợp lệ; simulator là đường test mặc định.
6. War Room là custom Next.js page và Go controller; Grafana vẫn là nguồn quan sát/đối chiếu.
7. Các số trong SDD như `>3.000 RPS`, `<10s`, `200–500 RPS`, `p95 <500ms` chỉ là target/hypothesis cho tới khi raw benchmark xác minh.
8. Không giả định cơ chế signature hay payload SePay ngoài tài liệu/config đã kiểm chứng.

Nếu một giả định bị bác bỏ, cập nhật decision record, contract, dependency table và các stage liên quan trước khi code tiếp.

---

**Hết kế hoạch.**

