// Package handlers exposes V5's HTTP layer: route setup, middleware, and
// per-endpoint handlers. Each handler depends on a small ports surface
// (StatePort, CatalogPort, Agent2Execute use case) — wiring lives in
// cmd/server/main.go.
package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"keepstar_v5/internal/domain"
)

type ctxKeyRequestID struct{}

// requestIDFromContext returns the per-request id stamped by WithLogging.
// Empty string when middleware wasn't applied (test-time direct calls).
func requestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRequestID{}).(string)
	return v
}

// WithLogging stamps each request with a fresh request_id, attaches it to
// the response header (`X-Request-ID`), logs the request line on receipt
// and the response line on completion (with status + latency). The
// request_id is also placed in context so downstream handlers can include
// it in their own log lines.
func WithLogging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := newRequestID()
			w.Header().Set("X-Request-ID", rid)

			// Stamp request id + a fresh SpanCollector so adapters and
			// use cases can emit spans without taking dep on context plumbing.
			sc := domain.NewSpanCollector()
			ctx := context.WithValue(r.Context(), ctxKeyRequestID{}, rid)
			ctx = domain.WithSpanCollector(ctx, sc)

			rw := &recordingWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()

			log.Info("http: request",
				"req_id", rid,
				"method", r.Method,
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
			)
			next.ServeHTTP(rw, r.WithContext(ctx))
			log.Info("http: response",
				"req_id", rid,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"latency_ms", time.Since(start).Milliseconds(),
				"span_count", len(sc.Spans()),
			)
		})
	}
}

// recordingWriter captures the response status code so logging middleware
// can include it in the response line. The standard ResponseWriter doesn't
// expose status after WriteHeader.
type recordingWriter struct {
	http.ResponseWriter
	status int
}

func (rw *recordingWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// newRequestID returns a fresh 16-hex-char request id (8 bytes of
// crypto/rand). Short enough to scan, long enough to be unique across a
// service lifetime.
func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
