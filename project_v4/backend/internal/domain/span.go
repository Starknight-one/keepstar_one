package domain

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Span represents a single timed operation in the pipeline waterfall
type Span struct {
	Name       string `json:"name"`                 // "agent1.llm.ttfb"
	StartMs    int64  `json:"startMs"`              // relative to pipeline start
	EndMs      int64  `json:"endMs"`                // relative to pipeline start
	DurationMs int64  `json:"durationMs"`           // EndMs - StartMs
	Detail     string `json:"detail,omitempty"`     // "28KB→1.2KB", "catalog_search"
}

// SpanCollector is a thread-safe collector of spans for one pipeline execution
type SpanCollector struct {
	mu    sync.Mutex
	start time.Time
	spans []Span
}

// NewSpanCollector creates a new SpanCollector anchored to now
func NewSpanCollector() *SpanCollector {
	return &SpanCollector{start: time.Now()}
}

// Start begins a named span and returns an end function.
// Call end() to close the span. Optionally pass a detail string: end("catalog_search").
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

// Spans returns a copy of collected spans sorted for waterfall display:
// StartMs ASC, then DurationMs DESC (parents before children at same start).
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

// Context keys
type ctxKeySpanCollector struct{}
type ctxKeyStage struct{}

// WithSpanCollector attaches a SpanCollector to the context
func WithSpanCollector(ctx context.Context, sc *SpanCollector) context.Context {
	return context.WithValue(ctx, ctxKeySpanCollector{}, sc)
}

// SpanFromContext retrieves the SpanCollector from context, or nil
func SpanFromContext(ctx context.Context) *SpanCollector {
	sc, _ := ctx.Value(ctxKeySpanCollector{}).(*SpanCollector)
	return sc
}

// WithStage attaches the current agent stage name (e.g. "agent1") to context
func WithStage(ctx context.Context, stage string) context.Context {
	return context.WithValue(ctx, ctxKeyStage{}, stage)
}

// StageFromContext retrieves the current stage from context, or ""
func StageFromContext(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeyStage{}).(string)
	return s
}
