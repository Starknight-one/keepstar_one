package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/streaming"
)

// Agent2ExecuteRequest is the contract callers (HTTP handler in chunk 6c
// or the smoke test in 6b) hand to Agent2Execute. SessionID must already
// exist (created upstream by /session/init in 6c, or directly by the
// smoke test). State.Current.Data must be populated by Agent1 before this
// call (chunk 7) or by the test/fixture wiring. Mode/Role arrive from the
// session row via the pipeline handler (R17); empty values default to the
// storefront form / visitor role.
type Agent2ExecuteRequest struct {
	SessionID  string
	TenantSlug string
	UserQuery  string
	Mode       domain.PipelineMode
	Role       domain.Role
	ActorID    string
	// Microcontext is a one-line signal from the pipeline orchestrator
	// (chunk 7) describing what Agent1 just did — e.g.
	// "new_search: 12 items found", "filtered: 3 items", "no_data_change".
	// When non-empty it's prepended to the user message inside a
	// <microcontext> envelope so Agent2 can decide whether to re-render.
	// V4 pattern at pipeline_execute.go (microcontext signal generation).
	Microcontext string
}

// Agent2ExecuteResponse carries everything the caller needs after one
// Agent2 turn: the resulting Document (from state.Current.Template), the
// LLM's tool calls, the token+cost usage, and end-to-end latency.
type Agent2ExecuteResponse struct {
	Document  map[string]interface{}
	ToolCalls []domain.ToolCall
	Usage     domain.LLMUsage
	LatencyMs int64
}

// Agent2Execute is V5's chat-runtime use case for one Agent2 turn.
//
// Single-turn tool loop (V4 pattern):
//   1. Get state.
//   2. Build / fetch the cached system prompt.
//   3. Trim Agent2 history to the last 4 messages (= 2 prior turns of
//      assistant:tool_use + user:tool_result).
//   4. Append the new user message.
//   5. ChatWithToolsCached(tools=DefinitionsFor(tenant, form, visual, role),
//      CacheConfig: tools+system+(conv if ≥ 2 msgs), ToolChoice="any").
//   6. For each tool call returned, run it through the operation registry;
//      on Go-error, retry once with empty input (V4 graceful degradation).
//   7. Append assistant:tool_use + user:tool_result messages to state.
//   8. Reload state, return Document.
type Agent2Execute struct {
	llm         ports.LLMPort
	state       ports.StatePort
	registry    ports.OperationRegistry
	promptCache *PromptCache
}

// NewAgent2Execute wires the use case. All deps required.
func NewAgent2Execute(
	llm ports.LLMPort,
	state ports.StatePort,
	registry ports.OperationRegistry,
	promptCache *PromptCache,
) *Agent2Execute {
	return &Agent2Execute{
		llm:         llm,
		state:       state,
		registry:    registry,
		promptCache: promptCache,
	}
}

// historyLimit is the V4-proven trim point for Agent2 history. Two prior
// turns × 2 messages each (assistant tool_use + user tool_result) = 4.
// Older turns are dropped from the LLM context to keep token cost bounded;
// the full history persists in state for replay/debugging.
const historyLimit = 4

// Execute runs one Agent2 turn end-to-end. Returns the assembled response
// or a Go error for transport-layer failures (state retrieval / write
// failed, LLM call failed). Operation-side errors land in the tool_result
// with IsError and are surfaced to the LLM next turn.
func (uc *Agent2Execute) Execute(ctx context.Context, req Agent2ExecuteRequest) (*Agent2ExecuteResponse, error) {
	start := time.Now()
	ctx, topSpan := withSpan(ctx, "agent2.execute")
	defer topSpan.End()

	mode := req.Mode
	if mode == "" {
		mode = domain.ModeStorefront
	}
	role := req.Role
	if role == "" {
		role = domain.RoleVisitor
	}

	// Real incremental streaming (final owner decision 3): on every
	// turn-composing form (anything but the storefront fast path, which
	// keeps its proven non-streaming call byte-identical) with a block
	// collector present, attach the tool-input hook: the LLM adapter
	// streams the call and feeds compose_turn's partial input JSON to the
	// EarlyEmitter AS GENERATED, so the leading text blocks fly as
	// `event: block` SSE frames mid-LLM-call. The emitter rides the same
	// ctx into compose_turn's Execute (via the registry) for the
	// count/claim dedupe handshake — nothing is ever emitted twice.
	// Adapters/fakes that ignore the hook degrade to today's batch
	// emission; correctness never depends on the hook firing.
	if collector := domain.TurnBlockCollectorFromContext(ctx); collector != nil && mode != domain.ModeStorefront {
		early := streaming.NewEarlyEmitter(collector.Emit)
		ctx = streaming.WithEarlyEmitter(ctx, early)
		ctx = streaming.WithToolInputHook(ctx, &streaming.ToolInputHook{
			Tool:       "compose_turn",
			OnFragment: early.Feed,
		})
	}

	state, err := uc.state.GetState(ctx, req.SessionID)
	if err != nil {
		topSpan.SetError(err)
		return nil, fmt.Errorf("get state: %w", err)
	}

	var systemPrompt string
	{
		_, promptSpan := withSpan(ctx, "agent2.prompt")
		// Per-form base prompt (R24): storefront keeps its exact bytes;
		// onboarding/crm append the compose_turn (+ choreography) blocks.
		systemPrompt, err = uc.promptCache.GetOrBuildForm(ctx, req.TenantSlug, 3, mode)
		if err != nil {
			promptSpan.SetError(err)
		}
		promptSpan.End()
		if err != nil {
			topSpan.SetError(err)
			return nil, fmt.Errorf("build system prompt: %w", err)
		}
	}

	// Trim history to the last 4 messages, append the new user query.
	//
	// Pair-aware trim: if the cutoff would leave a user message whose
	// content is a tool_result orphan (no preceding assistant tool_use),
	// step back one position. The Anthropic API rejects orphaned
	// tool_result blocks with «messages.0.content.0: unexpected
	// `tool_use_id` found in `tool_result` blocks». Each tool_use lands
	// just before its tool_result, so backing up by 1 always lands on
	// the matching assistant message (or earlier user content).
	prior := state.Agent2History
	if len(prior) > historyLimit {
		start := len(prior) - historyLimit
		for start > 0 && isToolResultOnly(prior[start]) {
			start--
		}
		prior = prior[start:]
	}
	messages := make([]domain.LLMMessage, 0, len(prior)+1)
	messages = append(messages, prior...)
	userContent := req.UserQuery
	if req.Microcontext != "" {
		userContent = "<microcontext>" + req.Microcontext + "</microcontext>\n" + req.UserQuery
	}
	// Build tree_map of the current Document (modify-mode context for
	// Agent2). Empty doc → nil → no <formation_tree> envelope. The map
	// fits inside the per-turn-variable suffix of the user message; it
	// does NOT enter the cached system+tools prefix.
	if treeBlock := buildFormationTreeBlock(ctx, state.Current.Template); treeBlock != "" {
		userContent = treeBlock + "\n" + userContent
	}
	messages = append(messages, domain.LLMMessage{Role: "user", Content: userContent})

	// Registry-scoped visibility (R8): the visual-plane operations for this
	// tenant + form + role. On the storefront form that is exactly
	// [visual_assembly] — byte-identical to the old name filter, so the
	// cached tools block (CacheTools=true) does not drift.
	toolDefs := uc.registry.DefinitionsFor(ctx, req.TenantSlug, mode, domain.AgentVisual, role)
	cfg := ports.CacheConfig{
		CacheTools:        true,
		CacheSystem:       true,
		CacheConversation: len(messages) > 1,
		ToolChoice:        "any",
	}

	_, llmSpan := withSpan(ctx, "agent2.llm")
	resp, err := uc.llm.ChatWithToolsCached(ctx, systemPrompt, messages, toolDefs, cfg)
	if err != nil {
		llmSpan.SetError(err)
		llmSpan.End()
		topSpan.SetError(err)
		return nil, fmt.Errorf("LLM call: %w", err)
	}
	llmSpan.SetAttrs(map[string]any{
		"model":                  resp.Usage.Model,
		"tokens.input":           resp.Usage.InputTokens,
		"tokens.output":          resp.Usage.OutputTokens,
		"tokens.cache_read":      resp.Usage.CacheReadInputTokens,
		"tokens.cache_creation":  resp.Usage.CacheCreationInputTokens,
		"cost_usd":               resp.Usage.CostUSD,
		"tool_calls":             len(resp.ToolCalls),
		"stop_reason":            resp.StopReason,
	})
	llmSpan.End()

	// Run each tool call through the registry choke point; collect history.
	octx := domain.OperationContext{
		SessionID:  req.SessionID,
		TenantSlug: req.TenantSlug,
		Mode:       mode,
		Role:       role,
		ActorID:    req.ActorID,
	}
	historyAppend := []domain.LLMMessage{}
	for _, tc := range resp.ToolCalls {
		_, toolSpan := withSpan(ctx, "agent2.tool."+tc.Name)
		toolSpan.SetAttr("tool_name", tc.Name)
		result, runErr := uc.runOpWithRetry(ctx, octx, tc)
		if runErr != nil {
			toolSpan.SetError(runErr)
		} else if result != nil && result.Outcome != domain.OutcomeOK && result.Outcome != domain.OutcomeEmpty {
			toolSpan.SetAttr("is_error", true)
			// Surface the error content so we can diagnose tool-side
			// failures from logs without re-running the LLM call. Summary
			// content is short (one-line summary or error string).
			slog.Warn("agent2: operation returned error outcome",
				"tool", tc.Name,
				"outcome", result.Outcome,
				"content", result.Summary,
				"input", tc.Input,
			)
		}
		toolSpan.End()
		if runErr != nil {
			// Transport-level failure (Go panic recovered as an error).
			// Surface to caller.
			topSpan.SetError(runErr)
			return nil, fmt.Errorf("tool %s: %w", tc.Name, runErr)
		}
		// Bridge to the legacy tool_result shape WITHOUT metadata — exactly
		// the bytes the pre-registry path persisted (prompt-cache duty C3).
		bridged := result.ToToolResult(tc.ID)
		historyAppend = append(historyAppend,
			domain.LLMMessage{Role: "assistant", ToolCalls: []domain.ToolCall{tc}},
			domain.LLMMessage{Role: "user", ToolResult: &domain.ToolResult{
				ToolUseID: bridged.ToolUseID,
				Content:   bridged.Content,
				IsError:   bridged.IsError,
			}},
		)
	}

	if len(historyAppend) > 0 {
		// Persist the user message + the new assistant/tool-result pair.
		// Use the same userContent (with microcontext) we sent to the LLM
		// so the cached conversation block stays byte-stable on the next
		// turn.
		full := append([]domain.LLMMessage{}, prior...)
		full = append(full, domain.LLMMessage{Role: "user", Content: userContent})
		full = append(full, historyAppend...)
		if err := uc.state.AppendAgent2History(ctx, req.SessionID, full); err != nil {
			slog.Warn("agent2: AppendAgent2History failed", "session", req.SessionID, "err", err)
		}
	}

	// Reload state to pick up the tool's UpdateTemplate write.
	state, err = uc.state.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("reload state: %w", err)
	}

	return &Agent2ExecuteResponse{
		Document:  state.Current.Template,
		ToolCalls: resp.ToolCalls,
		Usage:     resp.Usage,
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}

// isToolResultOnly reports whether m is a user message that carries only
// a ToolResult (no Content). Such messages are tool-result orphans when
// they appear without a preceding assistant tool_use — the trim helper
// uses this signal to pick a clean cutoff.
func isToolResultOnly(m domain.LLMMessage) bool {
	return m.Role == "user" && m.ToolResult != nil && m.Content == ""
}

// buildFormationTreeBlock turns state.Current.Template (the previous
// turn's Document, stored as map[string]interface{}) into a compact
// <formation_tree> envelope for Agent2's modify-mode context. Returns
// "" when the doc is empty/nil — no envelope, no token cost.
//
// The doc is round-tripped through engine.Document so BuildTreeMap's
// typed walker can run. The cost is one JSON marshal+unmarshal per turn;
// trivial relative to the LLM call. Logs size + atom count for cost
// visibility under chunk-8 spans.
func buildFormationTreeBlock(ctx context.Context, tpl map[string]interface{}) string {
	if len(tpl) == 0 {
		return ""
	}
	_, span := withSpan(ctx, "agent2.tree_map.build")
	defer span.End()

	raw, err := json.Marshal(tpl)
	if err != nil {
		span.SetError(err)
		return ""
	}
	var doc engine.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		span.SetError(err)
		return ""
	}
	tm := engine.BuildTreeMap(&doc)
	if tm == nil {
		return ""
	}
	body, err := json.Marshal(tm)
	if err != nil {
		span.SetError(err)
		return ""
	}
	span.SetAttrs(map[string]any{
		"size_bytes": len(body),
		"data_count": tm.DataCount,
		"components": len(tm.Components),
	})
	return "<formation_tree>" + string(body) + "</formation_tree>"
}

// startSpan is a legacy helper retained for un-migrated call sites.
// New code should use withSpan(ctx, name) which returns a SpanHandle for
// SetAttr / SetError and threads parent linkage through the returned ctx.
func startSpan(ctx context.Context, name string) func() {
	sc := domain.SpanFromContext(ctx)
	if sc == nil {
		return func() {}
	}
	end := sc.Start(name)
	return func() {
		// agent2 spans don't carry per-call detail strings.
		end()
	}
}

// runOpWithRetry invokes an operation once. On Go-error (transport
// failure), retries once with empty input — V4's graceful-degradation
// pattern that recovers from arg-parsing crashes. Structured
// invalid/denied results are NOT retried; they're informational for the
// LLM's next turn.
func (uc *Agent2Execute) runOpWithRetry(ctx context.Context, octx domain.OperationContext, tc domain.ToolCall) (*domain.OperationResult, error) {
	res, err := uc.registry.Execute(ctx, octx, tc)
	if err == nil {
		return res, nil
	}
	slog.Warn("agent2: tool errored, retrying with empty input", "tool", tc.Name, "err", err)
	emptyCall := domain.ToolCall{ID: tc.ID, Name: tc.Name, Input: map[string]interface{}{}}
	return uc.registry.Execute(ctx, octx, emptyCall)
}
