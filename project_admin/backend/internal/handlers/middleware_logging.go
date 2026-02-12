package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
)

// loggingResponseWriter wraps http.ResponseWriter to capture status code
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *loggingResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// LoggingMiddleware creates a middleware that assigns request_id, attaches SpanCollector,
// logs to stdout, and persists to Postgres (async)
func LoggingMiddleware(log *logger.Logger, logAdapter *postgres.LogAdapter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := uuid.New().String()

			sc := domain.NewSpanCollector()

			ctx := r.Context()
			ctx = logger.WithRequestID(ctx, requestID)
			ctx = domain.WithSpanCollector(ctx, sc)

			// Extract user_id and tenant_id from JWT context (set by AuthMiddleware)
			if uid := UserID(ctx); uid != "" {
				ctx = logger.WithUserID(ctx, uid)
			}
			if tid := TenantID(ctx); tid != "" {
				ctx = logger.WithTenantID(ctx, tid)
			}

			w.Header().Set("X-Request-ID", requestID)
			wrapped := &loggingResponseWriter{ResponseWriter: w, statusCode: 200}

			endRequest := sc.Start("http")
			next.ServeHTTP(wrapped, r.WithContext(ctx))
			endRequest()

			duration := time.Since(start)
			reqLog := log.FromContext(ctx)

			if wrapped.statusCode >= 400 {
				reqLog.Error("http_request", "method", r.Method, "path", r.URL.Path,
					"status", wrapped.statusCode, "duration_ms", duration.Milliseconds())
			} else if r.URL.Path == "/health" {
				reqLog.Debug("http_request", "method", r.Method, "path", r.URL.Path,
					"status", wrapped.statusCode, "duration_ms", duration.Milliseconds())
			} else {
				reqLog.Info("http_request", "method", r.Method, "path", r.URL.Path,
					"status", wrapped.statusCode, "duration_ms", duration.Milliseconds())
			}

			// Persist to Postgres (async)
			if logAdapter != nil {
				entry := &postgres.RequestLog{
					ID:         requestID,
					Service:    "admin",
					Method:     r.Method,
					Path:       r.URL.Path,
					Status:     wrapped.statusCode,
					DurationMs: duration.Milliseconds(),
					UserID:     logger.UserIDFrom(ctx),
					TenantSlug: logger.TenantIDFrom(ctx),
				}
				spans := sc.Spans()
				if len(spans) > 0 {
					entry.Spans = spans
				}
				go logAdapter.RecordRequestLog(context.Background(), entry)
			}
		})
	}
}
