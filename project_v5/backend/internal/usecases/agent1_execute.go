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
)

// filterTriggers matches user queries that imply filtering existing data
// (price / quantity / subset markers in Russian + English).
var filterTriggers = regexp.MustCompile(`(?i)(только|лишь|оставь|исключи|дешевле|дороже|выше|ниже|от \d|до \d)`)

// styleFieldNames matches queries about display fields — "убери <field>"
// is a STYLE request that should NOT trigger the deterministic guard.
var styleFieldNames = regexp.MustCompile(`(?i)(описани|рейтинг|бренд|цен[уыа]|фото|картинк|назван|изображ|тег|катего|рейт|rating|brand|price|image|name|description|tag)`)

// maxOnboardingOps caps the onboarding form's multi-op turn (R4): the
// onboarding agent executes ALL emitted tool calls sequentially, at most 8,
// with ONE LLM call per turn — no continuation call. Storefront/CRM keep
// the single-call fast path.
const maxOnboardingOps = 8

// Agent1ExecuteRequest is the per-turn input handed to Agent1. Mode, Role
// and ActorID arrive from the session row via the pipeline handler (R17);
// empty values default to the storefront form / visitor role.
type Agent1ExecuteRequest struct {
	SessionID  string
	TenantSlug string
	UserQuery  string
	TurnID     string
	Mode       domain.PipelineMode
	Role       domain.Role
	ActorID    string
}

// Agent1ExecuteResponse carries everything the pipeline orchestrator needs
// to compose a microcontext signal for Agent2 plus surface diagnostics.
type Agent1ExecuteResponse struct {
	ToolName      string                    // first executed operation (empty when none — style request)
	ToolInput     map[string]interface{}    // first call's raw input as the LLM emitted it
	ToolCalls     []domain.ToolCall         // every executed call, in emission order
	Results       []*domain.OperationResult // structured results, aligned with ToolCalls
	ToolResult    *domain.ToolResult        // first result bridged to the legacy shape (nil when no op ran)
	ProductsFound int                       // for the microcontext signal
	Usage         domain.LLMUsage
	LatencyMs     int64
	LLMCallMs     int64
	ToolExecuteMs int64
	StopReason    string
	EnrichedQuery string // <state>+query as actually sent to LLM
	Bypassed      bool   // true when deterministic guard fired
}

// Agent1Execute runs one Agent1 turn — single LLM call with a
// deterministic state-filter guard for obvious filter queries.
//
//  1. GetState
//  2. If data is loaded AND query matches filterTriggers AND NOT styleFieldNames →
//     bypass LLM, run _internal_state_filter through the registry directly
//  3. Otherwise build system prompt (cached) + enriched user query, call LLM
//     with registry.DefinitionsFor(tenant, form, data-plane, role)
//  4. Execute the returned tool calls through registry.Execute — the first
//     call only on storefront/crm (V4 fast path), ALL calls (cap 8) on the
//     onboarding form (R4); retry once with empty input on Go-error
//  5. Append user / assistant:tool_use / user:tool_result to ConversationHistory
type Agent1Execute struct {
	llm         ports.LLMPort
	state       ports.StatePort
	catalog     ports.CatalogPort
	registry    ports.OperationRegistry
	promptCache *Agent1PromptCache
	log         *slog.Logger
}

// NewAgent1Execute wires the use case. All deps required.
func NewAgent1Execute(
	llm ports.LLMPort,
	state ports.StatePort,
	catalog ports.CatalogPort,
	registry ports.OperationRegistry,
	promptCache *Agent1PromptCache,
	log *slog.Logger,
) *Agent1Execute {
	return &Agent1Execute{
		llm:         llm,
		state:       state,
		catalog:     catalog,
		registry:    registry,
		promptCache: promptCache,
		log:         log,
	}
}

// Execute runs one Agent1 turn end-to-end.
func (uc *Agent1Execute) Execute(ctx context.Context, req Agent1ExecuteRequest) (*Agent1ExecuteResponse, error) {
	start := time.Now()
	ctx, topSpan := withSpan(ctx, "agent1.execute")
	defer topSpan.End()

	mode := req.Mode
	if mode == "" {
		mode = domain.ModeStorefront
	}
	role := req.Role
	if role == "" {
		role = domain.RoleVisitor
	}

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

	octx := domain.OperationContext{
		SessionID:  req.SessionID,
		TenantSlug: req.TenantSlug,
		TurnID:     req.TurnID,
		Mode:       mode,
		Role:       role,
		ActorID:    req.ActorID,
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
		guardInput := map[string]interface{}{"text_match": req.UserQuery}
		toolStart := time.Now()
		result, runErr := uc.registry.Execute(ctx, octx, domain.ToolCall{
			Name:  "_internal_state_filter",
			Input: guardInput,
		})
		toolMs := time.Since(toolStart).Milliseconds()
		if runErr != nil {
			guardSpan.SetError(runErr)
		} else if result != nil && result.Outcome != domain.OutcomeOK && result.Outcome != domain.OutcomeEmpty {
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
			productsFound := len(state.Current.Data.Products)
			stampStateCounts([]*domain.OperationResult{result}, productsFound)
			return &Agent1ExecuteResponse{
				ToolName:      "_internal_state_filter",
				ToolInput:     guardInput,
				ToolCalls:     []domain.ToolCall{{Name: "_internal_state_filter", Input: guardInput}},
				Results:       []*domain.OperationResult{result},
				ToolResult:    result.ToToolResult(""),
				ProductsFound: productsFound,
				LatencyMs:     time.Since(start).Milliseconds(),
				ToolExecuteMs: toolMs,
				StopReason:    "deterministic_guard",
				Bypassed:      true,
			}, nil
		}
	}

	// ── LLM path ──
	_, promptSpan := withSpan(ctx, "agent1.prompt")
	// Form-keyed prompt selection (R17): unregistered forms fall back to
	// the storefront base, so pre-mode sessions stay byte-identical.
	systemPrompt := uc.promptCache.GetOrBuildForm(ctx, req.TenantSlug, req.Mode)
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

	// Registry-scoped visibility (R8): the operations visible to Agent1 for
	// this tenant + form + role, per-tenant schemas already materialized
	// (catalog_search's digest-derived filter schema arrives from the
	// executor's SpecForTenant). Sort is byte-stable. CacheTools is off for
	// Agent1, so a per-tenant schema does not fragment the Anthropic prompt
	// cache.
	toolDefs := uc.registry.DefinitionsFor(ctx, req.TenantSlug, mode, domain.AgentData, role)

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
		// Multi-op turn (R4): ONLY the onboarding form executes every
		// emitted call, sequentially, cap 8. Storefront/CRM keep the V4
		// single-call fast path.
		calls := resp.ToolCalls
		if mode != domain.ModeOnboarding {
			calls = calls[:1]
		} else if len(calls) > maxOnboardingOps {
			uc.log.Warn("agent1: onboarding turn exceeded op cap — truncating", "emitted", len(calls), "cap", maxOnboardingOps)
			calls = calls[:maxOnboardingOps]
		}

		for _, tc := range calls {
			_, toolSpan := withSpan(ctx, "agent1.tool."+tc.Name)
			toolSpan.SetAttr("tool_name", tc.Name)
			toolStart := time.Now()
			result, runErr := uc.runOpWithRetry(ctx, octx, tc)
			toolMs := time.Since(toolStart).Milliseconds()
			if runErr != nil {
				toolSpan.SetError(runErr)
			} else if result != nil && result.Outcome != domain.OutcomeOK && result.Outcome != domain.OutcomeEmpty {
				toolSpan.SetAttr("is_error", true)
			}
			toolSpan.End()

			if runErr != nil {
				topSpan.SetError(runErr)
				return nil, fmt.Errorf("agent1 tool %s: %w", tc.Name, runErr)
			}
			out.ToolCalls = append(out.ToolCalls, tc)
			out.Results = append(out.Results, result)
			out.ToolExecuteMs += toolMs

			// History bridges the structured result to the legacy tool_result
			// shape WITHOUT metadata — exactly the bytes the pre-registry
			// path persisted, so conversation-prefix caches stay warm.
			bridged := result.ToToolResult(tc.ID)
			historyTail = append(historyTail,
				domain.LLMMessage{Role: "assistant", ToolCalls: []domain.ToolCall{tc}},
				domain.LLMMessage{Role: "user", ToolResult: &domain.ToolResult{
					ToolUseID: bridged.ToolUseID,
					Content:   bridged.Content,
					IsError:   bridged.IsError,
				}},
			)
		}

		out.ToolName = out.ToolCalls[0].Name
		out.ToolInput = out.ToolCalls[0].Input
		out.ToolResult = out.Results[0].ToToolResult(out.ToolCalls[0].ID)

		// Reload state for products-found count.
		if reloaded, err := uc.state.GetState(ctx, req.SessionID); err == nil {
			out.ProductsFound = len(reloaded.Current.Data.Products) + len(reloaded.Current.Data.Services)
		}
		stampStateCounts(out.Results, out.ProductsFound)
	}

	uc.appendConversation(ctx, req.SessionID, state.ConversationHistory, historyTail)
	out.LatencyMs = time.Since(start).Milliseconds()
	return out, nil
}

// stampStateCounts writes the reloaded state count onto the legacy
// state-zone results. The pre-registry microcontext derived its counts from
// ProductsFound (stale data preserved on an empty search — V4 semantics);
// the legacy wraps don't know the state count, so the stamp keeps
// composeMicrocontext byte-compatible. Native executors (entity queries)
// set their own Count and are left untouched.
func stampStateCounts(results []*domain.OperationResult, productsFound int) {
	for _, r := range results {
		if r == nil {
			continue
		}
		if (r.Kind == domain.KindQuery && r.EntityKind == "") || r.Operation == "_internal_state_filter" {
			r.Count = productsFound
		}
	}
}

// runOpWithRetry runs the operation once. On Go-error (transport failure),
// retries once with empty input — the V4 graceful-degradation pattern that
// recovers from arg-parsing crashes. Structured invalid/denied results are
// not retried; they're informational for the LLM's next turn.
func (uc *Agent1Execute) runOpWithRetry(ctx context.Context, octx domain.OperationContext, tc domain.ToolCall) (*domain.OperationResult, error) {
	res, err := uc.registry.Execute(ctx, octx, tc)
	if err == nil {
		return res, nil
	}
	uc.log.Warn("agent1: tool errored, retrying with empty input", "tool", tc.Name, "err", err)
	emptyCall := domain.ToolCall{ID: tc.ID, Name: tc.Name, Input: map[string]interface{}{}}
	return uc.registry.Execute(ctx, octx, emptyCall)
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
