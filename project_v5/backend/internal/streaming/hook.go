// Package streaming implements real incremental turn streaming (final
// owner decision 3, RUNTIME_SPEC.md): compose_turn's tool input is parsed
// AS THE MODEL GENERATES IT, so leading text blocks reach the widget
// mid-LLM-call instead of arriving in one batch after the whole tool input
// is complete.
//
// Three pieces, all in this package:
//
//   - ToolInputHook — the per-call contract the anthropic adapter honors:
//     when present on the ctx, the adapter switches to the streaming
//     Messages API and forwards raw input_json_delta fragments of the
//     named tool to OnFragment as they arrive.
//   - BlockParser — an incremental parser over the growing JSON prefix of
//     {"blocks":[...]} that yields each complete block object exactly once.
//   - EarlyEmitter — glues the two to the domain.TurnBlockCollector: text
//     blocks are emitted to the collector the moment they complete;
//     render blocks (which need the assembly chain) are left for
//     compose_turn's Execute, which skips the already-emitted prefix via
//     the count/claim handshake.
package streaming

import "context"

// ToolInputHook asks the LLM adapter to stream the call and forward raw
// partial tool-input JSON fragments for ONE named tool as they arrive.
//
// Contract (implemented by adapters/anthropic ChatWithToolsCached):
//   - only the FIRST content block whose tool name equals Tool is
//     forwarded — a second call of the same tool in one response is
//     ignored here and refused later by compose_turn's one-call guard;
//   - OnFragment receives the raw input_json_delta payloads in order, on
//     the request goroutine, mid-LLM-call — keep it fast;
//   - the hook is advisory: an adapter that does not support it (fakes,
//     future providers) simply never calls OnFragment, and the turn
//     degrades to batch emission at execute time. Correctness never
//     depends on the hook firing.
type ToolInputHook struct {
	// Tool is the wire name whose input fragments are forwarded
	// (e.g. "compose_turn").
	Tool string
	// OnFragment receives each raw partial JSON fragment as it arrives.
	OnFragment func(fragment string)
}

// toolInputHookCtxKey is the private ctx key for the hook.
type toolInputHookCtxKey struct{}

// WithToolInputHook returns ctx with the hook attached. Installed by
// agent2_execute around the Agent2 LLM call on turn-composing forms.
func WithToolInputHook(ctx context.Context, h *ToolInputHook) context.Context {
	return context.WithValue(ctx, toolInputHookCtxKey{}, h)
}

// ToolInputHookFromContext returns the hook attached to ctx, or nil. The
// nil case is the default non-streaming path — byte-identical behavior to
// the pre-streaming adapter.
func ToolInputHookFromContext(ctx context.Context) *ToolInputHook {
	h, _ := ctx.Value(toolInputHookCtxKey{}).(*ToolInputHook)
	return h
}
