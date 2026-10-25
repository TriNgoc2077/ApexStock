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
