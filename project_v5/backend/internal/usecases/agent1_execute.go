package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"encoding/json"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/prompts"
	"keepstar_v5/internal/tools"
)

// filterTriggers matches user queries that imply filtering existing data
// (price / quantity / subset markers in Russian + English).
var filterTriggers = regexp.MustCompile(`(?i)(только|лишь|оставь|исключи|дешевле|дороже|выше|ниже|от \d|до \d)`)

// styleFieldNames matches queries about display fields — "убери <field>"
// is a STYLE request that should NOT trigger the deterministic guard.
var styleFieldNames = regexp.MustCompile(`(?i)(описани|рейтинг|бренд|цен[уыа]|фото|картинк|назван|изображ|тег|катего|рейт|rating|brand|price|image|name|description|tag)`)

// Agent1ExecuteRequest is the per-turn input handed to Agent1.
type Agent1ExecuteRequest struct {
	SessionID  string
	TenantSlug string
	UserQuery  string
	TurnID     string
}

// Agent1ExecuteResponse carries everything the pipeline orchestrator needs
// to compose a microcontext signal for Agent2 plus surface diagnostics.
type Agent1ExecuteResponse struct {
	ToolName      string                 // empty when no tool call (style request)
	ToolInput     map[string]interface{} // raw input as the LLM emitted it
	ToolResult    *domain.ToolResult     // pointer because no-tool runs leave it nil
	ProductsFound int                    // for the microcontext signal
	Usage         domain.LLMUsage
	LatencyMs     int64
	LLMCallMs     int64
	ToolExecuteMs int64
	StopReason    string
	EnrichedQuery string // <state>+query as actually sent to LLM
	Bypassed      bool   // true when deterministic guard fired
}

// Agent1Execute runs one Agent1 turn — single-turn tool loop with a
// deterministic state-filter guard for obvious filter queries.
//
//  1. GetState
//  2. If data is loaded AND query matches filterTriggers AND NOT styleFieldNames →
//     bypass LLM, run _internal_state_filter directly
//  3. Otherwise build system prompt (cached) + enriched user query, call LLM
//  4. Run the first tool call returned (V4 single-turn pattern), retry once
//     with empty input on Go-error
//  5. Append user / assistant:tool_use / user:tool_result to ConversationHistory
type Agent1Execute struct {
	llm          ports.LLMPort
	state        ports.StatePort
	catalog      ports.CatalogPort
	toolRegistry *tools.Registry
	promptCache  *Agent1PromptCache
	log          *slog.Logger
}

// NewAgent1Execute wires the use case. All deps required.
func NewAgent1Execute(
	llm ports.LLMPort,
	state ports.StatePort,
	catalog ports.CatalogPort,
	toolRegistry *tools.Registry,
	promptCache *Agent1PromptCache,
	log *slog.Logger,
) *Agent1Execute {
	return &Agent1Execute{
		llm:          llm,
		state:        state,
		catalog:      catalog,
		toolRegistry: toolRegistry,
		promptCache:  promptCache,
		log:          log,
	}
}

// Execute runs one Agent1 turn end-to-end.
func (uc *Agent1Execute) Execute(ctx context.Context, req Agent1ExecuteRequest) (*Agent1ExecuteResponse, error) {
	start := time.Now()
	ctx, topSpan := withSpan(ctx, "agent1.execute")
	defer topSpan.End()

	state, err := uc.state.GetState(ctx, req.SessionID)
	if err != nil {
		topSpan.SetError(err)
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Stamp tenant_slug on the state aliases so downstream tools (which look
	// the slug up via state.Current.Meta.Aliases when ToolContext.TenantSlug
	// is empty) can resolve it. V4 pattern at agent1_execute.go:114-119.
	if req.TenantSlug != "" {
		if state.Current.Meta.Aliases == nil {
			state.Current.Meta.Aliases = map[string]string{}
		}
		state.Current.Meta.Aliases["tenant_slug"] = req.TenantSlug
	}
	state.Current.Meta.ProductCount = len(state.Current.Data.Products)
	state.Current.Meta.ServiceCount = len(state.Current.Data.Services)

	toolCtx := domain.ToolContext{
		SessionID:  req.SessionID,
		TenantSlug: req.TenantSlug,
		TurnID:     req.TurnID,
	}

	// ── Deterministic guard ──
	// If data is on screen AND query is an obvious subset/filter request
	// (and NOT a style request) → bypass the LLM and call state_filter directly.
	if state.Current.Meta.ProductCount > 0 && filterTriggers.MatchString(req.UserQuery) && !styleFieldNames.MatchString(req.UserQuery) {
		_, guardSpan := withSpan(ctx, "agent1.tool._internal_state_filter")
		guardSpan.SetAttrs(map[string]any{
			"tool_name": "_internal_state_filter",
			"bypassed":  true,
		})
		toolStart := time.Now()
		result, runErr := uc.toolRegistry.Execute(ctx, toolCtx, domain.ToolCall{
			Name:  "_internal_state_filter",
			Input: map[string]interface{}{"text_match": req.UserQuery},
		})
		toolMs := time.Since(toolStart).Milliseconds()
		if runErr != nil {
			guardSpan.SetError(runErr)
		} else if result != nil && result.IsError {
			guardSpan.SetAttr("is_error", true)
		}
		guardSpan.End()
		if runErr != nil {
			// Guard failed — fall through to LLM path. Log and continue.
			uc.log.Warn("agent1: deterministic guard failed; falling through to LLM", "err", runErr)
		} else {
			// Append a `user` message only — the bypass didn't go through the
			// LLM so there's no assistant:tool_use to record. Conversation
			// history stays Anthropic-compatible (no orphan tool_result).
			uc.appendConversation(ctx, req.SessionID, state.ConversationHistory, []domain.LLMMessage{
				{Role: "user", Content: req.UserQuery},
			})

			// Reload state for ProductsFound count.
			if reloaded, err := uc.state.GetState(ctx, req.SessionID); err == nil {
				state = reloaded
			}
			return &Agent1ExecuteResponse{
				ToolName:      "_internal_state_filter",
				ToolInput:     map[string]interface{}{"text_match": req.UserQuery},
				ToolResult:    result,
				ProductsFound: len(state.Current.Data.Products),
				LatencyMs:     time.Since(start).Milliseconds(),
				ToolExecuteMs: toolMs,
				StopReason:    "deterministic_guard",
				Bypassed:      true,
			}, nil
		}
	}

	// ── LLM path ──
	_, promptSpan := withSpan(ctx, "agent1.prompt")
	systemPrompt := uc.promptCache.GetOrBuild(ctx, req.TenantSlug)
	rendered := buildRenderedSubsetFromState(state)
	promptSpan.SetAttrs(map[string]any{
		"rendered_count":  len(rendered),
		"loaded_products": state.Current.Meta.ProductCount,
	})
	promptSpan.End()

	enrichedQuery := prompts.BuildAgent1ContextPrompt(state.Current.Meta, rendered, req.UserQuery)

	// Conversation history is append-only; no trim. V4 relies on caching
	// rather than truncation, and V5 prefix sizes match.
	messages := make([]domain.LLMMessage, 0, len(state.ConversationHistory)+1)
	messages = append(messages, state.ConversationHistory...)
	messages = append(messages, domain.LLMMessage{Role: "user", Content: enrichedQuery})

	// Filter the shared registry to Agent1's tools. Name prefixes:
	//   - catalog_*    → user-facing catalog actions (catalog_search)
	//   - _internal_*  → housekeeping (_internal_state_filter, _internal_history_lookup)
	allDefs := uc.toolRegistry.GetDefinitions()
	toolDefs := make([]domain.ToolDefinition, 0, len(allDefs))
	for _, d := range allDefs {
		if hasAnyPrefix(d.Name, "catalog_", "_internal_") {
			toolDefs = append(toolDefs, d)
		}
	}

	cfg := ports.CacheConfig{
		// CacheTools deliberately OFF — V5 Agent1 tool defs sum below
		// Haiku's 2048-token per-block minimum (same reasoning as V4
		// agent1_execute.go:226-231). With CacheTools on, the first
		// breakpoint is invalid and Anthropic skips the rest silently.
		CacheTools:        false,
		CacheSystem:       true,
		CacheConversation: len(messages) > 1,
		// Agent1 may legitimately decline to call a tool (style request).
		// "auto" lets the model abstain; Agent2 uses "any".
		ToolChoice: "auto",
	}

	_, llmSpan := withSpan(ctx, "agent1.llm")
	llmStart := time.Now()
	resp, err := uc.llm.ChatWithToolsCached(ctx, systemPrompt, messages, toolDefs, cfg)
	llmMs := time.Since(llmStart).Milliseconds()
	if err != nil {
		llmSpan.SetError(err)
		llmSpan.End()
		topSpan.SetError(err)
		return nil, fmt.Errorf("agent1 LLM call: %w", err)
	}
	llmSpan.SetAttrs(map[string]any{
		"model":                 resp.Usage.Model,
		"tokens.input":          resp.Usage.InputTokens,
		"tokens.output":         resp.Usage.OutputTokens,
		"tokens.cache_read":     resp.Usage.CacheReadInputTokens,
		"tokens.cache_creation": resp.Usage.CacheCreationInputTokens,
		"cost_usd":              resp.Usage.CostUSD,
		"tool_calls":            len(resp.ToolCalls),
		"stop_reason":           resp.StopReason,
	})
	llmSpan.End()

	out := &Agent1ExecuteResponse{
		Usage:         resp.Usage,
		LLMCallMs:     llmMs,
		EnrichedQuery: enrichedQuery,
		StopReason:    resp.StopReason,
	}

	// Append the user message to history regardless of tool path. We do
	// this once at the end so a panic mid-tool doesn't leave an orphan
	// assistant block in history.
	historyTail := []domain.LLMMessage{{Role: "user", Content: req.UserQuery}}

	if len(resp.ToolCalls) > 0 {
		tc := resp.ToolCalls[0] // V4 single-turn — first call only
		out.ToolName = tc.Name
		out.ToolInput = tc.Input

		_, toolSpan := withSpan(ctx, "agent1.tool."+tc.Name)
		toolSpan.SetAttr("tool_name", tc.Name)
		toolStart := time.Now()
		result, runErr := uc.runToolWithRetry(ctx, toolCtx, tc)
		toolMs := time.Since(toolStart).Milliseconds()
		if runErr != nil {
			toolSpan.SetError(runErr)
		} else if result != nil && result.IsError {
			toolSpan.SetAttr("is_error", true)
		}
		toolSpan.End()

		if runErr != nil {
			topSpan.SetError(runErr)
			return nil, fmt.Errorf("agent1 tool %s: %w", tc.Name, runErr)
		}
		out.ToolResult = result
		out.ToolExecuteMs = toolMs

		historyTail = append(historyTail,
			domain.LLMMessage{Role: "assistant", ToolCalls: []domain.ToolCall{tc}},
			domain.LLMMessage{Role: "user", ToolResult: &domain.ToolResult{
				ToolUseID: tc.ID,
				Content:   result.Content,
				IsError:   result.IsError,
			}},
		)

		// Reload state for products-found count.
		if reloaded, err := uc.state.GetState(ctx, req.SessionID); err == nil {
			out.ProductsFound = len(reloaded.Current.Data.Products) + len(reloaded.Current.Data.Services)
		}
	}

	uc.appendConversation(ctx, req.SessionID, state.ConversationHistory, historyTail)
	out.LatencyMs = time.Since(start).Milliseconds()
	return out, nil
}

// runToolWithRetry runs the tool once. On Go-error (transport failure),
// retries once with empty input — the V4 graceful-degradation pattern that
// recovers from arg-parsing crashes. Tool-side IsError responses are not
// retried; they're informational for the LLM's next turn.
func (uc *Agent1Execute) runToolWithRetry(ctx context.Context, toolCtx domain.ToolContext, tc domain.ToolCall) (*domain.ToolResult, error) {
	res, err := uc.toolRegistry.Execute(ctx, toolCtx, tc)
	if err == nil {
		return res, nil
	}
	uc.log.Warn("agent1: tool errored, retrying with empty input", "tool", tc.Name, "err", err)
	emptyCall := domain.ToolCall{ID: tc.ID, Name: tc.Name, Input: map[string]interface{}{}}
	return uc.toolRegistry.Execute(ctx, toolCtx, emptyCall)
}

// appendConversation merges the existing history with the new tail and writes
// it back. Errors are logged but not returned — conversation persistence is
// best-effort relative to the actual tool work.
func (uc *Agent1Execute) appendConversation(ctx context.Context, sessionID string, prior, tail []domain.LLMMessage) {
	full := make([]domain.LLMMessage, 0, len(prior)+len(tail))
	full = append(full, prior...)
	full = append(full, tail...)
	if err := uc.state.AppendConversation(ctx, sessionID, full); err != nil {
		uc.log.Warn("agent1: AppendConversation failed", "session", sessionID, "err", err)
	}
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return false
}

// buildRenderedSubsetFromState extracts the products currently visible to
// the user (those bound to top-level replicate clones in
// state.Current.Template) and projects them into the compact RenderedItem
// shape Agent1 sees. Returns nil when:
//   - the template has no replicates (text_explainer / fresh session) — the
//     <state> block then omits the `rendered` key entirely; or
//   - data.Products is empty — nothing to project.
//
// state.Current.Template is stored as map[string]interface{} (JSON
// round-tripped). We re-marshal into engine.Document to reuse the typed
// walker — same pattern as agent2_execute.buildFormationTreeBlock.
func buildRenderedSubsetFromState(state *domain.SessionState) []prompts.RenderedItem {
	if state == nil || len(state.Current.Template) == 0 || len(state.Current.Data.Products) == 0 {
		return nil
	}
	raw, err := json.Marshal(state.Current.Template)
	if err != nil {
		return nil
	}
	var doc engine.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	indices := engine.RenderedDataIndices(&doc)
	if len(indices) == 0 {
		return nil
	}
	return prompts.BuildRenderedSubset(state.Current.Data.Products, indices)
}
