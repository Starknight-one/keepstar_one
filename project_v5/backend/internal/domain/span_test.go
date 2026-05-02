package domain

import (
	"context"
	"testing"
	"time"
)

func TestSpanCollectorBasic(t *testing.T) {
	sc := NewSpanCollector()
	end := sc.Start("op1")
	time.Sleep(2 * time.Millisecond)
	end("detail")
	spans := sc.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "op1" {
		t.Errorf("name = %q", spans[0].Name)
	}
	if spans[0].DurationMs < 1 {
		t.Errorf("duration = %d, expected ≥ 1ms", spans[0].DurationMs)
	}
	if spans[0].Detail != "detail" {
		t.Errorf("detail = %q", spans[0].Detail)
	}
}

func TestSpanCollectorOrdering(t *testing.T) {
	sc := NewSpanCollector()
	sc.Start("first")()
	sc.Start("second")()
	sc.Start("third")()
	spans := sc.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3, got %d", len(spans))
	}
	for i, want := range []string{"first", "second", "third"} {
		if spans[i].Name != want {
			t.Errorf("spans[%d].Name = %q, want %q", i, spans[i].Name, want)
		}
	}
}

func TestSpanContextNilSafe(t *testing.T) {
	ctx := context.Background()
	if sc := SpanFromContext(ctx); sc != nil {
		t.Errorf("empty ctx should return nil collector, got %+v", sc)
	}
}

func TestSpanContextRoundtrip(t *testing.T) {
	sc := NewSpanCollector()
	ctx := WithSpanCollector(context.Background(), sc)
	got := SpanFromContext(ctx)
	if got != sc {
		t.Errorf("expected same collector instance back; got %p vs %p", got, sc)
	}
}

func TestSpanCollectorConcurrent(t *testing.T) {
	sc := NewSpanCollector()
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			end := sc.Start("op")
			time.Sleep(time.Microsecond)
			end()
		}(i)
	}
	for i := 0; i < 50; i++ {
		<-done
	}
	if got := len(sc.Spans()); got != 50 {
		t.Errorf("expected 50 spans recorded under concurrent Start, got %d", got)
	}
}

// ─── chunk 8: StartSpan + SpanHandle ────────────────────────────────────

func TestStartSpanParentLinkage(t *testing.T) {
	sc := NewSpanCollector()
	ctx := WithSpanCollector(context.Background(), sc)

	ctx, parent := sc.StartSpan(ctx, "agent1.execute")
	_, child := sc.StartSpan(ctx, "agent1.llm")
	child.End()
	parent.End()

	spans := sc.Spans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	// Root: agent1.execute, no parent.
	var root, nested *Span
	for i := range spans {
		switch spans[i].Name {
		case "agent1.execute":
			root = &spans[i]
		case "agent1.llm":
			nested = &spans[i]
		}
	}
	if root == nil || nested == nil {
		t.Fatalf("missing spans; got names: %v", names(spans))
	}
	if root.ParentID != "" {
		t.Errorf("root parent_id = %q, want empty", root.ParentID)
	}
	if nested.ParentID != root.ID {
		t.Errorf("nested.ParentID = %q, want root.ID = %q", nested.ParentID, root.ID)
	}
	if root.ID == "" || nested.ID == "" || root.ID == nested.ID {
		t.Errorf("expected distinct non-empty IDs; root=%q nested=%q", root.ID, nested.ID)
	}
}

func TestStartSpanSetAttrs(t *testing.T) {
	sc := NewSpanCollector()
	ctx := WithSpanCollector(context.Background(), sc)

	_, span := sc.StartSpan(ctx, "agent1.llm")
	span.SetAttr("model", "claude-haiku-4-5")
	span.SetAttrs(map[string]any{
		"tokens.input":      4110,
		"tokens.cache_read": 6244,
		"cost_usd":          0.005,
	})
	span.End()

	got := sc.Spans()[0]
	if got.Status != SpanStatusOK {
		t.Errorf("Status = %q, want %q", got.Status, SpanStatusOK)
	}
	if got.Attrs["model"] != "claude-haiku-4-5" {
		t.Errorf("Attrs[model] = %v", got.Attrs["model"])
	}
	if got.Attrs["tokens.input"] != 4110 {
		t.Errorf("Attrs[tokens.input] = %v, want 4110", got.Attrs["tokens.input"])
	}
	if got.Attrs["cost_usd"] != 0.005 {
		t.Errorf("Attrs[cost_usd] = %v, want 0.005", got.Attrs["cost_usd"])
	}
}

func TestStartSpanSetError(t *testing.T) {
	sc := NewSpanCollector()
	ctx := WithSpanCollector(context.Background(), sc)

	_, span := sc.StartSpan(ctx, "postgres.UpdateData")
	span.SetError(errTesting{"connection refused"})
	span.End()

	got := sc.Spans()[0]
	if got.Status != SpanStatusError {
		t.Errorf("Status = %q, want %q", got.Status, SpanStatusError)
	}
	if got.Error != "connection refused" {
		t.Errorf("Error = %q", got.Error)
	}

	// Nil err should NOT flip status.
	_, span2 := sc.StartSpan(ctx, "ok.path")
	span2.SetError(nil)
	span2.End()
	if sc.Spans()[1].Status != SpanStatusOK {
		t.Errorf("nil error must leave Status=ok; got %q", sc.Spans()[1].Status)
	}
}

func TestStartSpanEndIsIdempotent(t *testing.T) {
	sc := NewSpanCollector()
	ctx := WithSpanCollector(context.Background(), sc)
	_, span := sc.StartSpan(ctx, "op")
	span.End()
	span.End()  // no panic, no double-record
	span.End()
	if got := len(sc.Spans()); got != 1 {
		t.Errorf("expected 1 span after multiple End(), got %d", got)
	}
}

func TestStartSpanRootHasNoParent(t *testing.T) {
	// A bare ctx with no prior StartSpan should yield root spans.
	sc := NewSpanCollector()
	ctx := WithSpanCollector(context.Background(), sc)
	_, s1 := sc.StartSpan(ctx, "first")
	s1.End()
	_, s2 := sc.StartSpan(ctx, "second")
	s2.End()
	for _, sp := range sc.Spans() {
		if sp.ParentID != "" {
			t.Errorf("span %q has unexpected ParentID=%q", sp.Name, sp.ParentID)
		}
	}
}

type errTesting struct{ msg string }

func (e errTesting) Error() string { return e.msg }

func names(ss []Span) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Name
	}
	return out
}
