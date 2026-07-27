package domain

// Streamed turn protocol (R9 as OVERRIDDEN by final owner decision 3):
// blocks stream to the widget AS PRODUCED — each TurnBlock the compose_turn
// operation emits becomes its own `event: block` SSE frame the moment it is
// assembled, not batched into the terminal frame. The TurnBlockCollector is
// the seam: the pipeline orchestrator installs one on the ctx before Agent2
// runs; compose_turn pulls it via TurnBlockCollectorFromContext and Emits
// each block as it is produced; the collector forwards every block to the
// transport's OnBlock sink (the SSE handler) AND retains the ordered list
// for the terminal `result` frame / POST /pipeline JSON body.
//
// Mirrors the SpanCollector ctx pattern (span.go): code that wants the
// collector when it's available reads it once and degrades gracefully when
// nil (no streaming transport, unit tests, direct registry invocations).

import (
	"context"
	"sync"
)

// TurnBlockCollector accumulates the TurnBlocks of one pipeline turn and
// forwards each to an optional per-block sink as it is emitted. Safe for
// concurrent use; in practice compose_turn emits sequentially on the
// request goroutine, so sink invocations arrive in block order.
type TurnBlockCollector struct {
	mu      sync.Mutex
	blocks  []TurnBlock
	onBlock func(TurnBlock)
}

// NewTurnBlockCollector builds a collector. onBlock is the per-block sink
// (the streaming handler's `event: block` writer); nil = collect only
// (plain POST /pipeline still gets the full ordered list via Blocks).
func NewTurnBlockCollector(onBlock func(TurnBlock)) *TurnBlockCollector {
	return &TurnBlockCollector{onBlock: onBlock}
}

// Emit records one block and forwards it to the sink. Called by
// compose_turn for each block AS PRODUCED — text blocks immediately,
// document blocks after their assembly chain finishes.
func (c *TurnBlockCollector) Emit(b TurnBlock) {
	c.mu.Lock()
	c.blocks = append(c.blocks, b)
	sink := c.onBlock
	c.mu.Unlock()
	if sink != nil {
		sink(b)
	}
}

// Blocks returns a copy of the ordered block list emitted so far.
func (c *TurnBlockCollector) Blocks() []TurnBlock {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.blocks) == 0 {
		return nil
	}
	out := make([]TurnBlock, len(c.blocks))
	copy(out, c.blocks)
	return out
}

// Count returns how many blocks have been emitted this turn — compose_turn
// uses it to enforce its one-call-per-turn contract (§4.7): a second call
// after a successful compose is refused, while a call that failed before
// emitting anything leaves the count at 0 and the retry path open.
func (c *TurnBlockCollector) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.blocks)
}

// turnBlockCtxKey is the private ctx key for the collector.
type turnBlockCtxKey struct{}

// WithTurnBlockCollector returns ctx with c attached. The pipeline
// orchestrator installs it around the Agent2 leg of every turn.
func WithTurnBlockCollector(ctx context.Context, c *TurnBlockCollector) context.Context {
	return context.WithValue(ctx, turnBlockCtxKey{}, c)
}

// TurnBlockCollectorFromContext returns the collector attached to ctx, or
// nil if none. The nil case is the silent-success path: compose_turn still
// assembles and returns its summary, it just has nowhere to stream.
func TurnBlockCollectorFromContext(ctx context.Context) *TurnBlockCollector {
	v, _ := ctx.Value(turnBlockCtxKey{}).(*TurnBlockCollector)
	return v
}
