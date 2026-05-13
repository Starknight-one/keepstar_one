package usecases

import (
	"context"

	"keepstar_v5/internal/domain"
)

// withSpan is the chunk-8 ctx-aware span helper. Returns the updated ctx
// (carrying the new span as the active parent for nested withSpan calls)
// plus a SpanHandle the caller uses for SetAttr / SetError / End.
//
// Nil-safe: when no SpanCollector is in ctx, returns the original ctx
// and a nil-as-noop handle. SpanHandle's methods all early-return on nil
// receiver, so callers can write the happy path without an "if span !=
// nil" guard.
//
// Pattern:
//
//	ctx, span := withSpan(ctx, "agent1.llm")
//	defer span.End()
//	span.SetAttrs(map[string]any{"model": ..., "tokens.input": ...})
//	if err != nil { span.SetError(err) }
func withSpan(ctx context.Context, name string) (context.Context, *domain.SpanHandle) {
	sc := domain.SpanFromContext(ctx)
	if sc == nil {
		return ctx, nil
	}
	return sc.StartSpan(ctx, name)
}
