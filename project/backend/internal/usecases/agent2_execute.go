package usecases

import (
	"context"
	"fmt"
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
	SessionID string
	TurnID    string // Turn ID for delta grouping
	UserQuery string // User's original query (for style selection)
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
}

// Agent2ExecuteUseCase executes Agent 2 (Preset Selector)
type Agent2ExecuteUseCase struct {
	llm          ports.LLMPort
	statePort    ports.StatePort
	toolRegistry *tools.Registry
	log          *logger.Logger
}

// NewAgent2ExecuteUseCase creates Agent 2 use case
func NewAgent2ExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
	toolRegistry *tools.Registry,
	log *logger.Logger,
) *Agent2ExecuteUseCase {
	return &Agent2ExecuteUseCase{
		llm:          llm,
		statePort:    statePort,
		toolRegistry: toolRegistry,
		log:          log,
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
					Atoms: []domain.Atom{{
						Type:    domain.AtomTypeText,
						Subtype: domain.SubtypeString,
						Slot:    domain.AtomSlotTitle,
						Value:   "К сожалению, по вашему запросу ничего не найдено",
					}},
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

	// Build user message with view context, user query, and data delta
	userPrompt := prompts.BuildAgent2ToolPrompt(state.Current.Meta, state.View, req.UserQuery, dataDelta)

	// Include recent user queries from conversation history for context (last 4 user messages max).
	// Only take user messages with Content (skip assistant, tool_use, tool_result).
	// Agent1 ConversationHistory contains tool_use/tool_result blocks from Agent1's tools,
	// which would confuse Agent2 (it has different tools: render_* and freestyle).
	var messages []domain.LLMMessage
	if len(state.ConversationHistory) > 0 {
		var userMessages []domain.LLMMessage
		for _, msg := range state.ConversationHistory {
			if msg.Role == "user" && msg.Content != "" && msg.ToolResult == nil {
				userMessages = append(userMessages, msg)
			}
		}
		historyLimit := 4
		start := len(userMessages) - historyLimit
		if start < 0 {
			start = 0
		}
		messages = append(messages, userMessages[start:]...)
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
		prompts.Agent2ToolSystemPrompt,
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
		Usage:      llmResp.Usage,
		LatencyMs:  int(time.Since(start).Milliseconds()),
		LLMCallMs:  llmDuration,
		PromptSent: userPrompt,
		MetaCount:  state.Current.Meta.Count,
		MetaFields: state.Current.Meta.Fields,
	}

	// Execute tool calls — tools create deltas via zone-write internally
	for _, toolCall := range llmResp.ToolCalls {
		response.ToolCalled = true
		response.ToolName = toolCall.Name

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
			SessionID: req.SessionID,
			TurnID:    req.TurnID,
			ActorID:   "agent2",
		}, toolCall)
		toolDuration := time.Since(toolStart).Milliseconds()
		if endToolSpan != nil {
			endToolSpan(toolCall.Name)
		}

		if err != nil {
			uc.log.Error("tool_execution_failed", "error", err, "tool", toolCall.Name, "actor", "agent2")
			return nil, fmt.Errorf("execute tool %s: %w", toolCall.Name, err)
		}

		uc.log.ToolExecuted(toolCall.Name, req.SessionID, result.Content, toolDuration)

		response.RawResponse = result.Content

		// Tool writes formation to state
		if result.IsError {
			return nil, fmt.Errorf("tool error: %s", result.Content)
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

// getAgent2Tools returns render_* and freestyle tools for Agent 2
func (uc *Agent2ExecuteUseCase) getAgent2Tools() []domain.ToolDefinition {
	allTools := uc.toolRegistry.GetDefinitions()
	var agent2Tools []domain.ToolDefinition
	for _, t := range allTools {
		if strings.HasPrefix(t.Name, "render_") || t.Name == "freestyle" {
			agent2Tools = append(agent2Tools, t)
		}
	}
	return agent2Tools
}

// Note: convertToFormation is defined in pipeline_execute.go and reused here
