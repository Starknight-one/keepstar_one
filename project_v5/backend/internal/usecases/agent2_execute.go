package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
)

// Agent2ExecuteRequest is the contract callers (HTTP handler in chunk 6c
// or the smoke test in 6b) hand to Agent2Execute. SessionID must already
// exist (created upstream by /session/init in 6c, or directly by the
// smoke test). State.Current.Data must be populated by Agent1 before this
// call (chunk 7) or by the test/fixture wiring.
type Agent2ExecuteRequest struct {
	SessionID  string
	TenantSlug string
	UserQuery  string
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
//   5. ChatWithToolsCached(tools=[visual_assembly], CacheConfig:
//      tools+system+(conv if ≥ 2 msgs), ToolChoice="any").
//   6. For each tool call returned, run it; on Go-error, retry once with
//      empty input (V4 graceful degradation).
//   7. Append assistant:tool_use + user:tool_result messages to state.
//   8. Reload state, return Document.
type Agent2Execute struct {
	llm          ports.LLMPort
	state        ports.StatePort
	toolRegistry *tools.Registry
	promptCache  *PromptCache
}

// NewAgent2Execute wires the use case. All deps required.
func NewAgent2Execute(
	llm ports.LLMPort,
	state ports.StatePort,
	toolRegistry *tools.Registry,
	promptCache *PromptCache,
) *Agent2Execute {
	return &Agent2Execute{
		llm:          llm,
		state:        state,
		toolRegistry: toolRegistry,
		promptCache:  promptCache,
	}
}

// historyLimit is the V4-proven trim point for Agent2 history. Two prior
// turns × 2 messages each (assistant tool_use + user tool_result) = 4.
// Older turns are dropped from the LLM context to keep token cost bounded;
// the full history persists in state for replay/debugging.
const historyLimit = 4

// Execute runs one Agent2 turn end-to-end. Returns the assembled response
// or a Go error for transport-layer failures (state retrieval / write
// failed, LLM call failed). Tool-side errors land in ToolResult.IsError
// and are surfaced to the LLM next turn.
func (uc *Agent2Execute) Execute(ctx context.Context, req Agent2ExecuteRequest) (*Agent2ExecuteResponse, error) {
	start := time.Now()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("agent2.execute")
		defer end()
	}

	state, err := uc.state.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	var systemPrompt string
	{
		end := startSpan(ctx, "agent2.prompt")
		systemPrompt, err = uc.promptCache.GetOrBuild(ctx, req.TenantSlug, 3)
		end()
		if err != nil {
			return nil, fmt.Errorf("build system prompt: %w", err)
		}
	}

	// Trim history to the last 4 messages, append the new user query.
	prior := state.Agent2History
	if len(prior) > historyLimit {
		prior = prior[len(prior)-historyLimit:]
	}
	messages := make([]domain.LLMMessage, 0, len(prior)+1)
	messages = append(messages, prior...)
	userContent := req.UserQuery
	if req.Microcontext != "" {
		userContent = "<microcontext>" + req.Microcontext + "</microcontext>\n" + req.UserQuery
	}
	messages = append(messages, domain.LLMMessage{Role: "user", Content: userContent})

	tools := uc.toolRegistry.GetDefinitions()
	cfg := ports.CacheConfig{
		CacheTools:        true,
		CacheSystem:       true,
		CacheConversation: len(messages) > 1,
		ToolChoice:        "any",
	}

	end := startSpan(ctx, "agent2.llm")
	resp, err := uc.llm.ChatWithToolsCached(ctx, systemPrompt, messages, tools, cfg)
	end()
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}

	// Run each tool call; collect history messages.
	toolCtx := domain.ToolContext{
		SessionID:  req.SessionID,
		TenantSlug: req.TenantSlug,
	}
	historyAppend := []domain.LLMMessage{}
	for _, tc := range resp.ToolCalls {
		toolEnd := startSpan(ctx, "agent2.tool."+tc.Name)
		result, runErr := uc.runToolWithRetry(ctx, toolCtx, tc)
		toolEnd()
		if runErr != nil {
			// Transport-level failure (registry doesn't know the tool, or
			// Go panic recovered as an error). Surface to caller.
			return nil, fmt.Errorf("tool %s: %w", tc.Name, runErr)
		}
		historyAppend = append(historyAppend,
			domain.LLMMessage{Role: "assistant", ToolCalls: []domain.ToolCall{tc}},
			domain.LLMMessage{Role: "user", ToolResult: &domain.ToolResult{
				ToolUseID: tc.ID,
				Content:   result.Content,
				IsError:   result.IsError,
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

// startSpan is a small helper that returns a no-op closer when there's
// no SpanCollector in ctx, so the call sites stay one-liner-ish without
// the "if sc != nil { ... }" boilerplate spread across the use case.
// Calling the returned func ends the span — typical pattern is one
// imperative call after the work block (not defer), so the span end
// happens before later spans that depend on the same work.
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

// runToolWithRetry invokes a tool once. On Go-error (transport failure),
// retries once with empty input — V4's graceful-degradation pattern that
// recovers from arg-parsing crashes. Tool-side IsError responses are NOT
// retried; they're informational for the LLM's next turn.
func (uc *Agent2Execute) runToolWithRetry(ctx context.Context, toolCtx domain.ToolContext, tc domain.ToolCall) (*domain.ToolResult, error) {
	res, err := uc.toolRegistry.Execute(ctx, toolCtx, tc)
	if err == nil {
		return res, nil
	}
	slog.Warn("agent2: tool errored, retrying with empty input", "tool", tc.Name, "err", err)
	emptyCall := domain.ToolCall{ID: tc.ID, Name: tc.Name, Input: map[string]interface{}{}}
	return uc.toolRegistry.Execute(ctx, toolCtx, emptyCall)
}
