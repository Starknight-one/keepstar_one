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
