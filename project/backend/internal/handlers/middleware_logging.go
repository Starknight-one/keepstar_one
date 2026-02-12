package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// LoggingMiddleware creates a middleware that:
// 1. Assigns a UUID request_id and sets X-Request-ID header
// 2. Attaches a SpanCollector to context for waterfall tracing
// 3. Logs the request to stdout (JSON)
// 4. Persists RequestLog to Postgres (async, fire-and-forget)
func LoggingMiddleware(log *logger.Logger, logAdapter *postgres.LogAdapter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := uuid.New().String()

			// SpanCollector for waterfall tracing
			sc := domain.NewSpanCollector()

			// Enrich context
			ctx := r.Context()
			ctx = logger.WithRequestID(ctx, requestID)
			ctx = domain.WithSpanCollector(ctx, sc)

			// Tenant from middleware (if already resolved)
			if tenant := GetTenantFromContext(ctx); tenant != nil {
				ctx = logger.WithTenantSlug(ctx, tenant.Slug)
			}

			w.Header().Set("X-Request-ID", requestID)
			wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

			// Span for the entire HTTP request
			endRequest := sc.Start("http")
			next.ServeHTTP(wrapped, r.WithContext(ctx))
			endRequest()

			duration := time.Since(start)

			// Re-read context values (handlers may have set session_id, tenant_slug)
			finalCtx := r.Context()
			// Use the ctx we built for logging since r.Context() won't have our additions
			reqLog := log.FromContext(ctx)

			// Log to stdout
			if wrapped.statusCode >= 400 {
				reqLog.Error("http_request", "method", r.Method, "path", r.URL.Path,
					"status", wrapped.statusCode, "duration_ms", duration.Milliseconds())
			} else if r.URL.Path == "/health" || r.URL.Path == "/ready" {
				reqLog.Debug("http_request", "method", r.Method, "path", r.URL.Path,
					"status", wrapped.statusCode, "duration_ms", duration.Milliseconds())
			} else {
				reqLog.Info("http_request", "method", r.Method, "path", r.URL.Path,
					"status", wrapped.statusCode, "duration_ms", duration.Milliseconds())
			}

			// Persist to Postgres (async, fire-and-forget)
			if logAdapter != nil {
				entry := &postgres.RequestLog{
					ID:         requestID,
					Service:    "chat",
					Method:     r.Method,
					Path:       r.URL.Path,
					Status:     wrapped.statusCode,
					DurationMs: duration.Milliseconds(),
					SessionID:  logger.SessionIDFrom(ctx),
					TenantSlug: logger.TenantSlugFrom(ctx),
				}
				spans := sc.Spans()
				if len(spans) > 0 {
					entry.Spans = spans
				}
				_ = finalCtx // suppress unused
				go logAdapter.RecordRequestLog(context.Background(), entry)
			}
		})
	}
}
