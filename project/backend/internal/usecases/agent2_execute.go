package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/ports"
	"keepstar/internal/prompts"
	"keepstar/internal/tools"
)

// Agent2ExecuteRequest is the input for Agent 2
type Agent2ExecuteRequest struct {
	SessionID     string
	TurnID        string         // Turn ID for delta grouping
	TenantSlug    string         // Tenant context (for v2 field definitions lookup)
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
	llm            ports.LLMPort
	statePort      ports.StatePort
	toolRegistry   *tools.Registry
	log            *logger.Logger
	fieldDefPort   ports.FieldDefinitionPort // V2: field definitions (nil = v1 mode)
	promptVersion  string                    // "v1" or "v2" (from AGENT2_PROMPT_VERSION env)
}

// NewAgent2ExecuteUseCase creates Agent 2 use case
func NewAgent2ExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
	toolRegistry *tools.Registry,
	log *logger.Logger,
) *Agent2ExecuteUseCase {
	return &Agent2ExecuteUseCase{
		llm:           llm,
		statePort:     statePort,
		toolRegistry:  toolRegistry,
		log:           log,
		promptVersion: "v1",
	}
}

// NewAgent2ExecuteUseCaseV2 creates Agent 2 use case with v2 prompt support
func NewAgent2ExecuteUseCaseV2(
	llm ports.LLMPort,
	statePort ports.StatePort,
	toolRegistry *tools.Registry,
	log *logger.Logger,
	fieldDefPort ports.FieldDefinitionPort,
) *Agent2ExecuteUseCase {
	version := os.Getenv("AGENT2_PROMPT_VERSION")
	if version == "" {
		version = "v1"
	}
	return &Agent2ExecuteUseCase{
		llm:           llm,
		statePort:     statePort,
		toolRegistry:  toolRegistry,
		log:           log,
		fieldDefPort:  fieldDefPort,
		promptVersion: version,
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
					ID:   "no-results",
					Type: domain.WidgetTypeTextBlock,
					Atoms: []domain.Atom{
						{
							Type:    domain.AtomTypeText,
							Subtype: domain.SubtypeString,
							Display: "h3",
							Slot:    domain.AtomSlotTitle,
							Value:   "Ничего не найдено",
						},
						{
							Type:    domain.AtomTypeText,
							Subtype: domain.SubtypeString,
							Display: "body-sm",
							Slot:    domain.AtomSlotSecondary,
							Value:   "Попробуйте изменить запрос или уточнить категорию",
						},
						{
							Type:    domain.AtomTypeText,
							Subtype: domain.SubtypeString,
							Display: "tag",
							Slot:    domain.AtomSlotSecondary,
							Value:   "Показать все товары",
							Meta:    map[string]interface{}{"action": "show_all"},
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

	// Extract current RenderConfig from formation (what is on screen now)
	var currentConfig *domain.RenderConfig
	if state.Current.Template != nil {
		if formationData, ok := state.Current.Template["formation"]; ok {
			if f, ok := formationData.(*domain.FormationWithData); ok && f != nil && f.Config != nil {
				currentConfig = f.Config
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

	// Build user message — v1 or v2 depending on prompt version
	var userPrompt string
	var systemPrompt string

	if uc.promptVersion == "v2" {
		systemPrompt = prompts.Agent2ToolSystemPromptV2
		// Load field labels from field_definitions for v2 context
		fieldLabels := uc.loadFieldLabels(ctx, req.TenantSlug, state)
		userPrompt = prompts.BuildAgent2ToolPromptV2(state.Current.Meta, state.View, req.UserQuery, dataDelta, currentConfig, allDeltas, req.Microcontext, screenCtx, fieldLabels)
	} else {
		systemPrompt = prompts.Agent2ToolSystemPrompt
		userPrompt = prompts.BuildAgent2ToolPrompt(state.Current.Meta, state.View, req.UserQuery, dataDelta, currentConfig, allDeltas, req.Microcontext, screenCtx)
	}

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
			CacheTools:  true,
			CacheSystem: true,
			ToolChoice:  "any", // Force tool call — Agent2 must always render
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
		MetaCount:         state.Current.Meta.Count,
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

// loadFieldLabels loads field name → label mapping from field_definitions for v2 prompt context.
func (uc *Agent2ExecuteUseCase) loadFieldLabels(ctx context.Context, tenantSlug string, state *domain.SessionState) map[string]string {
	if uc.fieldDefPort == nil || tenantSlug == "" {
		return nil
	}

	labels := make(map[string]string)

	// Load product field labels
	if state.Current.Meta.ProductCount > 0 {
		defs, err := uc.fieldDefPort.ListFieldDefinitions(ctx, tenantSlug, domain.EntityTypeProduct)
		if err != nil {
			uc.log.Warn("failed to load product field definitions for v2 prompt", "error", err)
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
			uc.log.Warn("failed to load service field definitions for v2 prompt", "error", err)
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

// Note: convertToFormation is defined in pipeline_execute.go and reused here
