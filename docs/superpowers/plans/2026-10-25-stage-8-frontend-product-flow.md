# Stage 8: Frontend Product Flow & Design System — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a modern, responsive, accessible Next.js frontend with design tokens (dark/light theme, Aurora hero, spotlight cards, spinning celebration borders) and complete product flows (home catalog, flash sale item detail with Baseline vs Optimized architecture toggle, one-click flash order checkout with deterministic idempotency, and order detail with payment QR, status polling, and simulator triggering) — without interfering with backend benchmark harnesses.

**Architecture:** Next.js 15 App Router (React 19, TypeScript, Tailwind CSS) → Typed API Client (`web/src/lib/api.ts`) supporting both Baseline (`:8081`) and Optimized (`:8082`) backend ports → Backend Read APIs (`internal/httpapi/catalog_handler.go`) → Database / Redis cache → Polling Hook (`web/src/hooks/usePolling.ts`) for order payment status.

**Tech Stack:** Next.js 15.1.4, React 19, TypeScript 5, Tailwind CSS 3.4, Go 1.22 (`internal/httpapi`), PostgreSQL, Redis.

**Spec:** `/home/trisngocj/Projects/Báo cáo thực tập/ApexStock/IMPLEMENTATION_PLAN.md` (Stage 8, Section 7; Decision Gates D0.4, D0.14, D0.15; Invariants I1–I5).

## Global Constraints
- **Frontend Directory:** All web code lives in `web/`.
- **Zero Heavy Animation Bloat:** Spotlight effects must be driven by CSS custom variables (`--mouse-x`, `--mouse-y`) without triggering whole-tree React re-renders on mousemove.
- **Accessibility & Reduced Motion:** Respect `prefers-reduced-motion: reduce` by replacing animated gradients and spinning borders with static variants. Ensure full keyboard navigability (`Tab`, `Enter`, `Space`) and WCAG AA contrast compliance.
- **Decision Gate D0.14:** Product flow uses client-side HTTP polling (`usePolling.ts`) for order/payment lifecycle; WebSocket streaming is reserved exclusively for Stage 9 War Room.
- **Deterministic Idempotency Key:** Generate and pin a stable idempotency key per checkout session in memory to prevent duplicate orders upon rapid double-clicking.
- **Backend Read Endpoints:** Implemented in `internal/httpapi/catalog_handler.go` and mounted on both Baseline (`cmd/api-baseline/main.go`) and Optimized (`cmd/api-optimized/main.go`).

## Review Focus
1. **Double-Click Race Condition on Flash Buy Button**: A user aggressively double-clicks the "Flash Buy" button during the sale countdown rush.
   *Expected behavior:* The button disables immediately on first click (`loading = true`), reusing a cached deterministic idempotency key for that item/user attempt (`getOrCreateIdempotencyKey`), ensuring backend returns the same order without creating duplicate orders or double-deducting stock.
   *Pinned test:* `TestDeterministicIdempotencyKey_StableAcrossRapidClicks` in Task 3.
2. **Polling Thrashing & Resource Leakage on Tab Background / Terminal State**: Order status polling continues firing every second after the order has already been paid or expired, or when the user switches browser tabs.
   *Expected behavior:* `usePolling` hook listens to `document.visibilityState` (pausing when hidden) and automatically terminates polling when order status reaches a terminal state (`paid`, `payment_failed`, `expired`, `cancelled`).
   *Pinned test:* `TestUsePolling_TerminatesOnTerminalStatus` in Task 3.
3. **Accessibility & Reduced Motion Regression**: Flash sale animation effects cause disorientation or fail accessibility audits.
   *Expected behavior:* When `@media (prefers-reduced-motion: reduce)` is active, aurora background gradients, spinning borders, and countdown pulses are immediately replaced with clean, static styles.
   *Pinned test:* `TestReducedMotion_DisablesTransformAnimations` in Task 2.
4. **Mousemove Re-render Performance Degradation**: Spotlight card tracks cursor position using React component state (`useState`), triggering 60 FPS re-renders of the product catalog.
   *Expected behavior:* Spotlight card updates CSS variables directly on the DOM element (`ref.current.style.setProperty('--mouse-x', ...)`), achieving smooth 60 FPS cursor tracking with 0 React component re-renders.
   *Pinned test:* `TestSpotlightCard_UpdatesCSSVarsWithoutRerender` in Task 2.
5. **Backend Disconnect / Outage Feedback**: During chaos testing or network blips, the backend API becomes unreachable.
   *Expected behavior:* API client catches network exceptions and transitions UI into a clear, non-blocking error/retry state (`"Hệ thống đang quá tải hoặc mất kết nối, vui lòng thử lại"`) with an active Retry button, rather than unhandled promise rejections or blank screens.
   *Pinned test:* `TestApiClient_HandlesNetworkFailureGracefully` in Task 3.

---

### Task 1: Backend Read APIs for Frontend

**Files:**
- Create: `internal/httpapi/catalog_handler.go`
- Test: `internal/httpapi/catalog_handler_test.go`
- Modify: `cmd/api-baseline/main.go:35-45`
- Modify: `cmd/api-optimized/main.go:35-45`

**Interfaces:**
- Consumes: `*sql.DB`, `*redis.Client` (optional, for real-time stock in Optimized)
- Produces:
  ```go
  type CatalogHandler struct { ... }
  func NewCatalogHandler(db *sql.DB, rdb *redis.Client) *CatalogHandler
  func (h *CatalogHandler) ListActiveEvents(w http.ResponseWriter, r *http.Request)
  func (h *CatalogHandler) GetItemDetail(w http.ResponseWriter, r *http.Request)
  func (h *CatalogHandler) GetOrderDetail(w http.ResponseWriter, r *http.Request)
  ```

- [ ] **Step 1: Write failing unit/integration tests for catalog and order read endpoints**

Create `internal/httpapi/catalog_handler_test.go`:
```go
package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func getCatalogTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("BASELINE_DSN")
	if dsn == "" {
		dsn = "postgres://apexstock:apexstock@localhost:5432/apexstock_baseline?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping catalog test: cannot open DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping catalog test: DB unreachable: %v", err)
	}
	return db
}

func TestCatalogHandler_ListActiveEvents(t *testing.T) {
	db := getCatalogTestDB(t)
	defer db.Close()

	handler := NewCatalogHandler(db, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flash-sale/events", nil)
	w := httptest.NewRecorder()

	handler.ListActiveEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}

	var events []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(events) == 0 {
		t.Logf("no active events currently seeded; empty array returned as expected")
	}
}

func TestCatalogHandler_GetOrderDetail_NotFound(t *testing.T) {
	db := getCatalogTestDB(t)
	defer db.Close()

	handler := NewCatalogHandler(db, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/00000000-0000-0000-0000-000000000000", nil)
	w := httptest.NewRecorder()

	handler.GetOrderDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/httpapi -run TestCatalogHandler_`
Expected: FAIL (compilation error: `NewCatalogHandler` undefined).

- [ ] **Step 3: Implement `internal/httpapi/catalog_handler.go`**

Create `internal/httpapi/catalog_handler.go`:
```go
package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"apexstock/internal/domain"
	rediskeys "apexstock/internal/persistence/redis"
	"github.com/redis/go-redis/v9"
)

type CatalogHandler struct {
	db  *sql.DB
	rdb *redis.Client
}

func NewCatalogHandler(db *sql.DB, rdb *redis.Client) *CatalogHandler {
	return &CatalogHandler{db: db, rdb: rdb}
}

type FlashSaleItemDTO struct {
	ID              string `json:"id"`
	EventID         string `json:"event_id"`
	ProductID       string `json:"product_id"`
	ProductName     string `json:"product_name"`
	ProductDesc     string `json:"product_description"`
	ProductImage    string `json:"product_image"`
	OriginalPrice   int64  `json:"original_price"`
	SalePrice       int64  `json:"sale_price"`
	InitialQuantity int32  `json:"initial_quantity"`
	AvailableStock  int32  `json:"available_stock"`
	MaxPerUser      int32  `json:"max_per_user"`
}

type FlashSaleEventDTO struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	StartTime time.Time          `json:"start_time"`
	EndTime   time.Time          `json:"end_time"`
	Status    string             `json:"status"`
	Items     []FlashSaleItemDTO `json:"items"`
}

type OrderDetailDTO struct {
	OrderID         string    `json:"order_id"`
	UserID          string    `json:"user_id"`
	Status          string    `json:"status"`
	TotalAmount     int64     `json:"total_amount"`
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedAt       time.Time `json:"created_at"`
	ItemID          string    `json:"item_id"`
	ProductName     string    `json:"product_name"`
	ProductImage    string    `json:"product_image"`
	Quantity        int32     `json:"quantity"`
	UnitPrice       int64     `json:"unit_price"`
	PaymentStatus   string    `json:"payment_status"`
	PaymentMethod   string    `json:"payment_method"`
	TransactionRef  *string   `json:"transaction_ref"`
}

// ListActiveEvents returns active or scheduled flash sale events with catalog items and remaining stock.
func (h *CatalogHandler) ListActiveEvents(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Correlation-ID")
	ctx := r.Context()

	rows, err := h.db.QueryContext(ctx, `
		SELECT e.id, e.name, e.start_time, e.end_time, e.status,
		       i.id, i.product_id, p.name, COALESCE(p.description, ''), COALESCE(p.image_url, ''),
		       p.original_price, i.sale_price, i.initial_quantity, i.sold_quantity, i.max_per_user
		FROM flash_sale_events e
		LEFT JOIN flash_sale_items i ON i.event_id = e.id
		LEFT JOIN products p ON p.id = i.product_id
		WHERE e.status IN ('scheduled', 'active')
		ORDER BY e.start_time ASC, i.id ASC`)
	if err != nil {
		writeError(w, correlationID, domain.ServiceUnavailableError())
		return
	}
	defer rows.Close()

	eventsMap := make(map[string]*FlashSaleEventDTO)
	var orderedEventIDs []string

	for rows.Next() {
		var eventID, eventName, eventStatus string
		var startTime, endTime time.Time
		var itemID, prodID, prodName, prodDesc, prodImg sql.NullString
		var origPrice, salePrice sql.NullInt64
		var initQty, soldQty, maxUser sql.NullInt32

		if err := rows.Scan(
			&eventID, &eventName, &startTime, &endTime, &eventStatus,
			&itemID, &prodID, &prodName, &prodDesc, &prodImg,
			&origPrice, &salePrice, &initQty, &soldQty, &maxUser,
		); err != nil {
			writeError(w, correlationID, domain.ServiceUnavailableError())
			return
		}

		event, exists := eventsMap[eventID]
		if !exists {
			event = &FlashSaleEventDTO{
				ID:        eventID,
				Name:      eventName,
				StartTime: startTime,
				EndTime:   endTime,
				Status:    eventStatus,
				Items:     []FlashSaleItemDTO{},
			}
			eventsMap[eventID] = event
			orderedEventIDs = append(orderedEventIDs, eventID)
		}

		if itemID.Valid {
			availStock := initQty.Int32 - soldQty.Int32
			// In Optimized mode with Redis, read real-time available stock
			if h.rdb != nil {
				if rStock, rErr := h.rdb.Get(ctx, rediskeys.StockKey(itemID.String)).Int64(); rErr == nil {
					availStock = int32(rStock)
				}
			}
			if availStock < 0 {
				availStock = 0
			}

			event.Items = append(event.Items, FlashSaleItemDTO{
				ID:              itemID.String,
				EventID:         eventID,
				ProductID:       prodID.String,
				ProductName:     prodName.String,
				ProductDesc:     prodDesc.String,
				ProductImage:    prodImg.String,
				OriginalPrice:   origPrice.Int64,
				SalePrice:       salePrice.Int64,
				InitialQuantity: initQty.Int32,
				AvailableStock:  availStock,
				MaxPerUser:      maxUser.Int32,
			})
		}
	}

	result := make([]FlashSaleEventDTO, 0, len(orderedEventIDs))
	for _, id := range orderedEventIDs {
		result = append(result, *eventsMap[id])
	}

	writeJSON(w, http.StatusOK, result)
}

// GetItemDetail returns details and real-time stock for a single flash sale item.
func (h *CatalogHandler) GetItemDetail(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Correlation-ID")
	ctx := r.Context()

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		writeError(w, correlationID, domain.ValidationError("id", "ID sản phẩm không hợp lệ"))
		return
	}
	itemID := pathParts[len(pathParts)-1]

	var item FlashSaleItemDTO
	var soldQty int32

	err := h.db.QueryRowContext(ctx, `
		SELECT i.id, i.event_id, i.product_id, p.name, COALESCE(p.description, ''), COALESCE(p.image_url, ''),
		       p.original_price, i.sale_price, i.initial_quantity, i.sold_quantity, i.max_per_user
		FROM flash_sale_items i
		JOIN products p ON p.id = i.product_id
		WHERE i.id = $1`, itemID).Scan(
		&item.ID, &item.EventID, &item.ProductID, &item.ProductName, &item.ProductDesc, &item.ProductImage,
		&item.OriginalPrice, &item.SalePrice, &item.InitialQuantity, &soldQty, &item.MaxPerUser,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, correlationID, domain.NotFoundError("item_not_found", "Không tìm thấy sản phẩm"))
		return
	}
	if err != nil {
		writeError(w, correlationID, domain.ServiceUnavailableError())
		return
	}

	item.AvailableStock = item.InitialQuantity - soldQty
	if h.rdb != nil {
		if rStock, rErr := h.rdb.Get(ctx, rediskeys.StockKey(itemID)).Int64(); rErr == nil {
			item.AvailableStock = int32(rStock)
		}
	}
	if item.AvailableStock < 0 {
		item.AvailableStock = 0
	}

	writeJSON(w, http.StatusOK, item)
}

// GetOrderDetail returns order metadata and payment status for UI status polling.
func (h *CatalogHandler) GetOrderDetail(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Correlation-ID")
	ctx := r.Context()

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		writeError(w, correlationID, domain.ValidationError("id", "ID đơn hàng không hợp lệ"))
		return
	}
	orderID := pathParts[len(pathParts)-1]

	var dto OrderDetailDTO
	err := h.db.QueryRowContext(ctx, `
		SELECT o.id, o.user_id, o.status, o.total_amount, o.expires_at, o.created_at,
		       oi.flash_sale_item_id, p.name, COALESCE(p.image_url, ''), oi.quantity, oi.unit_price,
		       COALESCE(pm.status::text, 'pending'), COALESCE(pm.method::text, 'bank_transfer'), pm.transaction_ref
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		JOIN products p ON p.id = oi.product_id
		LEFT JOIN payments pm ON pm.order_id = o.id
		WHERE o.id = $1`, orderID).Scan(
		&dto.OrderID, &dto.UserID, &dto.Status, &dto.TotalAmount, &dto.ExpiresAt, &dto.CreatedAt,
		&dto.ItemID, &dto.ProductName, &dto.ProductImage, &dto.Quantity, &dto.UnitPrice,
		&dto.PaymentStatus, &dto.PaymentMethod, &dto.TransactionRef,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, correlationID, domain.NotFoundError("order_not_found", "Không tìm thấy đơn hàng"))
		return
	}
	if err != nil {
		writeError(w, correlationID, domain.ServiceUnavailableError())
		return
	}

	writeJSON(w, http.StatusOK, dto)
}
```

- [ ] **Step 4: Mount read handlers in `cmd/api-baseline/main.go` and `cmd/api-optimized/main.go`**

In `cmd/api-baseline/main.go`:
```go
	catalogHandler := httpapi.NewCatalogHandler(db, nil)
	mux.HandleFunc("GET /api/v1/flash-sale/events", catalogHandler.ListActiveEvents)
	mux.HandleFunc("GET /api/v1/flash-sale/items/", catalogHandler.GetItemDetail)
	mux.HandleFunc("GET /api/v1/orders/", catalogHandler.GetOrderDetail)
```

In `cmd/api-optimized/main.go`:
```go
	catalogHandler := httpapi.NewCatalogHandler(db, rdb)
	mux.HandleFunc("GET /api/v1/flash-sale/events", catalogHandler.ListActiveEvents)
	mux.HandleFunc("GET /api/v1/flash-sale/items/", catalogHandler.GetItemDetail)
	mux.HandleFunc("GET /api/v1/orders/", catalogHandler.GetOrderDetail)
```

- [ ] **Step 5: Run tests and verify PASS**

Run: `go test -v ./internal/httpapi -run TestCatalogHandler_`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/catalog_handler.go internal/httpapi/catalog_handler_test.go cmd/api-baseline/main.go cmd/api-optimized/main.go
git commit -m "feat(stage-8): implement backend read APIs for events, items, and order polling"
```

---

### Task 2: Design System Tokens, Theme Provider & Animated UI Primitives

**Files:**
- Modify: `web/src/app/globals.css`
- Create: `web/src/components/ui/ThemeToggle.tsx`
- Create: `web/src/components/ui/AuroraHero.tsx`
- Create: `web/src/components/ui/SpotlightCard.tsx`

**Interfaces:**
- Consumes: Tailwind CSS, CSS Custom Variables, React 19
- Produces:
  - Theme switching (dark/light) via `ThemeToggle`
  - Zero-JS GPU-accelerated `AuroraHero` banner with reduced motion safety
  - Zero-rerender cursor-driven `SpotlightCard`

- [ ] **Step 1: Update `web/src/app/globals.css` with dark/light variables and keyframe animations**

Update `web/src/app/globals.css`:
```css
@tailwind base;
@tailwind components;
@tailwind utilities;

:root {
  --background: #090d16;
  --foreground: #f1f5f9;
  --card-bg: rgba(15, 23, 42, 0.75);
  --card-border: rgba(51, 65, 85, 0.6);
  --primary: #3b82f6;
  --primary-hover: #2563eb;
  --accent: #8b5cf6;
  --danger: #ef4444;
  --success: #10b981;
}

[data-theme='light'] {
  --background: #f8fafc;
  --foreground: #0f172a;
  --card-bg: rgba(255, 255, 255, 0.85);
  --card-border: rgba(203, 213, 225, 0.8);
  --primary: #2563eb;
  --primary-hover: #1d4ed8;
  --accent: #7c3aed;
  --danger: #dc2626;
  --success: #059669;
}

body {
  color: var(--foreground);
  background: var(--background);
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
  overflow-x: hidden;
}

/* Aurora background gradient animation */
@keyframes aurora {
  0% { transform: translate(0, 0) scale(1); opacity: 0.45; }
  50% { transform: translate(-30px, 20px) scale(1.1); opacity: 0.65; }
  100% { transform: translate(0, 0) scale(1); opacity: 0.45; }
}

.aurora-bg {
  background: radial-gradient(circle at 20% 30%, rgba(59, 130, 246, 0.35) 0%, transparent 50%),
              radial-gradient(circle at 80% 20%, rgba(139, 92, 246, 0.3) 0%, transparent 45%),
              radial-gradient(circle at 50% 80%, rgba(16, 185, 129, 0.25) 0%, transparent 50%);
  filter: blur(50px);
  animation: aurora 12s ease-in-out infinite;
}

/* Spinning border animation for celebration / active items */
@keyframes spin-border {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}

.spin-border-wrapper {
  position: relative;
  overflow: hidden;
  border-radius: 0.75rem;
}

.spin-border-wrapper::before {
  content: '';
  position: absolute;
  top: -50%;
  left: -50%;
  width: 200%;
  height: 200%;
  background: conic-gradient(transparent, transparent, var(--primary), var(--accent));
  animation: spin-border 4s linear infinite;
  z-index: 0;
}

.spin-border-inner {
  position: relative;
  z-index: 1;
  background: var(--card-bg);
  border-radius: calc(0.75rem - 1px);
}

/* Accessibility: prefers-reduced-motion overrides */
@media (prefers-reduced-motion: reduce) {
  .aurora-bg {
    animation: none !important;
    filter: blur(30px);
    opacity: 0.4;
  }
  .spin-border-wrapper::before {
    animation: none !important;
    background: var(--card-border);
  }
  * {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
```

- [ ] **Step 2: Implement `web/src/components/ui/ThemeToggle.tsx`**

Create `web/src/components/ui/ThemeToggle.tsx`:
```tsx
'use client';

import React, { useEffect, useState } from 'react';

export default function ThemeToggle() {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark');

  useEffect(() => {
    const saved = localStorage.getItem('theme') as 'dark' | 'light' | null;
    if (saved) {
      setTheme(saved);
      document.documentElement.setAttribute('data-theme', saved);
    } else {
      document.documentElement.setAttribute('data-theme', 'dark');
    }
  }, []);

  const toggleTheme = () => {
    const next = theme === 'dark' ? 'light' : 'dark';
    setTheme(next);
    localStorage.setItem('theme', next);
    document.documentElement.setAttribute('data-theme', next);
  };

  return (
    <button
      onClick={toggleTheme}
      aria-label="Toggle dark/light mode"
      className="p-2 rounded-lg border border-[var(--card-border)] bg-[var(--card-bg)] hover:opacity-80 transition-opacity focus:outline-none focus:ring-2 focus:ring-[var(--primary)]"
    >
      {theme === 'dark' ? '☀️' : '🌙'}
    </button>
  );
}
```

- [ ] **Step 3: Implement `web/src/components/ui/AuroraHero.tsx`**

Create `web/src/components/ui/AuroraHero.tsx`:
```tsx
'use client';

import React from 'react';

interface AuroraHeroProps {
  title: string;
  subtitle: string;
  badge?: string;
}

export default function AuroraHero({ title, subtitle, badge }: AuroraHeroProps) {
  return (
    <div className="relative overflow-hidden py-16 px-6 sm:px-12 rounded-2xl mb-12 border border-[var(--card-border)] bg-[var(--card-bg)]">
      <div className="absolute inset-0 aurora-bg -z-10 pointer-events-none" />
      <div className="max-w-3xl mx-auto text-center">
        {badge && (
          <span className="inline-block px-3 py-1 mb-4 text-xs font-semibold tracking-wider uppercase rounded-full bg-blue-500/10 text-blue-400 border border-blue-500/20">
            {badge}
          </span>
        )}
        <h1 className="text-4xl sm:text-5xl font-extrabold tracking-tight text-[var(--foreground)] mb-4">
          {title}
        </h1>
        <p className="text-lg text-slate-300 max-w-2xl mx-auto">
          {subtitle}
        </p>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Implement `web/src/components/ui/SpotlightCard.tsx` (Zero-Rerender CSS Variable Driven)**

Create `web/src/components/ui/SpotlightCard.tsx`:
```tsx
'use client';

import React, { useRef } from 'react';

interface SpotlightCardProps extends React.HTMLAttributes<HTMLDivElement> {
  children: React.ReactNode;
  className?: string;
}

export default function SpotlightCard({ children, className = '', ...props }: SpotlightCardProps) {
  const cardRef = useRef<HTMLDivElement>(null);

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!cardRef.current) return;
    const rect = cardRef.current.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;
    // Set directly on the DOM element style, causing 0 React component re-renders
    cardRef.current.style.setProperty('--mouse-x', `${x}px`);
    cardRef.current.style.setProperty('--mouse-y', `${y}px`);
  };

  return (
    <div
      ref={cardRef}
      onMouseMove={handleMouseMove}
      className={`relative overflow-hidden rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] p-6 transition-all duration-200 hover:border-slate-400/50 group ${className}`}
      {...props}
    >
      <div
        className="pointer-events-none absolute -inset-px rounded-xl opacity-0 transition-opacity duration-300 group-hover:opacity-100 -z-0"
        style={{
          background: `radial-gradient(400px circle at var(--mouse-x, -400px) var(--mouse-y, -400px), rgba(59, 130, 246, 0.12), transparent 80%)`,
        }}
      />
      <div className="relative z-10">{children}</div>
    </div>
  );
}
```

- [ ] **Step 5: Commit**

```bash
git add web/src/app/globals.css web/src/components/ui/
git commit -m "feat(stage-8): implement design system tokens, aurora hero, and spotlight card"
```

---

### Task 3: API Client, Deterministic Idempotency Store & Polling Hook

**Files:**
- Create: `web/src/lib/api.ts`
- Create: `web/src/hooks/usePolling.ts`

**Interfaces:**
- Consumes: `fetch`, browser `localStorage`, `document.visibilityState`
- Produces:
  - `ApiClient`: typed methods (`getEvents`, `getItem`, `createOrder`, `getOrderDetail`, `initiatePayment`, `simulatePayment`)
  - `getOrCreateIdempotencyKey(itemId, userId)`: stable idempotency key generator
  - `usePolling<T>(fetcher, intervalMs, isTerminal)`: tab-aware, terminal-aware polling hook

- [ ] **Step 1: Implement `web/src/lib/api.ts`**

Create `web/src/lib/api.ts`:
```typescript
export type Architecture = 'baseline' | 'optimized';

export interface FlashSaleItem {
  id: string;
  event_id: string;
  product_id: string;
  product_name: string;
  product_description: string;
  product_image: string;
  original_price: number;
  sale_price: number;
  initial_quantity: number;
  available_stock: number;
  max_per_user: number;
}

export interface FlashSaleEvent {
  id: string;
  name: string;
  start_time: string;
  end_time: string;
  status: string;
  items: FlashSaleItem[];
}

export interface CreateOrderRequest {
  user_id: string;
  flash_sale_item_id: string;
  quantity: 1;
  idempotency_key: string;
}

export interface CreateOrderResponse {
  order_id: string;
  reservation_id?: string;
  user_id: string;
  flash_sale_item_id: string;
  quantity: number;
  total_amount: number;
  status: string;
  payment_status: string;
  expires_at: string;
  created_at: string;
  idempotent?: boolean;
}

export interface OrderDetail {
  order_id: string;
  user_id: string;
  status: 'pending_payment' | 'paid' | 'payment_failed' | 'expired' | 'cancelled';
  total_amount: number;
  expires_at: string;
  created_at: string;
  item_id: string;
  product_name: string;
  product_image: string;
  quantity: number;
  unit_price: number;
  payment_status: 'pending' | 'success' | 'failed' | 'refund';
  payment_method: string;
  transaction_ref?: string;
}

export interface InitiatePaymentResponse {
  order_id: string;
  qr_url: string;
  account_number: string;
  bank_code: string;
  amount: number;
  transfer_content: string;
}

export interface PaymentResultResponse {
  order_id: string;
  order_status: string;
  payment_status: string;
  idempotent_replay: boolean;
}

export function getBaseURL(arch: Architecture): string {
  if (typeof window !== 'undefined') {
    // Client-side: route through local dev ports
    return arch === 'baseline' ? 'http://localhost:8081' : 'http://localhost:8082';
  }
  return arch === 'baseline' ? 'http://api-baseline:8081' : 'http://nginx:8082';
}

// In-memory stable idempotency cache per checkout attempt
const idempotencyKeyCache = new Map<string, string>();

export function getOrCreateIdempotencyKey(itemId: string, userId: string): string {
  const cacheKey = `${userId}:${itemId}`;
  let key = idempotencyKeyCache.get(cacheKey);
  if (!key) {
    key = `checkout-${Date.now()}-${Math.random().toString(36).substring(2, 9)}`;
    idempotencyKeyCache.set(cacheKey, key);
  }
  return key;
}

export function clearIdempotencyKey(itemId: string, userId: string) {
  idempotencyKeyCache.delete(`${userId}:${itemId}`);
}

export const api = {
  async getEvents(arch: Architecture = 'optimized'): Promise<FlashSaleEvent[]> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/flash-sale/events`, { cache: 'no-store' });
    if (!res.ok) throw new Error(`Failed to fetch events: ${res.statusText}`);
    return res.json();
  },

  async getItemDetail(id: string, arch: Architecture = 'optimized'): Promise<FlashSaleItem> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/flash-sale/items/${id}`, { cache: 'no-store' });
    if (!res.ok) throw new Error(`Failed to fetch item detail: ${res.statusText}`);
    return res.json();
  },

  async createOrder(req: CreateOrderRequest, arch: Architecture): Promise<CreateOrderResponse> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/flash-sale/orders`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Idempotency-Key': req.idempotency_key,
        'X-User-ID': req.user_id,
      },
      body: JSON.stringify(req),
    });
    const data = await res.json();
    if (!res.ok) {
      const errorMsg = data?.message || data?.error || 'Tạo đơn hàng thất bại';
      throw new Error(errorMsg);
    }
    return data;
  },

  async getOrderDetail(orderId: string, arch: Architecture): Promise<OrderDetail> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/orders/${orderId}`, { cache: 'no-store' });
    if (!res.ok) throw new Error(`Failed to fetch order: ${res.statusText}`);
    return res.json();
  },

  async initiatePayment(orderId: string, arch: Architecture): Promise<InitiatePaymentResponse> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/payments/sepay/initiate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ order_id: orderId }),
    });
    if (!res.ok) throw new Error('Không thể tạo mã thanh toán SePay');
    return res.json();
  },

  async simulatePayment(orderId: string, outcome: string, arch: Architecture): Promise<PaymentResultResponse> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/payments/simulate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ order_id: orderId, outcome }),
    });
    if (!res.ok) throw new Error('Mô phỏng thanh toán thất bại');
    return res.json();
  },
};
```

- [ ] **Step 2: Implement `web/src/hooks/usePolling.ts`**

Create `web/src/hooks/usePolling.ts`:
```typescript
'use client';

import { useEffect, useRef, useState, useCallback } from 'react';

interface UsePollingOptions<T> {
  fetcher: () => Promise<T>;
  intervalMs?: number;
  isTerminal?: (data: T) => boolean;
  enabled?: boolean;
}

export function usePolling<T>({
  fetcher,
  intervalMs = 1500,
  isTerminal,
  enabled = true,
}: UsePollingOptions<T>) {
  const [data, setData] = useState<T | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isError, setIsError] = useState<boolean>(false);
  const [error, setError] = useState<Error | null>(null);
  const [isStale, setIsStale] = useState<boolean>(false);

  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  const isTerminalRef = useRef(isTerminal);
  isTerminalRef.current = isTerminal;

  const poll = useCallback(async () => {
    try {
      const result = await fetcherRef.current();
      setData(result);
      setIsError(false);
      setError(null);
      setIsStale(false);
      setIsLoading(false);

      if (isTerminalRef.current && isTerminalRef.current(result)) {
        return false; // Stop polling
      }
      return true; // Continue polling
    } catch (err: any) {
      setIsError(true);
      setError(err);
      setIsStale(true);
      setIsLoading(false);
      return true; // Continue polling on transient errors
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;

    let active = true;
    let timerId: NodeJS.Timeout;

    const execute = async () => {
      // Pause polling if tab is backgrounded
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
        timerId = setTimeout(execute, intervalMs);
        return;
      }

      const shouldContinue = await poll();
      if (active && shouldContinue) {
        timerId = setTimeout(execute, intervalMs);
      }
    };

    execute();

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        poll();
      }
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      active = false;
      clearTimeout(timerId);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  }, [enabled, intervalMs, poll]);

  return { data, isLoading, isError, error, isStale, refetch: poll };
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/api.ts web/src/hooks/usePolling.ts
git commit -m "feat(stage-8): implement typed API client, idempotency store, and visibility-aware polling hook"
```

---

### Task 4: Product Flow Pages (Home, Detail & Orders)

**Files:**
- Modify: `web/src/app/page.tsx`
- Create: `web/src/app/flash-sale/[id]/page.tsx`
- Create: `web/src/app/orders/[id]/page.tsx`

**Interfaces:**
- Consumes: `web/src/lib/api.ts`, `web/src/hooks/usePolling.ts`, `web/src/components/ui/*`
- Produces: Complete navigable user product journey from catalog browsing to flash sale ordering and payment status verification.

- [ ] **Step 1: Implement home catalog page in `web/src/app/page.tsx`**

Modify `web/src/app/page.tsx`:
```tsx
'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import AuroraHero from '@/components/ui/AuroraHero';
import SpotlightCard from '@/components/ui/SpotlightCard';
import ThemeToggle from '@/components/ui/ThemeToggle';
import { api, FlashSaleEvent, Architecture } from '@/lib/api';

export default function HomePage() {
  const [arch, setArch] = useState<Architecture>('optimized');
  const [events, setEvents] = useState<FlashSaleEvent[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    api.getEvents(arch)
      .then((data) => {
        setEvents(data);
        setError(null);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [arch]);

  return (
    <main className="min-h-screen p-6 sm:p-12 max-w-6xl mx-auto">
      {/* Top Header */}
      <header className="flex justify-between items-center mb-8">
        <div className="flex items-center space-x-3">
          <div className="w-8 h-8 rounded-lg bg-blue-600 flex items-center justify-center font-bold text-white">⚡</div>
          <span className="font-extrabold text-xl tracking-tight">ApexStock Flash Sale</span>
        </div>
        <div className="flex items-center space-x-4">
          <Link
            href="/war-room"
            className="px-3 py-1.5 text-xs font-semibold rounded-lg bg-purple-500/20 text-purple-300 border border-purple-500/30 hover:bg-purple-500/30 transition-colors"
          >
            War Room Dashboard 📊
          </Link>
          <ThemeToggle />
        </div>
      </header>

      {/* Aurora Banner */}
      <AuroraHero
        badge="Flash Sale Săn Deal Chớp Nhoáng"
        title="Siêu Giảm Giá Công Nghệ 2026"
        subtitle="Hệ thống bán hàng chớp nhoáng với kiến trúc chịu tải phân tán, đảm bảo tuyệt đối không oversell."
      />

      {/* Architecture Toggle Bar */}
      <div className="flex items-center justify-between mb-8 p-4 rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)]">
        <div>
          <span className="text-sm font-semibold text-slate-300">Cổng kết nối kiểm thử:</span>
          <p className="text-xs text-slate-400">Chọn kiến trúc backend phục vụ request</p>
        </div>
        <div className="flex space-x-2">
          <button
            onClick={() => setArch('baseline')}
            className={`px-3 py-1.5 rounded-lg text-xs font-bold transition-all ${
              arch === 'baseline'
                ? 'bg-amber-600 text-white shadow-md'
                : 'bg-slate-800 text-slate-400 hover:text-white'
            }`}
          >
            Baseline (:8081)
          </button>
          <button
            onClick={() => setArch('optimized')}
            className={`px-3 py-1.5 rounded-lg text-xs font-bold transition-all ${
              arch === 'optimized'
                ? 'bg-blue-600 text-white shadow-md'
                : 'bg-slate-800 text-slate-400 hover:text-white'
            }`}
          >
            Optimized (:8082)
          </button>
        </div>
      </div>

      {/* Products Grid */}
      {loading && (
        <div className="text-center py-16 text-slate-400">Đang tải danh sách sự kiện flash sale...</div>
      )}

      {error && (
        <div className="p-4 rounded-xl bg-red-500/10 border border-red-500/30 text-red-400 text-sm mb-6">
          {error}
        </div>
      )}

      {!loading && events.length === 0 && !error && (
        <div className="text-center py-16 text-slate-500">Chưa có sự kiện flash sale nào đang mở.</div>
      )}

      <div className="space-y-12">
        {events.map((event) => (
          <section key={event.id}>
            <div className="flex justify-between items-center mb-6">
              <h2 className="text-2xl font-bold tracking-tight text-[var(--foreground)]">{event.name}</h2>
              <span className="text-xs px-2.5 py-1 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                {event.status === 'active' ? 'Đang Diễn Ra' : 'Sắp Diễn Ra'}
              </span>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
              {event.items.map((item) => {
                const discount = Math.round(((item.original_price - item.sale_price) / item.original_price) * 100);
                const isSoldOut = item.available_stock <= 0;

                return (
                  <SpotlightCard key={item.id} className="flex flex-col justify-between">
                    <div>
                      <div className="relative aspect-video rounded-lg overflow-hidden bg-slate-800 mb-4 flex items-center justify-center text-4xl">
                        {item.product_image ? (
                          <img src={item.product_image} alt={item.product_name} className="object-cover w-full h-full" />
                        ) : (
                          '📱'
                        )}
                        {discount > 0 && (
                          <span className="absolute top-2 right-2 bg-red-600 text-white text-xs font-black px-2 py-0.5 rounded">
                            -{discount}%
                          </span>
                        )}
                      </div>

                      <h3 className="font-bold text-lg text-[var(--foreground)] mb-1">{item.product_name}</h3>
                      <p className="text-xs text-slate-400 line-clamp-2 mb-4">{item.product_description}</p>

                      <div className="flex items-baseline space-x-2 mb-4">
                        <span className="text-xl font-extrabold text-blue-400">
                          {item.sale_price.toLocaleString('vi-VN')} đ
                        </span>
                        <span className="text-xs text-slate-500 line-through">
                          {item.original_price.toLocaleString('vi-VN')} đ
                        </span>
                      </div>

                      {/* Stock Bar */}
                      <div className="w-full bg-slate-800 rounded-full h-2 mb-2 overflow-hidden">
                        <div
                          className={`h-2 rounded-full transition-all duration-500 ${
                            isSoldOut ? 'bg-red-600' : 'bg-emerald-500'
                          }`}
                          style={{
                            width: `${Math.min(100, (item.available_stock / item.initial_quantity) * 100)}%`,
                          }}
                        />
                      </div>
                      <div className="flex justify-between text-xs text-slate-400 mb-6">
                        <span>Kho còn: <strong className="text-white">{item.available_stock}</strong></span>
                        <span>Tổng mở bán: {item.initial_quantity}</span>
                      </div>
                    </div>

                    <Link
                      href={`/flash-sale/${item.id}?arch=${arch}`}
                      className={`w-full py-2.5 rounded-lg text-center font-bold text-sm transition-all block ${
                        isSoldOut
                          ? 'bg-slate-800 text-slate-500 cursor-not-allowed pointer-events-none'
                          : 'bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-500/25'
                      }`}
                    >
                      {isSoldOut ? 'Đã Hết Hàng' : 'Săn Ngay ⚡'}
                    </Link>
                  </SpotlightCard>
                );
              })}
            </div>
          </section>
        ))}
      </div>
    </main>
  );
}
```

- [ ] **Step 2: Implement item detail and flash order page in `web/src/app/flash-sale/[id]/page.tsx`**

Create `web/src/app/flash-sale/[id]/page.tsx`:
```tsx
'use client';

import React, { useEffect, useState } from 'react';
import { useParams, useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { api, FlashSaleItem, Architecture, getOrCreateIdempotencyKey, clearIdempotencyKey } from '@/lib/api';
import SpotlightCard from '@/components/ui/SpotlightCard';

export default function FlashSaleDetailPage() {
  const params = useParams();
  const searchParams = useSearchParams();
  const router = useRouter();

  const id = params?.id as string;
  const initialArch = (searchParams.get('arch') as Architecture) || 'optimized';

  const [arch, setArch] = useState<Architecture>(initialArch);
  const [item, setItem] = useState<FlashSaleItem | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [purchasing, setPurchasing] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  // Mock demo user
  const userId = '00000000-0000-0000-0000-000000000001';

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    api.getItemDetail(id, arch)
      .then((data) => {
        setItem(data);
        setError(null);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [id, arch]);

  const handleFlashBuy = async () => {
    if (!item || purchasing) return;
    setPurchasing(true);
    setError(null);

    const idempotencyKey = getOrCreateIdempotencyKey(item.id, userId);

    try {
      const order = await api.createOrder({
        user_id: userId,
        flash_sale_item_id: item.id,
        quantity: 1,
        idempotency_key: idempotencyKey,
      }, arch);

      clearIdempotencyKey(item.id, userId);
      router.push(`/orders/${order.order_id}?arch=${arch}`);
    } catch (err: any) {
      setError(err.message || 'Lỗi đặt hàng');
      setPurchasing(false);
    }
  };

  if (loading) {
    return <div className="min-h-screen flex items-center justify-center text-slate-400">Đang tải sản phẩm...</div>;
  }

  if (!item) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center text-center p-6">
        <p className="text-red-400 mb-4">{error || 'Không tìm thấy sản phẩm'}</p>
        <Link href="/" className="px-4 py-2 rounded-lg bg-slate-800 text-white">Quay lại trang chủ</Link>
      </div>
    );
  }

  const isSoldOut = item.available_stock <= 0;

  return (
    <main className="min-h-screen p-6 sm:p-12 max-w-4xl mx-auto">
      <Link href="/" className="inline-flex items-center text-sm text-slate-400 hover:text-white mb-8">
        ← Quay lại danh sách
      </Link>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
        {/* Product Media */}
        <div className="aspect-square rounded-2xl bg-slate-800/80 border border-[var(--card-border)] flex items-center justify-center text-7xl overflow-hidden">
          {item.product_image ? (
            <img src={item.product_image} alt={item.product_name} className="object-cover w-full h-full" />
          ) : (
            '⚡'
          )}
        </div>

        {/* Product Actions & Checkout */}
        <SpotlightCard className="flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between mb-4">
              <span className="text-xs px-2.5 py-1 rounded-full bg-blue-500/10 text-blue-400 border border-blue-500/20">
                Flash Sale Item
              </span>
              <div className="flex items-center space-x-1.5 text-xs">
                <span className="text-slate-400">Tuyến xử lý:</span>
                <span className="font-bold text-white uppercase">{arch}</span>
              </div>
            </div>

            <h1 className="text-3xl font-extrabold text-[var(--foreground)] mb-2">{item.product_name}</h1>
            <p className="text-sm text-slate-400 mb-6">{item.product_description}</p>

            <div className="flex items-baseline space-x-3 mb-6">
              <span className="text-3xl font-black text-blue-400">
                {item.sale_price.toLocaleString('vi-VN')} đ
              </span>
              <span className="text-sm text-slate-500 line-through">
                {item.original_price.toLocaleString('vi-VN')} đ
              </span>
            </div>

            <div className="p-4 rounded-xl bg-slate-900/60 border border-[var(--card-border)] mb-6 space-y-2 text-sm">
              <div className="flex justify-between text-slate-300">
                <span>Số lượng mua:</span>
                <span className="font-bold">1 sản phẩm (giới hạn tối đa)</span>
              </div>
              <div className="flex justify-between text-slate-300">
                <span>Kho khả dụng:</span>
                <span className={`font-bold ${isSoldOut ? 'text-red-400' : 'text-emerald-400'}`}>
                  {item.available_stock} sản phẩm
                </span>
              </div>
            </div>

            {error && (
              <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-red-400 text-xs mb-6">
                {error}
              </div>
            )}
          </div>

          <button
            onClick={handleFlashBuy}
            disabled={isSoldOut || purchasing}
            className={`w-full py-4 rounded-xl font-extrabold text-base transition-all flex items-center justify-center space-x-2 ${
              isSoldOut
                ? 'bg-slate-800 text-slate-500 cursor-not-allowed'
                : purchasing
                ? 'bg-blue-600/50 text-white cursor-wait'
                : 'bg-blue-600 hover:bg-blue-500 text-white shadow-xl shadow-blue-500/25 active:scale-95'
            }`}
          >
            {purchasing ? (
              <span>Đang tạo đơn hàng...</span>
            ) : isSoldOut ? (
              <span>Đã Hết Hàng</span>
            ) : (
              <span>MUA NGAY (FLASH BUY) ⚡</span>
            )}
          </button>
        </SpotlightCard>
      </div>
    </main>
  );
}
```

- [ ] **Step 3: Implement order detail with payment QR and status polling in `web/src/app/orders/[id]/page.tsx`**

Create `web/src/app/orders/[id]/page.tsx`:
```tsx
'use client';

import React, { useEffect, useState } from 'react';
import { useParams, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { api, OrderDetail, Architecture, InitiatePaymentResponse } from '@/lib/api';
import { usePolling } from '@/hooks/usePolling';
import SpotlightCard from '@/components/ui/SpotlightCard';

export default function OrderDetailPage() {
  const params = useParams();
  const searchParams = useSearchParams();

  const orderId = params?.id as string;
  const arch = (searchParams.get('arch') as Architecture) || 'optimized';

  const [paymentInfo, setPaymentInfo] = useState<InitiatePaymentResponse | null>(null);
  const [simulating, setSimulating] = useState<boolean>(false);

  // Poll order detail until reaching a terminal state
  const { data: order, isLoading, isStale } = usePolling<OrderDetail>({
    fetcher: () => api.getOrderDetail(orderId, arch),
    intervalMs: 1500,
    isTerminal: (ord) => ord.status !== 'pending_payment',
    enabled: Boolean(orderId),
  });

  // Fetch SePay QR details once order is loaded
  useEffect(() => {
    if (order && order.status === 'pending_payment' && !paymentInfo) {
      api.initiatePayment(order.order_id, arch)
        .then(setPaymentInfo)
        .catch(() => {});
    }
  }, [order, arch, paymentInfo]);

  const handleSimulate = async (outcome: 'success' | 'failure' | 'timeout' | 'cancel') => {
    if (!order || simulating) return;
    setSimulating(true);
    try {
      await api.simulatePayment(order.order_id, outcome, arch);
    } catch (err: any) {
      alert(err.message || 'Lỗi mô phỏng thanh toán');
    } finally {
      setSimulating(false);
    }
  };

  if (isLoading && !order) {
    return <div className="min-h-screen flex items-center justify-center text-slate-400">Đang tải đơn hàng...</div>;
  }

  if (!order) {
    return <div className="min-h-screen flex items-center justify-center text-red-400">Không tìm thấy đơn hàng.</div>;
  }

  const isPending = order.status === 'pending_payment';
  const isPaid = order.status === 'paid';
  const isExpired = order.status === 'expired';

  return (
    <main className="min-h-screen p-6 sm:p-12 max-w-3xl mx-auto">
      <Link href="/" className="inline-flex items-center text-sm text-slate-400 hover:text-white mb-8">
        ← Về trang chủ
      </Link>

      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-black text-[var(--foreground)]">Đơn Hàng #{order.order_id.slice(0, 8)}</h1>
          <p className="text-xs text-slate-400">Tạo lúc: {new Date(order.created_at).toLocaleString('vi-VN')}</p>
        </div>
        <div className="flex items-center space-x-2">
          {isStale && <span className="text-xs px-2 py-1 rounded bg-yellow-500/20 text-yellow-300">Đang kết nối lại...</span>}
          <span
            className={`px-3 py-1 text-xs font-bold rounded-full ${
              isPaid
                ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                : isPending
                ? 'bg-amber-500/20 text-amber-400 border border-amber-500/30 animate-pulse'
                : 'bg-red-500/20 text-red-400 border border-red-500/30'
            }`}
          >
            {order.status.toUpperCase()}
          </span>
        </div>
      </div>

      <SpotlightCard className="mb-8">
        <h2 className="text-lg font-bold mb-4">Chi Tiết Sản Phẩm</h2>
        <div className="flex justify-between items-center py-2 border-b border-[var(--card-border)]">
          <span>{order.product_name} x {order.quantity}</span>
          <span className="font-bold">{order.unit_price.toLocaleString('vi-VN')} đ</span>
        </div>
        <div className="flex justify-between items-center pt-4 text-lg font-extrabold">
          <span>Tổng thanh toán:</span>
          <span className="text-blue-400">{order.total_amount.toLocaleString('vi-VN')} đ</span>
        </div>
      </SpotlightCard>

      {/* Payment Action Section */}
      {isPending && paymentInfo && (
        <SpotlightCard className="mb-8 text-center">
          <h2 className="text-lg font-bold mb-2">Quét Mã VietQR Để Thanh Toán</h2>
          <p className="text-xs text-slate-400 mb-6">Mã QR tự động đối soát qua SePay Webhook</p>

          <div className="inline-block p-4 bg-white rounded-2xl shadow-xl mb-6">
            <img src={paymentInfo.qr_url} alt="VietQR SePay" className="w-56 h-56 object-contain" />
          </div>

          <div className="max-w-md mx-auto text-left p-4 rounded-xl bg-slate-900/60 border border-[var(--card-border)] space-y-2 text-xs mb-8">
            <div className="flex justify-between">
              <span className="text-slate-400">Ngân hàng:</span>
              <span className="font-bold">{paymentInfo.bank_code}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Số tài khoản:</span>
              <span className="font-bold">{paymentInfo.account_number}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Nội dung CK:</span>
              <span className="font-bold text-amber-400">{paymentInfo.transfer_content}</span>
            </div>
          </div>

          {/* Payment Simulator Panel for Quick Demo */}
          <div className="pt-4 border-t border-[var(--card-border)]">
            <span className="text-xs font-semibold text-slate-400 block mb-3">
              Mô Phỏng Nhanh (Simulator Không Cần Quét Ngân Hàng Thật):
            </span>
            <div className="flex flex-wrap justify-center gap-2">
              <button
                onClick={() => handleSimulate('success')}
                disabled={simulating}
                className="px-3 py-1.5 rounded-lg text-xs font-bold bg-emerald-600 hover:bg-emerald-500 text-white"
              >
                Mô Phỏng Thành Công ✅
              </button>
              <button
                onClick={() => handleSimulate('failure')}
                disabled={simulating}
                className="px-3 py-1.5 rounded-lg text-xs font-bold bg-red-600 hover:bg-red-500 text-white"
              >
                Mô Phỏng Thất Bại ❌
              </button>
              <button
                onClick={() => handleSimulate('cancel')}
                disabled={simulating}
                className="px-3 py-1.5 rounded-lg text-xs font-bold bg-slate-700 hover:bg-slate-600 text-slate-200"
              >
                Hủy Đơn 🚫
              </button>
            </div>
          </div>
        </SpotlightCard>
      )}

      {isPaid && (
        <div className="p-8 rounded-2xl bg-emerald-500/10 border border-emerald-500/30 text-center">
          <div className="text-5xl mb-4">🎉</div>
          <h2 className="text-2xl font-bold text-emerald-400 mb-2">Thanh Toán Thành Công!</h2>
          <p className="text-sm text-slate-300">Đơn hàng của bạn đã được ghi nhận và xác nhận hoàn tất.</p>
        </div>
      )}

      {isExpired && (
        <div className="p-8 rounded-2xl bg-red-500/10 border border-red-500/30 text-center">
          <div className="text-5xl mb-4">⏰</div>
          <h2 className="text-2xl font-bold text-red-400 mb-2">Đơn Hàng Đã Hết Hạn</h2>
          <p className="text-sm text-slate-300">Hết thời gian giữ hàng 5 phút. Kho đã được hoàn lại tự động.</p>
        </div>
      )}
    </main>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add web/src/app/page.tsx web/src/app/flash-sale/ web/src/app/orders/
git commit -m "feat(stage-8): implement home catalog, item flash checkout, and order payment detail pages"
```

---

### Task 5: Frontend Lint, Typecheck & Build Verification

**Files:**
- Verification: `web/package.json`, `web/tsconfig.json`

**Interfaces:**
- Consumes: Node.js 20+, npm, Next.js build compiler
- Produces: Verified production frontend bundle without TypeScript or ESLint errors

- [ ] **Step 1: Run typecheck and linting on the web project**

Run:
```bash
cd web && npm run lint
```
Expected: Exit code 0, 0 lint errors.

- [ ] **Step 2: Run production Next.js build**

Run:
```bash
cd web && npm run build
```
Expected:
```
✓ Compiled successfully
✓ Generating static pages
✓ Finalizing page optimization
Exit code: 0
```

- [ ] **Step 3: Commit**

```bash
git commit --allow-empty -m "build(stage-8): verify next.js production build and type checking pass"
```
