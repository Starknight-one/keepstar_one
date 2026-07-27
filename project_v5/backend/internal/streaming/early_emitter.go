package streaming

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"keepstar_v5/internal/domain"
)

// maxEarlyBlocks bounds early emission — mirrors compose_turn's 8-block
// cap (§4.7). Anything beyond stops early emission; execute-time
// validation rejects the whole call anyway.
const maxEarlyBlocks = 8

// EarlyEmitter converts the incremental parse of a compose_turn input
// into real mid-generation TurnBlock emissions.
//
// Chosen path (the simplest correct one, per lane brief): only the
// LEADING RUN OF TEXT BLOCKS is emitted early — text needs no assembly,
// so the emitted block is byte-identical to what compose_turn would emit
// at execute time. The first render (document) block STOPS early emission:
// documents need the visual_assembly chain, which runs inside
// compose_turn's Execute, and emitting later text blocks ahead of an
// unassembled document would reorder the wire. Blocks after the stop
// point are emitted at execute time in order, exactly as today.
// No placeholders, no replace frames, no ordering races.
//
// Handshake with compose_turn (the dedupe contract):
//   - Count() = how many leading blocks are already on the wire —
//     compose_turn skips re-emitting specs[0..Count()-1];
//   - Claim()/Claimed() = the one-call-per-turn guard. compose_turn
//     claims the turn when its validation passes, OR as soon as any early
//     block from its input reached the wire (a sibling call must not
//     compose on top of a half-streamed turn). A call that failed
//     validation before anything was emitted does NOT claim, keeping the
//     existing retry contract intact.
//
// Feed runs on the LLM-call goroutine; Count/Claim run on the same
// goroutine strictly after the stream is drained. The mutex is defensive
// depth, not a load-bearing synchronization.
type EarlyEmitter struct {
	mu      sync.Mutex
	parser  *BlockParser
	emit    func(domain.TurnBlock)
	count   int
	stopped bool
	claimed bool
}

// NewEarlyEmitter builds an emitter over sink — in production the
// domain.TurnBlockCollector's Emit, so every early block hits the SSE
// `event: block` writer AND is retained for the terminal result frame.
func NewEarlyEmitter(sink func(domain.TurnBlock)) *EarlyEmitter {
	return &EarlyEmitter{parser: NewBlockParser(), emit: sink}
}

// Feed consumes one raw tool-input fragment (ToolInputHook.OnFragment
// signature). Complete leading text blocks are emitted immediately; the
// first non-text / invalid block stops early emission for the turn.
func (e *EarlyEmitter) Feed(fragment string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopped {
		return
	}
	for _, raw := range e.parser.Feed(fragment) {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			e.stopped = true
			return
		}
		kind, _ := m["kind"].(string)
		if kind != domain.BlockKindText {
			// Render (or unknown) block: needs execute-time assembly —
			// everything from here on is emitted by compose_turn itself.
			e.stopped = true
			return
		}
		text, _ := m["text"].(string)
		if strings.TrimSpace(text) == "" {
			// Would fail compose_turn validation; leave the whole call to
			// execute time so its error path stays authoritative.
			e.stopped = true
			return
		}
		if e.count >= maxEarlyBlocks {
			e.stopped = true
			return
		}
		// Same shape compose_turn emits for a text block (Display empty),
		// so the terminal frame's canonical list is indistinguishable
		// from a batch-emitted turn.
		e.emit(domain.TurnBlock{
			BlockID: newEarlyBlockID(),
			Kind:    domain.BlockKindText,
			Text:    text,
		})
		e.count++
	}
}

// Count returns how many blocks were emitted early this turn.
func (e *EarlyEmitter) Count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.count
}

// Claim marks the turn as composed. Idempotent.
func (e *EarlyEmitter) Claim() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.claimed = true
}

// Claimed reports whether a compose_turn call already consumed this turn.
func (e *EarlyEmitter) Claimed() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.claimed
}

// earlyEmitterCtxKey is the private ctx key for the emitter.
type earlyEmitterCtxKey struct{}

// WithEarlyEmitter returns ctx with e attached. agent2_execute installs
// it (together with the ToolInputHook) before the Agent2 LLM call so the
// same ctx later carries it through the registry into compose_turn.
func WithEarlyEmitter(ctx context.Context, e *EarlyEmitter) context.Context {
	return context.WithValue(ctx, earlyEmitterCtxKey{}, e)
}

// EarlyEmitterFromContext returns the emitter attached to ctx, or nil.
// nil = no streaming hook this turn — compose_turn falls back to the
// legacy collector-count guard and emits every block at execute time.
func EarlyEmitterFromContext(ctx context.Context) *EarlyEmitter {
	e, _ := ctx.Value(earlyEmitterCtxKey{}).(*EarlyEmitter)
	return e
}

// newEarlyBlockID mints a wire id for an early-emitted block — same
// format as compose_turn's newBlockID ("blk_" + 8 hex chars), so apply
// targets and the frontend dedupe treat both identically.
func newEarlyBlockID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("blk_%x", time.Now().UnixNano())
	}
	return "blk_" + hex.EncodeToString(b[:])
}
