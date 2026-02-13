package domain

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSpanCollector_BasicSpan(t *testing.T) {
	sc := NewSpanCollector()
	end := sc.Start("test.op")
	time.Sleep(5 * time.Millisecond)
	end()

	spans := sc.Spans()
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	if spans[0].Name != "test.op" {
		t.Errorf("want name test.op, got %s", spans[0].Name)
	}
	if spans[0].DurationMs < 1 {
		t.Errorf("want duration >= 1ms, got %d", spans[0].DurationMs)
	}
	if spans[0].Detail != "" {
		t.Errorf("want empty detail, got %q", spans[0].Detail)
	}
}

func TestSpanCollector_SpanWithDetail(t *testing.T) {
	sc := NewSpanCollector()
	end := sc.Start("agent1.tool")
	end("catalog_search")

	spans := sc.Spans()
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	if spans[0].Detail != "catalog_search" {
		t.Errorf("want detail 'catalog_search', got %q", spans[0].Detail)
	}
}

func TestSpanCollector_SortedByStartMs(t *testing.T) {
	sc := NewSpanCollector()

	// Create spans in reverse order
	end3 := sc.Start("third")
	time.Sleep(2 * time.Millisecond)
	end2 := sc.Start("second")
	time.Sleep(2 * time.Millisecond)
	end1 := sc.Start("first")

	// End in arbitrary order
	end1()
	end2()
	end3()

	spans := sc.Spans()
	if len(spans) != 3 {
		t.Fatalf("want 3 spans, got %d", len(spans))
	}

	// Should be sorted by StartMs ASC
	for i := 1; i < len(spans); i++ {
		if spans[i].StartMs < spans[i-1].StartMs {
			t.Errorf("spans not sorted: [%d].StartMs=%d < [%d].StartMs=%d",
				i, spans[i].StartMs, i-1, spans[i-1].StartMs)
		}
	}
}

func TestSpanCollector_SameStartSortedByDuration(t *testing.T) {
	sc := NewSpanCollector()

	// Both start at ~same time, but parent lasts longer
	endParent := sc.Start("parent")
	endChild := sc.Start("child")
	time.Sleep(2 * time.Millisecond)
	endChild()
	time.Sleep(5 * time.Millisecond)
	endParent()

	spans := sc.Spans()
	if len(spans) != 2 {
		t.Fatalf("want 2 spans, got %d", len(spans))
	}

	// If same start, longer duration should come first (parent before child)
	if spans[0].DurationMs < spans[1].DurationMs {
		t.Errorf("same-start sort: parent (dur=%d) should be before child (dur=%d)",
			spans[0].DurationMs, spans[1].DurationMs)
	}
}

func TestSpanCollector_ConcurrentSafety(t *testing.T) {
	sc := NewSpanCollector()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			end := sc.Start("concurrent")
			time.Sleep(time.Millisecond)
			end("done")
		}(i)
	}

	wg.Wait()
	spans := sc.Spans()
	if len(spans) != goroutines {
		t.Errorf("want %d spans, got %d", goroutines, len(spans))
	}
}

func TestSpanCollector_ContextRoundTrip(t *testing.T) {
	sc := NewSpanCollector()
	ctx := WithSpanCollector(context.Background(), sc)

	retrieved := SpanFromContext(ctx)
	if retrieved != sc {
		t.Error("SpanFromContext should return same collector")
	}

	end := retrieved.Start("from-ctx")
	end()

	spans := sc.Spans()
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
}

func TestSpanFromContext_NilCollector(t *testing.T) {
	sc := SpanFromContext(context.Background())
	if sc != nil {
		t.Error("SpanFromContext on plain context should return nil")
	}
}

func TestStageContextRoundTrip(t *testing.T) {
	ctx := WithStage(context.Background(), "agent1")
	stage := StageFromContext(ctx)
	if stage != "agent1" {
		t.Errorf("want stage 'agent1', got %q", stage)
	}
}

func TestStageFromContext_Empty(t *testing.T) {
	stage := StageFromContext(context.Background())
	if stage != "" {
		t.Errorf("want empty stage, got %q", stage)
	}
}
