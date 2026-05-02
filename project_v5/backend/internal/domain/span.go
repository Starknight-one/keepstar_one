package domain

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Span describes a single timed operation in V5's request waterfall.
// Times are millisecond offsets from the SpanCollector's anchor (request
// start), not absolute wall-clock — so a serialised span list stays
// stable across timezones and serves the /debug/traces UI cleanly.
type Span struct {
	Name       string `json:"name"`
	StartMs    int64  `json:"start_ms"`
	EndMs      int64  `json:"end_ms"`
	DurationMs int64  `json:"duration_ms"`
	Detail     string `json:"detail,omitempty"`
}

// SpanCollector accumulates Spans for one request. Concurrent-safe.
// V4-aligned semantics: anchored to a single time.Time at construction;
// every span's StartMs/EndMs is millis since that anchor. Spans returned
// sorted by StartMs ASC (then DurationMs DESC) so the waterfall renders
// in chronological order with longer parent spans before nested children.
type SpanCollector struct {
	mu     sync.Mutex
	anchor time.Time
	spans  []Span
}

// NewSpanCollector returns a collector anchored to time.Now().
func NewSpanCollector() *SpanCollector {
	return &SpanCollector{anchor: time.Now()}
}

// Start records a span's begin time and returns an end-fn the caller
// invokes (typically via `defer`) to record the end time. Optional
// `detail` strings passed to the end-fn are joined with " | " and stored.
//
// Usage:
//
//	end := sc.Start("postgres.GetState")
//	defer end("session=" + id)
func (sc *SpanCollector) Start(name string) func(detail ...string) {
	startMs := time.Since(sc.anchor).Milliseconds()
	return func(detail ...string) {
		endMs := time.Since(sc.anchor).Milliseconds()
		joined := ""
		for i, d := range detail {
			if i > 0 {
				joined += " | "
			}
			joined += d
		}
		sc.mu.Lock()
		defer sc.mu.Unlock()
		sc.spans = append(sc.spans, Span{
			Name:       name,
			StartMs:    startMs,
			EndMs:      endMs,
			DurationMs: endMs - startMs,
			Detail:     joined,
		})
	}
}

// Spans returns a sorted snapshot of every span recorded so far.
// Subsequent Start calls do NOT affect the returned slice.
func (sc *SpanCollector) Spans() []Span {
	sc.mu.Lock()
	out := make([]Span, len(sc.spans))
	copy(out, sc.spans)
	sc.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartMs != out[j].StartMs {
			return out[i].StartMs < out[j].StartMs
		}
		return out[i].DurationMs > out[j].DurationMs
	})
	return out
}

type spanCtxKey struct{}

// WithSpanCollector returns ctx with sc attached. Adapters and use cases
// pull the collector via SpanFromContext to emit spans without taking a
// SpanCollector dep on every method signature.
func WithSpanCollector(ctx context.Context, sc *SpanCollector) context.Context {
	return context.WithValue(ctx, spanCtxKey{}, sc)
}

// SpanFromContext returns the SpanCollector attached to ctx, or nil if
// none. The nil case is the silent-success path: code that wants spans
// when they're available but doesn't insist on having a collector reads
// the value once and skips the Start call when nil.
//
// Pattern:
//
//	if sc := domain.SpanFromContext(ctx); sc != nil {
//	    end := sc.Start("postgres.GetState")
//	    defer end()
//	}
func SpanFromContext(ctx context.Context) *SpanCollector {
	v, _ := ctx.Value(spanCtxKey{}).(*SpanCollector)
	return v
}
