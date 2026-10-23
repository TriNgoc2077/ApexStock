package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	_ "github.com/lib/pq"
)

// HealthzHandler checks container process is alive
func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ReadyzHandler checks dependencies (PostgreSQL) are ready
func ReadyzHandler(w http.ResponseWriter, r *http.Request, dsn string) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		respondNotReady(w, "database connection failed: "+err.Error())
		return
	}
	defer db.Close()

	db.SetConnMaxLifetime(5 * time.Second)
	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(1)

	ctx := r.Context()
	if err := db.PingContext(ctx); err != nil {
		respondNotReady(w, "database ping failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func respondNotReady(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{"status": "not_ready", "reason": reason})
}
