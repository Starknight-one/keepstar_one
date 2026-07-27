package streaming

// EarlyEmitter tests: the leading-text-run policy, the stop-at-render
// rule (documents wait for execute-time assembly), the count/claim
// handshake surface, and shape parity with compose_turn's text blocks.

import (
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

func TestEarlyEmitterLeadingTextRun(t *testing.T) {
	var got []domain.TurnBlock
	e := NewEarlyEmitter(func(b domain.TurnBlock) { got = append(got, b) })

	input := `{"blocks":[` +
		`{"kind":"text","text":"first"},` +
		`{"kind":"text","text":"second"},` +
		`{"kind":"render","preset":"card"},` +
		`{"kind":"text","text":"AFTER RENDER — must NOT early-emit"}]}`
	// Hostile split: byte at a time.
	for i := 0; i < len(input); i++ {
		e.Feed(input[i : i+1])
	}

	if len(got) != 2 {
		t.Fatalf("early-emitted %d blocks, want 2 (leading text run only): %+v", len(got), got)
	}
	if got[0].Text != "first" || got[1].Text != "second" {
		t.Errorf("texts = %q, %q", got[0].Text, got[1].Text)
	}
	for i, b := range got {
		if b.Kind != domain.BlockKindText {
			t.Errorf("block[%d].Kind = %q, want text", i, b.Kind)
		}
		// Shape parity with compose_turn's execute-time text blocks:
		// Display empty, Document nil, blk_-prefixed id.
		if b.Display != "" || b.Document != nil {
			t.Errorf("block[%d] carries display/document: %+v", i, b)
		}
		if !strings.HasPrefix(b.BlockID, "blk_") || len(b.BlockID) != len("blk_")+8 {
			t.Errorf("block[%d].BlockID = %q, want blk_ + 8 hex", i, b.BlockID)
		}
	}
	if got[0].BlockID == got[1].BlockID {
		t.Error("early block ids must be unique")
	}
	if e.Count() != 2 {
		t.Errorf("Count() = %d, want 2", e.Count())
	}
}

func TestEarlyEmitterRenderFirstEmitsNothing(t *testing.T) {
	var got []domain.TurnBlock
	e := NewEarlyEmitter(func(b domain.TurnBlock) { got = append(got, b) })
	e.Feed(`{"blocks":[{"kind":"render","preset":"card"},{"kind":"text","text":"later"}]}`)
	if len(got) != 0 || e.Count() != 0 {
		t.Fatalf("render-first turn early-emitted %d blocks, want 0", len(got))
	}
}

// TestEarlyEmitterInvalidTextStops — empty/whitespace text would fail
// compose_turn validation; the emitter must leave the whole call to the
// execute-time error path rather than emit a block the validator rejects.
func TestEarlyEmitterInvalidTextStops(t *testing.T) {
	var got []domain.TurnBlock
	e := NewEarlyEmitter(func(b domain.TurnBlock) { got = append(got, b) })
	e.Feed(`{"blocks":[{"kind":"text","text":"ok"},{"kind":"text","text":"  "},{"kind":"text","text":"never"}]}`)
	if len(got) != 1 || got[0].Text != "ok" {
		t.Fatalf("got %+v, want just the leading valid text", got)
	}
	if e.Count() != 1 {
		t.Errorf("Count() = %d, want 1", e.Count())
	}
}

func TestEarlyEmitterUnknownKindStops(t *testing.T) {
	var got []domain.TurnBlock
	e := NewEarlyEmitter(func(b domain.TurnBlock) { got = append(got, b) })
	e.Feed(`{"blocks":[{"kind":"mystery"},{"kind":"text","text":"never"}]}`)
	if len(got) != 0 {
		t.Fatalf("unknown kind early-emitted %+v", got)
	}
}

func TestEarlyEmitterClaimHandshake(t *testing.T) {
	e := NewEarlyEmitter(func(domain.TurnBlock) {})
	if e.Claimed() {
		t.Fatal("fresh emitter must not be claimed")
	}
	e.Claim()
	if !e.Claimed() {
		t.Fatal("Claim did not stick")
	}
	e.Claim() // idempotent
	if !e.Claimed() {
		t.Fatal("Claim must be idempotent")
	}
}

func TestEarlyEmitterCapStops(t *testing.T) {
	var got []domain.TurnBlock
	e := NewEarlyEmitter(func(b domain.TurnBlock) { got = append(got, b) })
	var sb strings.Builder
	sb.WriteString(`{"blocks":[`)
	for i := 0; i < maxEarlyBlocks+2; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"kind":"text","text":"t"}`)
	}
	sb.WriteString(`]}`)
	e.Feed(sb.String())
	if len(got) != maxEarlyBlocks {
		t.Fatalf("early-emitted %d, want cap %d", len(got), maxEarlyBlocks)
	}
}

func TestEarlyEmitterContextRoundTrip(t *testing.T) {
	e := NewEarlyEmitter(func(domain.TurnBlock) {})
	ctx := WithEarlyEmitter(t.Context(), e)
	if EarlyEmitterFromContext(ctx) != e {
		t.Fatal("ctx round-trip lost the emitter")
	}
	if EarlyEmitterFromContext(t.Context()) != nil {
		t.Fatal("bare ctx must return nil")
	}
}
