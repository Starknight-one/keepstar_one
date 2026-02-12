package domain

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Span represents a single timed operation
type Span struct {
	Name       string `json:"name"`
	StartMs    int64  `json:"startMs"`
	EndMs      int64  `json:"endMs"`
	DurationMs int64  `json:"durationMs"`
	Detail     string `json:"detail,omitempty"`
}

// SpanCollector is a thread-safe collector of spans
type SpanCollector struct {
	mu    sync.Mutex
	start time.Time
	spans []Span
}

// NewSpanCollector creates a new SpanCollector anchored to now
func NewSpanCollector() *SpanCollector {
	return &SpanCollector{start: time.Now()}
}

// Start begins a named span and returns an end function
func (sc *SpanCollector) Start(name string) func(...string) {
	startMs := time.Since(sc.start).Milliseconds()
	return func(detail ...string) {
		endMs := time.Since(sc.start).Milliseconds()
		s := Span{
			Name:       name,
			StartMs:    startMs,
			EndMs:      endMs,
			DurationMs: endMs - startMs,
		}
		if len(detail) > 0 {
			s.Detail = detail[0]
		}
		sc.mu.Lock()
		sc.spans = append(sc.spans, s)
		sc.mu.Unlock()
	}
}

// Spans returns a sorted copy of collected spans
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

type ctxKeySpanCollector struct{}

// WithSpanCollector attaches a SpanCollector to the context
func WithSpanCollector(ctx context.Context, sc *SpanCollector) context.Context {
	return context.WithValue(ctx, ctxKeySpanCollector{}, sc)
}

// SpanFromContext retrieves the SpanCollector from context, or nil
func SpanFromContext(ctx context.Context) *SpanCollector {
	sc, _ := ctx.Value(ctxKeySpanCollector{}).(*SpanCollector)
	return sc
}
