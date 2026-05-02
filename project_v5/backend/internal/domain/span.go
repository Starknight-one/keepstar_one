package domain

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// SpanStatus is "ok" by default; SetError flips it to "error".
type SpanStatus string

const (
	SpanStatusOK    SpanStatus = "ok"
	SpanStatusError SpanStatus = "error"
)

// Span describes a single timed operation in V5's request waterfall.
// Times are millisecond offsets from the SpanCollector's anchor (request
// start), not absolute wall-clock — so a serialised span list stays
// stable across timezones and serves the /debug/traces UI cleanly.
//
// Chunk 8 added structured fields:
//   - ID        — monotonic "s-N" inside the collector; safe to use as
//                 a parent reference. Not globally unique — scope is one
//                 request.
//   - ParentID  — set automatically by StartSpan when a parent is active
//                 in ctx. Empty for root spans.
//   - Status    — "ok" / "error". Empty before End() is called.
//   - Error     — populated when SetError(err) was called.
//   - Attrs     — structured tags (tokens.input, rows, tenant_id, …).
//   - Detail    — legacy freeform string from the old Start API.
type Span struct {
	ID         string         `json:"id"`
	ParentID   string         `json:"parent_id,omitempty"`
	Name       string         `json:"name"`
	StartMs    int64          `json:"start_ms"`
	EndMs      int64          `json:"end_ms"`
	DurationMs int64          `json:"duration_ms"`
	Status     SpanStatus     `json:"status,omitempty"`
	Error      string         `json:"error,omitempty"`
	Attrs      map[string]any `json:"attrs,omitempty"`
	Detail     string         `json:"detail,omitempty"`
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
	nextID int // monotonic counter for span IDs (1-based)
}

// NewSpanCollector returns a collector anchored to time.Now().
func NewSpanCollector() *SpanCollector {
	return &SpanCollector{anchor: time.Now()}
}

// Start records a span's begin time and returns an end-fn the caller
// invokes (typically via `defer`) to record the end time. Optional
// `detail` strings passed to the end-fn are joined with " | " and stored.
//
// Legacy API — predates parent linkage and structured attrs. New code
// should prefer StartSpan(ctx, name) which auto-tracks parents and
// returns a SpanHandle for SetAttr / SetError. Both APIs coexist; pick
// whichever fits.
//
// Usage:
//
//	end := sc.Start("postgres.GetState")
//	defer end("session=" + id)
func (sc *SpanCollector) Start(name string) func(detail ...string) {
	startMs := time.Since(sc.anchor).Milliseconds()
	sc.mu.Lock()
	sc.nextID++
	id := fmt.Sprintf("s-%d", sc.nextID)
	sc.mu.Unlock()
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
			ID:         id,
			Name:       name,
			StartMs:    startMs,
			EndMs:      endMs,
			DurationMs: endMs - startMs,
			Status:     SpanStatusOK,
			Detail:     joined,
		})
	}
}

// StartSpan begins a new span as a child of whatever span is currently
// "active" in ctx (returned ctx propagates this span as the new parent).
// Caller MUST invoke handle.End() — typically via defer — to record
// duration and finalise the span. Status defaults to "ok"; SetError
// flips to "error".
//
// Pattern:
//
//	ctx, span := sc.StartSpan(ctx, "agent1.llm")
//	defer span.End()
//	span.SetAttrs(map[string]any{"model": ..., "tokens.input": ...})
//	if err != nil { span.SetError(err) }
func (sc *SpanCollector) StartSpan(ctx context.Context, name string) (context.Context, *SpanHandle) {
	startMs := time.Since(sc.anchor).Milliseconds()
	parentID := currentSpanID(ctx)

	sc.mu.Lock()
	sc.nextID++
	id := fmt.Sprintf("s-%d", sc.nextID)
	idx := len(sc.spans)
	sc.spans = append(sc.spans, Span{
		ID:       id,
		ParentID: parentID,
		Name:     name,
		StartMs:  startMs,
		// EndMs / DurationMs / Status filled in by End()
	})
	sc.mu.Unlock()

	h := &SpanHandle{sc: sc, idx: idx, status: SpanStatusOK}
	newCtx := context.WithValue(ctx, spanParentIDCtxKey{}, id)
	return newCtx, h
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

// SpanHandle is the caller-side handle returned by StartSpan. Methods are
// safe to call before End(); calls after End() are silent no-ops (so a
// panic in user code that runs SetError on a deferred-ended span doesn't
// double-fault).
type SpanHandle struct {
	sc     *SpanCollector
	idx    int
	mu     sync.Mutex
	ended  bool
	status SpanStatus
	errMsg string
	attrs  map[string]any
}

// SetAttr records one structured tag on this span.
func (h *SpanHandle) SetAttr(key string, value any) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.attrs == nil {
		h.attrs = map[string]any{}
	}
	h.attrs[key] = value
}

// SetAttrs merges every key/value into the span's attrs. Useful for
// dropping a struct conversion's worth of usage data in one call.
func (h *SpanHandle) SetAttrs(attrs map[string]any) {
	if h == nil || len(attrs) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.attrs == nil {
		h.attrs = make(map[string]any, len(attrs))
	}
	for k, v := range attrs {
		h.attrs[k] = v
	}
}

// SetError flips status to "error" and stores the error message. Safe to
// call multiple times — the most recent error wins. Nil err is ignored
// (no-op) so callers can always `span.SetError(err)` without an `if`.
func (h *SpanHandle) SetError(err error) {
	if h == nil || err == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = SpanStatusError
	h.errMsg = err.Error()
}

// End finalises the span: records duration, copies the accumulated
// status / error / attrs into the collector's stored Span. Idempotent —
// repeat calls are no-ops.
func (h *SpanHandle) End() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.ended {
		h.mu.Unlock()
		return
	}
	h.ended = true
	status := h.status
	errMsg := h.errMsg
	attrs := h.attrs
	h.mu.Unlock()

	endMs := time.Since(h.sc.anchor).Milliseconds()
	h.sc.mu.Lock()
	defer h.sc.mu.Unlock()
	if h.idx < 0 || h.idx >= len(h.sc.spans) {
		return
	}
	s := &h.sc.spans[h.idx]
	s.EndMs = endMs
	s.DurationMs = endMs - s.StartMs
	s.Status = status
	s.Error = errMsg
	if len(attrs) > 0 {
		// Copy so post-End mutations on h.attrs don't leak into the stored span.
		copyAttrs := make(map[string]any, len(attrs))
		for k, v := range attrs {
			copyAttrs[k] = v
		}
		s.Attrs = copyAttrs
	}
}

type spanCtxKey struct{}
type spanParentIDCtxKey struct{}
type requestIDCtxKey struct{}

// WithRequestID returns ctx with id stamped as the per-request id. Set by
// the logging middleware so use cases / adapters can pick it up via
// RequestIDFromContext and attach it to spans / logs without the handler
// needing to thread it through every request struct.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDCtxKey{}, id)
}

// RequestIDFromContext returns the per-request id stamped by the logging
// middleware, or "" when missing (e.g. tests that bypass middleware).
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDCtxKey{}).(string)
	return v
}

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

// currentSpanID returns the span ID currently active in ctx (set by
// StartSpan), or "" when there's no parent (root span).
func currentSpanID(ctx context.Context) string {
	v, _ := ctx.Value(spanParentIDCtxKey{}).(string)
	return v
}
