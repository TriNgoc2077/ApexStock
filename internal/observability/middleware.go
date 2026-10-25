package observability

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// HTTPMiddleware instruments requests with correlation ID, Prometheus metrics, and structured JSON logs.
func HTTPMiddleware(arch string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = NewCorrelationID()
		}

		ctx := WithCorrelationID(r.Context(), correlationID)
		w.Header().Set("X-Correlation-ID", correlationID)

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r.WithContext(ctx))

		duration := time.Since(start)
		route := normalizeRoute(r.URL.Path)
		statusStr := strconv.Itoa(rw.statusCode)

		// Record Prometheus metrics
		IncHTTPRequests(arch, route, r.Method, statusStr)
		ObserveHTTPRequestDuration(arch, route, duration.Seconds())

		// Emit structured JSON log
		slog.InfoContext(ctx, "http_request",
			slog.String("arch", arch),
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.Int("status", rw.statusCode),
			slog.Float64("duration_ms", float64(duration.Microseconds())/1000.0),
			slog.Int("bytes", rw.bytes),
			slog.String("correlation_id", correlationID),
			slog.String("client_ip", r.RemoteAddr),
		)
	})
}

func normalizeRoute(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/v1/flash-sale/orders"):
		return "/api/v1/flash-sale/orders"
	case strings.HasPrefix(path, "/api/v1/payments/simulate"):
		return "/api/v1/payments/simulate"
	case strings.HasPrefix(path, "/api/v1/payments/sepay/initiate"):
		return "/api/v1/payments/sepay/initiate"
	case strings.HasPrefix(path, "/api/v1/webhooks/sepay"):
		return "/api/v1/webhooks/sepay"
	case strings.HasPrefix(path, "/healthz"):
		return "/healthz"
	case strings.HasPrefix(path, "/readyz"):
		return "/readyz"
	case strings.HasPrefix(path, "/metrics"):
		return "/metrics"
	default:
		return path
	}
}

// InjectNATSCorrelationID sets X-Correlation-ID in NATS message header.
func InjectNATSCorrelationID(ctx context.Context, msg *nats.Msg) {
	if msg == nil {
		return
	}
	id := CorrelationIDFromContext(ctx)
	if id == "" {
		id = NewCorrelationID()
	}
	if msg.Header == nil {
		msg.Header = make(nats.Header)
	}
	msg.Header.Set("X-Correlation-ID", id)
}

// ExtractNATSCorrelationID retrieves X-Correlation-ID from NATS message header and wraps context.
func ExtractNATSCorrelationID(msg *nats.Msg) (string, context.Context) {
	var id string
	if msg != nil && msg.Header != nil {
		id = msg.Header.Get("X-Correlation-ID")
	}
	if id == "" {
		id = NewCorrelationID()
	}
	ctx := WithCorrelationID(context.Background(), id)
	return id, ctx
}
