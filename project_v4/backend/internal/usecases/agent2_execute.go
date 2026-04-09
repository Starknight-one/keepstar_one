package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"keepstar_v4/internal/domain"
	engine_v4 "keepstar_v4/internal/engine_v4"
	"keepstar_v4/internal/logger"
	"keepstar_v4/internal/ports"
	"keepstar_v4/internal/prompts"
	"keepstar_v4/internal/tools"
)

// Agent2ExecuteRequest is the input for Agent 2
type Agent2ExecuteRequest struct {
	SessionID     string
	TurnID        string         // Turn ID for delta grouping
	TenantSlug    string         // Tenant context (for field definitions lookup)
	UserQuery     string         // User's original query (for style selection)
	Microcontext  string         // Pipeline-generated context signal (e.g. "new_search: 23 items found")
	ScreenContext *ScreenContext  // Current UI state from frontend
}

// Agent2ExecuteResponse is the output from Agent 2
type Agent2ExecuteResponse struct {
	Template   *domain.FormationTemplate
	Formation  *domain.FormationWithData // New: formation built by tool
	Usage      domain.LLMUsage
	LatencyMs  int
	ToolCalled bool   // Whether a tool was called
	ToolName   string // Name of the tool called
	// Detailed timing and data
	LLMCallMs    int64    `json:"llmCallMs"`
	PromptSent   string   `json:"promptSent"`
	RawResponse  string   `json:"rawResponse"`
	TemplateJSON string   `json:"templateJson"`
	MetaCount    int      `json:"metaCount"`
	MetaFields   []string `json:"metaFields"`
	// Trace enrichment
	SystemPrompt      string                 `json:"systemPrompt,omitempty"`
	SystemPromptChars int                    `json:"systemPromptChars,omitempty"`
	ToolInput         string                 `json:"toolInput,omitempty"`
	ToolBreakdown     map[string]interface{} `json:"toolBreakdown,omitempty"`
	MessageCount      int                    `json:"messageCount,omitempty"`
	ToolDefCount      int                    `json:"toolDefCount,omitempty"`
}

// Agent2ExecuteUseCase executes Agent 2 (Preset Selector)
type Agent2ExecuteUseCase struct {
	llm          ports.LLMPort
	statePort    ports.StatePort
	toolRegistry *tools.Registry
	log          *logger.Logger
	fieldDefPort ports.FieldDefinitionPort // field definitions
	// fieldsPromptCache memoizes the per-tenant system prompt with the
	// <fields> block already appended. Mirrors Agent1's digestCache so the
	// system prompt stays byte-stable across turns (required for Anthropic
	// prompt caching). Invalidated by process restart.
	fieldsPromptCache sync.Map // key: tenantSlug (string) → value: systemPrompt (string)
}

// NewAgent2ExecuteUseCase creates Agent 2 use case with field definitions support
func NewAgent2ExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
	toolRegistry *tools.Registry,
	log *logger.Logger,
	fieldDefPort ports.FieldDefinitionPort,
) *Agent2ExecuteUseCase {
	return &Agent2ExecuteUseCase{
		llm:          llm,
		statePort:    statePort,
		toolRegistry: toolRegistry,
		log:          log,
		fieldDefPort: fieldDefPort,
	}
}

// Execute runs Agent 2: meta → LLM (tools) → render tool → formation in state
func (uc *Agent2ExecuteUseCase) Execute(ctx context.Context, req Agent2ExecuteRequest) (*Agent2ExecuteResponse, error) {
	start := time.Now()

	// Span instrumentation
	sc := domain.SpanFromContext(ctx)
	if sc != nil {
		endAgent := sc.Start("agent2")
		defer endAgent()
	}
	ctx = domain.WithStage(ctx, "agent2")

	// Get current state (must exist after Agent 1)
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Update meta counts
	state.Current.Meta.ProductCount = len(state.Current.Data.Products)
	state.Current.Meta.ServiceCount = len(state.Current.Data.Services)

	// Check if we have data — no data means nothing to render
	if state.Current.Meta.ProductCount == 0 && state.Current.Meta.ServiceCount == 0 {
		return &Agent2ExecuteResponse{
			Formation: &domain.FormationWithData{
				Mode: domain.FormationTypeSingle,
				Widgets: []domain.Widget{{
					ID: "no-results",
					Atoms: []domain.Atom{
						{
							Type:    domain.AtomTypeText,
							Subtype: domain.SubtypeString,
							Slot:    domain.AtomSlotTitle,
							Value:   "Ничего не найдено",
							TextStyle: &domain.TextStyle{
								FontSize:   "lg",
								FontWeight: "semibold",
							},
						},
						{
							Type:    domain.AtomTypeText,
							Subtype: domain.SubtypeString,
							Slot:    domain.AtomSlotSecondary,
							Value:   "Попробуйте изменить запрос или уточнить категорию",
							TextStyle: &domain.TextStyle{
								FontSize: "sm",
							},
						},
						{
							Type:    domain.AtomTypeText,
							Subtype: domain.SubtypeString,
							Slot:    domain.AtomSlotSecondary,
							Value:   "Показать все товары",
							Wrapper: &domain.WrapperConfig{
								Type:    "tag",
								Variant: "default",
							},
							Meta: map[string]interface{}{"action": "show_all"},
						},
					},
				}},
			},
			LatencyMs: int(time.Since(start).Milliseconds()),
		}, nil
	}

	// Get data delta for current turn (for Agent2 context)
	var dataDelta *domain.Delta
	if req.TurnID != "" {
		deltas, _ := uc.statePort.GetDeltasSince(ctx, req.SessionID, 0)
		for i := len(deltas) - 1; i >= 0; i-- {
			if deltas[i].TurnID == req.TurnID && strings.HasPrefix(deltas[i].Path, "data.") {
				dataDelta = &deltas[i]
				break
			}
		}
	}

	// Extract current RenderConfig and formation tree from state (what is on screen now)
	var currentConfig *domain.RenderConfig
	var formationTree map[string]interface{}
	if state.Current.Template != nil {
		if formationData, ok := state.Current.Template["formation"]; ok {
			var f *domain.FormationWithData
			if typed, ok := formationData.(*domain.FormationWithData); ok {
				f = typed
			} else {
				// After DB roundtrip: map[string]interface{} → convert via JSON
				f = convertToFormation(formationData)
			}
			if f != nil {
				if f.Config != nil {
					currentConfig = f.Config
				}
				formationTree = engine_v4.BuildTreeMap(f)
			}
		}
	}

	// Load all deltas for history summary
	allDeltas, _ := uc.statePort.GetDeltas(ctx, req.SessionID)

	// Build screen context for prompt
	var screenCtx *prompts.ScreenContext
	if req.ScreenContext != nil {
		screenCtx = &prompts.ScreenContext{
			Mode:        req.ScreenContext.Mode,
			WidgetCount: req.ScreenContext.WidgetCount,
			Fields:      req.ScreenContext.Fields,
		}
	}

	// Build user message and system prompt. The system prompt now includes
	// a per-tenant <fields> block so Agent2 can match atom slots against
	// the tenant's actual data fields (metadata-driven binding, see
	// docs/New features/METADATA_DRIVEN_BINDING_2026-04-09.md).
	systemPrompt := uc.buildSystemPromptWithFields(ctx, req.TenantSlug)
	// Load field labels from field_definitions for context
	fieldLabels := uc.loadFieldLabels(ctx, req.TenantSlug, state)
	userPrompt := prompts.BuildAgent2ToolPrompt(state.Current.Meta, state.View, req.UserQuery, dataDelta, currentConfig, allDeltas, req.Microcontext, screenCtx, fieldLabels, formationTree)

	// Agent2's own tool call history (last 2 turns = 4 messages: assistant:tool_use + user:tool_result × 2).
	// This replaces the old approach of including Agent1's text user messages which caused stale parameters.
	var messages []domain.LLMMessage
	if len(state.Agent2History) > 0 {
		historyLimit := 4 // 2 turns × 2 messages (assistant:tool_use + user:tool_result)
		start := len(state.Agent2History) - historyLimit
		if start < 0 {
			start = 0
		}
		messages = append(messages, state.Agent2History[start:]...)
	}
	messages = append(messages, domain.LLMMessage{
		Role:    "user",
		Content: userPrompt,
	})

	// Get render tool definitions (filter only render_* tools)
	toolDefs := uc.getAgent2Tools()

	// Call LLM with caching and forced tool use
	llmStart := time.Now()
	llmResp, err := uc.llm.ChatWithToolsCached(
		ctx,
		systemPrompt,
		messages,
		toolDefs,
		&ports.CacheConfig{
			CacheTools:        true,
			CacheSystem:       true,
			CacheConversation: len(messages) > 1, // also cache Agent2History when present
			ToolChoice:        "any",              // Force tool call — Agent2 must always render
		},
	)
	llmDuration := time.Since(llmStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("llm call: %w", err)
	}

	// Log LLM usage with cache metrics
	uc.log.LLMUsageWithCache(
		"agent2",
		llmResp.Usage.Model,
		llmResp.Usage.InputTokens,
		llmResp.Usage.OutputTokens,
		llmResp.Usage.CacheCreationInputTokens,
		llmResp.Usage.CacheReadInputTokens,
		llmResp.Usage.CostUSD,
		llmDuration,
	)

	response := &Agent2ExecuteResponse{
		Usage:             llmResp.Usage,
		LatencyMs:         int(time.Since(start).Milliseconds()),
		LLMCallMs:         llmDuration,
		PromptSent:        userPrompt,
		MetaCount:         state.Current.Meta.ProductCount + state.Current.Meta.ServiceCount,
		MetaFields:        state.Current.Meta.Fields,
		SystemPrompt:      systemPrompt,
		SystemPromptChars: len(systemPrompt),
		MessageCount:      len(messages),
		ToolDefCount:      len(toolDefs),
	}

	// Execute tool calls — tools create deltas via zone-write internally
	for _, toolCall := range llmResp.ToolCalls {
		response.ToolCalled = true
		response.ToolName = toolCall.Name

		// Capture tool input for tracing
		if inputJSON, err := json.Marshal(toolCall.Input); err == nil {
			response.ToolInput = string(inputJSON)
		}

		uc.log.Debug("tool_call_received",
			"tool", toolCall.Name,
			"input", toolCall.Input,
			"session_id", req.SessionID,
			"actor", "agent2",
		)

		var endToolSpan func(...string)
		if sc != nil {
			endToolSpan = sc.Start("agent2.tool")
		}
		toolStart := time.Now()
		result, err := uc.toolRegistry.Execute(ctx, tools.ToolContext{
			SessionID:  req.SessionID,
			TurnID:     req.TurnID,
			ActorID:    "agent2",
			TenantSlug: req.TenantSlug,
			UserQuery:  req.UserQuery,
		}, toolCall)
		toolDuration := time.Since(toolStart).Milliseconds()
		if endToolSpan != nil {
			endToolSpan(toolCall.Name)
		}

		if err != nil {
			uc.log.Error("tool_execution_failed", "error", err, "tool", toolCall.Name, "actor", "agent2")
			// Graceful degradation: retry with no parameters
			fallbackResult, fallbackErr := uc.toolRegistry.Execute(ctx, tools.ToolContext{
				SessionID:  req.SessionID,
				TurnID:     req.TurnID,
				ActorID:    "agent2",
				TenantSlug: req.TenantSlug,
				UserQuery:  req.UserQuery,
			}, domain.ToolCall{Name: "visual_assembly", Input: map[string]interface{}{}})
			if fallbackErr != nil {
				return nil, fmt.Errorf("execute tool %s (fallback also failed): %w", toolCall.Name, err)
			}
			result = fallbackResult
			uc.log.Info("graceful_degradation", "session_id", req.SessionID, "original_error", err.Error())
		}

		uc.log.ToolExecuted(toolCall.Name, req.SessionID, result.Content, toolDuration)

		response.RawResponse = result.Content
		if result.Metadata != nil {
			response.ToolBreakdown = result.Metadata
		}

		// Tool writes formation to state
		if result.IsError {
			return nil, fmt.Errorf("tool error: %s", result.Content)
		}

		// Save Agent2's tool call + result for multi-turn history
		agent2Messages := append(state.Agent2History,
			domain.LLMMessage{Role: "assistant", ToolCalls: llmResp.ToolCalls},
			domain.LLMMessage{
				Role: "user",
				ToolResult: &domain.ToolResult{
					ToolUseID: toolCall.ID,
					Content:   result.Content,
				},
			},
		)
		if err := uc.statePort.AppendAgent2History(ctx, req.SessionID, agent2Messages); err != nil {
			uc.log.Error("append_agent2_history_failed", "error", err)
		}
	}

	// Get formation from state (built by tool)
	state, err = uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state after tool: %w", err)
	}

	if formationData, ok := state.Current.Template["formation"]; ok {
		// Try direct type assertion first (in-memory)
		if formation, ok := formationData.(*domain.FormationWithData); ok {
			response.Formation = formation
		} else {
			// After DB read, it's map[string]interface{} - convert via JSON
			response.Formation = convertToFormation(formationData)
		}
	}

	return response, nil
}

// getAgent2Tools returns visual_* tools for Agent 2
func (uc *Agent2ExecuteUseCase) getAgent2Tools() []domain.ToolDefinition {
	allTools := uc.toolRegistry.GetDefinitions()
	var agent2Tools []domain.ToolDefinition
	for _, t := range allTools {
		if strings.HasPrefix(t.Name, "visual_") {
			agent2Tools = append(agent2Tools, t)
		}
	}
	return agent2Tools
}

// loadFieldLabels loads field name → label mapping from field_definitions for prompt context.
func (uc *Agent2ExecuteUseCase) loadFieldLabels(ctx context.Context, tenantSlug string, state *domain.SessionState) map[string]string {
	if uc.fieldDefPort == nil || tenantSlug == "" {
		return nil
	}

	labels := make(map[string]string)

	// Load product field labels
	if state.Current.Meta.ProductCount > 0 {
		defs, err := uc.fieldDefPort.ListFieldDefinitions(ctx, tenantSlug, domain.EntityTypeProduct)
		if err != nil {
			uc.log.Warn("failed to load product field definitions for prompt", "error", err)
		} else {
			for _, d := range defs {
				if d.Label != "" {
					labels[d.FieldName] = d.Label
				}
			}
		}
	}

	// Load service field labels
	if state.Current.Meta.ServiceCount > 0 {
		defs, err := uc.fieldDefPort.ListFieldDefinitions(ctx, tenantSlug, domain.EntityTypeService)
		if err != nil {
			uc.log.Warn("failed to load service field definitions for prompt", "error", err)
		} else {
			for _, d := range defs {
				if d.Label != "" {
					labels[d.FieldName] = d.Label
				}
			}
		}
	}

	if len(labels) == 0 {
		return nil
	}
	return labels
}

// buildSystemPromptWithFields returns the base Agent2 system prompt with a
// per-tenant <fields> block appended. The result is memoized per tenantSlug
// in fieldsPromptCache so it stays byte-stable across turns (required for
// Anthropic prompt caching). On missing tenant, missing port, empty field
// definitions, or any error, returns the base prompt unchanged — the LLM
// still has presets and hey-babes examples to fall back on.
//
// Called from Execute in place of the static prompts.Agent2ToolSystemPrompt
// constant. Symmetric to Agent1ExecuteUseCase.buildSystemPromptWithDigest.
func (uc *Agent2ExecuteUseCase) buildSystemPromptWithFields(ctx context.Context, tenantSlug string) string {
	if tenantSlug == "" || uc.fieldDefPort == nil {
		return prompts.Agent2ToolSystemPrompt
	}
	if cached, ok := uc.fieldsPromptCache.Load(tenantSlug); ok {
		return cached.(string)
	}

	fields, err := uc.fieldDefPort.ListFieldDefinitions(ctx, tenantSlug, domain.EntityTypeProduct)
	if err != nil || len(fields) == 0 {
		return prompts.Agent2ToolSystemPrompt
	}
	// Samples are best-effort enrichment. A failure or empty result is fine —
	// the block will still carry label/type/slot info which is the minimum
	// the LLM needs to reason about field binding.
	samples, _ := uc.fieldDefPort.SampleFieldValues(ctx, tenantSlug, domain.EntityTypeProduct, 3)
	block := formatFieldsBlock(fields, samples)
	if block == "" {
		return prompts.Agent2ToolSystemPrompt
	}

	full := prompts.Agent2ToolSystemPrompt + "\n\n" + block + "\n"
	uc.fieldsPromptCache.Store(tenantSlug, full)
	return full
}

// formatFieldsBlock renders a compact <fields entity="product">...</fields>
// text block for injection into Agent2's system prompt. One line per field,
// ordered by priority ASC (most important first).
//
// Example output:
//
//	<fields entity="product">
//	images         image   label="Images"         samples=["https://..."]
//	name           text    label="Name"           samples=["COSRX BHA Power","Laneige Moisture"]
//	price          number  label="Price"          samples=[2490,3990,1200]
//	brand          text    label="Brand"          samples=["COSRX","Laneige"]
//	...
//	</fields>
//
// Fields without samples still appear (label+type are informative alone).
// Fields with samples get them JSON-encoded so the LLM sees the data shape.
func formatFieldsBlock(fields []ports.FieldDefinition, samples map[string][]interface{}) string {
	if len(fields) == 0 {
		return ""
	}
	// Copy + sort by priority (lower = more important)
	sorted := make([]ports.FieldDefinition, len(fields))
	copy(sorted, fields)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	var b strings.Builder
	b.WriteString(`<fields entity="product">`)
	b.WriteByte('\n')
	for _, fd := range sorted {
		b.WriteString(fd.FieldName)
		// Pad field name to a consistent column width for readability.
		if pad := 16 - len(fd.FieldName); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		} else {
			b.WriteByte(' ')
		}

		// Type descriptor — atom_type + subtype gives more info than type alone
		// (e.g. "number/currency" vs "number/rating").
		typeStr := string(fd.AtomType)
		if fd.AtomSubtype != "" && string(fd.AtomSubtype) != typeStr {
			typeStr = typeStr + "/" + string(fd.AtomSubtype)
		}
		b.WriteString(typeStr)
		if pad := 18 - len(typeStr); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		} else {
			b.WriteByte(' ')
		}

		// Label in quotes for human context.
		if fd.Label != "" {
			b.WriteString(`label=`)
			if labelJSON, err := json.Marshal(fd.Label); err == nil {
				b.Write(labelJSON)
			}
			b.WriteString("  ")
		}

		// Unit if present (e.g. "RUB" for price).
		if fd.Unit != "" {
			b.WriteString(`unit="`)
			b.WriteString(fd.Unit)
			b.WriteString(`"  `)
		}

		// Default slot hints where this field "wants" to live.
		if fd.DefaultSlot != "" {
			b.WriteString(`slot=`)
			b.WriteString(string(fd.DefaultSlot))
			b.WriteString("  ")
		}

		// Samples — JSON-encoded array of the first few real values.
		if vals, ok := samples[fd.FieldName]; ok && len(vals) > 0 {
			if sampleJSON, err := json.Marshal(vals); err == nil {
				b.WriteString(`samples=`)
				b.Write(sampleJSON)
			}
		}

		b.WriteByte('\n')
	}
	b.WriteString(`</fields>`)
	return b.String()
}

// Note: convertToFormation is defined in pipeline_execute.go and reused here
