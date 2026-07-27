package domain

// Tests for the streamed-turn-protocol seam (R9 as overridden by final
// owner decision 3): the TurnBlockCollector must forward each block to the
// sink AS EMITTED (real streaming, not batched at the end), retain the
// ordered list for the terminal frame, and ride the ctx like the
// SpanCollector does.

import (
	"context"
	"testing"
)

func TestTurnBlockCollectorForwardsAsEmitted(t *testing.T) {
	var streamed []TurnBlock
	c := NewTurnBlockCollector(func(b TurnBlock) { streamed = append(streamed, b) })

	c.Emit(TurnBlock{BlockID: "b1", Kind: BlockKindText, Text: "hello"})
	// The sink must already have the first block BEFORE the second is
	// emitted — batching all blocks at the end would break this.
	if len(streamed) != 1 || streamed[0].BlockID != "b1" {
		t.Fatalf("sink after first Emit = %+v, want exactly [b1]", streamed)
	}
	c.Emit(TurnBlock{BlockID: "b2", Kind: BlockKindDocument, Display: DisplayInline})

	if len(streamed) != 2 || streamed[0].BlockID != "b1" || streamed[1].BlockID != "b2" {
		t.Fatalf("sink order = %+v, want [b1 b2]", streamed)
	}
	blocks := c.Blocks()
	if len(blocks) != 2 || blocks[0].BlockID != "b1" || blocks[1].BlockID != "b2" {
		t.Fatalf("Blocks() = %+v, want ordered [b1 b2]", blocks)
	}
	if c.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", c.Count())
	}
}

func TestTurnBlockCollectorNilSinkCollects(t *testing.T) {
	c := NewTurnBlockCollector(nil) // plain POST /pipeline: collect only
	c.Emit(TurnBlock{BlockID: "b1", Kind: BlockKindText, Text: "t"})
	if got := c.Blocks(); len(got) != 1 {
		t.Fatalf("Blocks() len = %d, want 1", len(got))
	}
}

func TestTurnBlockCollectorBlocksReturnsCopy(t *testing.T) {
	c := NewTurnBlockCollector(nil)
	c.Emit(TurnBlock{BlockID: "b1", Kind: BlockKindText, Text: "t"})
	got := c.Blocks()
	got[0].BlockID = "mutated"
	if c.Blocks()[0].BlockID != "b1" {
		t.Fatal("Blocks() must return a copy — caller mutation leaked into the collector")
	}
}

func TestTurnBlockCollectorEmptyBlocksNil(t *testing.T) {
	c := NewTurnBlockCollector(nil)
	if got := c.Blocks(); got != nil {
		t.Fatalf("Blocks() on empty collector = %+v, want nil (wire omits the field)", got)
	}
}

func TestTurnBlockCollectorContextRoundTrip(t *testing.T) {
	if got := TurnBlockCollectorFromContext(context.Background()); got != nil {
		t.Fatalf("bare ctx collector = %v, want nil", got)
	}
	c := NewTurnBlockCollector(nil)
	ctx := WithTurnBlockCollector(context.Background(), c)
	if got := TurnBlockCollectorFromContext(ctx); got != c {
		t.Fatalf("round-trip returned %v, want the installed collector", got)
	}
}
