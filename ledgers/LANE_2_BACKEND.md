# Parallel Ledger — LANE 2: Backend Core (Baseline, Optimized, Payment & SePay)

- **Assigned Stages:** `Stage 3` → `Stage 4` → `Stage 5` → `Stage 6`
- **Current Stage:** `Stage 3`
- **Current Phase:** `READY`
- **Owned Directories/Files (CHỈ ĐƯỢC SỬA CÁC ĐƯỜNG DẪN NÀY):**
  - `cmd/api-baseline/`, `cmd/api-optimized/`, `cmd/worker/`, `cmd/scheduler/`
  - `internal/domain/`, `internal/httpapi/`, `internal/baseline/`, `internal/optimized/`
  - `internal/inventory/`, `internal/messaging/`, `internal/payments/`, `internal/persistence/`
  - `ledgers/LANE_2_BACKEND.md`
- **Commit Command bắt buộc:**
  `./scripts/safe-commit.sh "feat(stage-X): <summary>" cmd/api-baseline cmd/api-optimized cmd/worker cmd/scheduler internal/domain internal/httpapi internal/baseline internal/optimized internal/inventory internal/messaging internal/payments internal/persistence go.mod go.sum ledgers/LANE_2_BACKEND.md`

---

## Tiến độ từng Stage của Lane 2

| Stage | Tên Stage | Primary Agent | Support Agent | Reviewer | Status | Fix Rounds | Commit SHA |
|---:|---|---|---|---|---|---:|---|
| **3** | Baseline order vertical slice | `backend-core` | - | `code-reviewer` | `PENDING` | 0 | - |
| **4** | Optimized reservation, idempotency, NATS & worker | `backend-core` | - | `code-reviewer` | `PENDING` | 0 | - |
| **5** | Payment simulator, confirm/fail/timeout/cancel & restock | `backend-core` | `payment-integrations` | `code-reviewer` | `PENDING` | 0 | - |
| **6** | SePay adapter thật cho UI | `payment-integrations` | `backend-core` | `code-reviewer` | `PENDING` | 0 | - |

---

## Nhật ký thực thi Lane 2
*(Cập nhật sau mỗi stage)*
