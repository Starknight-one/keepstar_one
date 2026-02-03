package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
	"keepstar/internal/prompts"
	"keepstar/internal/tools"
)

// extractJSON extracts JSON from LLM response, handling markdown code blocks
func extractJSON(response string) string {
	response = strings.TrimSpace(response)

	// Check for ```json ... ``` block
	if strings.HasPrefix(response, "```") {
		// Find the end of the first line (after ```json or ```)
		firstNewline := strings.Index(response, "\n")
		if firstNewline == -1 {
			return response
		}

		// Find the closing ```
		lastBackticks := strings.LastIndex(response, "```")
		if lastBackticks > firstNewline {
			return strings.TrimSpace(response[firstNewline+1 : lastBackticks])
		}
	}

	return response
}

// Agent2ExecuteRequest is the input for Agent 2
type Agent2ExecuteRequest struct {
	SessionID  string
	LayoutHint string // Optional hint for layout
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
}

// NewAgent2ExecuteUseCase creates Agent 2 use case
func NewAgent2ExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
	toolRegistry *tools.Registry,
) *Agent2ExecuteUseCase {
	return &Agent2ExecuteUseCase{
		llm:          llm,
		statePort:    statePort,
		toolRegistry: toolRegistry,
	}
}

// Execute runs Agent 2: meta → LLM (tools) → render tool → formation in state
func (uc *Agent2ExecuteUseCase) Execute(ctx context.Context, req Agent2ExecuteRequest) (*Agent2ExecuteResponse, error) {
	start := time.Now()

	// Get current state (must exist after Agent 1)
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Update meta counts
	state.Current.Meta.ProductCount = len(state.Current.Data.Products)
	state.Current.Meta.ServiceCount = len(state.Current.Data.Services)

	// Check if we have data
	if state.Current.Meta.ProductCount == 0 && state.Current.Meta.ServiceCount == 0 {
		// No data, return empty
		return &Agent2ExecuteResponse{
			Template:  nil,
			LatencyMs: int(time.Since(start).Milliseconds()),
		}, nil
	}

	// Build user message with meta info
	userPrompt := prompts.BuildAgent2ToolPrompt(state.Current.Meta, req.LayoutHint)

	messages := []domain.LLMMessage{
		{Role: "user", Content: userPrompt},
	}

	// Get render tool definitions (filter only render_* tools)
	toolDefs := uc.getAgent2Tools()

	// Call LLM with tools
	llmStart := time.Now()
	llmResp, err := uc.llm.ChatWithTools(ctx, prompts.Agent2ToolSystemPrompt, messages, toolDefs)
	llmDuration := time.Since(llmStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("llm call: %w", err)
	}

	response := &Agent2ExecuteResponse{
		Usage:      llmResp.Usage,
		LatencyMs:  int(time.Since(start).Milliseconds()),
		LLMCallMs:  llmDuration,
		PromptSent: userPrompt,
		MetaCount:  state.Current.Meta.Count,
		MetaFields: state.Current.Meta.Fields,
	}

	// Execute tool calls
	for _, toolCall := range llmResp.ToolCalls {
		response.ToolCalled = true
		response.ToolName = toolCall.Name

		result, err := uc.toolRegistry.Execute(ctx, req.SessionID, toolCall)
		if err != nil {
			return nil, fmt.Errorf("execute tool %s: %w", toolCall.Name, err)
		}

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

// getAgent2Tools returns only render_* tools for Agent 2
func (uc *Agent2ExecuteUseCase) getAgent2Tools() []domain.ToolDefinition {
	allTools := uc.toolRegistry.GetDefinitions()
	var agent2Tools []domain.ToolDefinition
	for _, t := range allTools {
		if strings.HasPrefix(t.Name, "render_") {
			agent2Tools = append(agent2Tools, t)
		}
	}
	return agent2Tools
}

// ExecuteLegacy runs Agent 2 using the legacy JSON generation approach
// Kept for backward compatibility and testing
func (uc *Agent2ExecuteUseCase) ExecuteLegacy(ctx context.Context, req Agent2ExecuteRequest) (*Agent2ExecuteResponse, error) {
	start := time.Now()

	// Get current state (must exist after Agent 1)
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Check if we have data
	if state.Current.Meta.Count == 0 {
		// No data, return empty template
		return &Agent2ExecuteResponse{
			Template:  nil,
			LatencyMs: int(time.Since(start).Milliseconds()),
		}, nil
	}

	// Build prompt with meta only (NOT raw data)
	userPrompt := prompts.BuildAgent2Prompt(state.Current.Meta, req.LayoutHint)

	// Call LLM with usage tracking
	llmStart := time.Now()
	llmResp, err := uc.llm.ChatWithUsage(ctx, prompts.Agent2SystemPrompt, userPrompt)
	llmDuration := time.Since(llmStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("llm call: %w", err)
	}

	// Parse template from response (handle markdown code blocks)
	jsonStr := extractJSON(llmResp.Text)
	var template domain.FormationTemplate
	if err := json.Unmarshal([]byte(jsonStr), &template); err != nil {
		return nil, fmt.Errorf("parse template: %w (response: %s)", err, llmResp.Text)
	}

	// Save template to state
	state.Current.Template = map[string]interface{}{
		"mode":           template.Mode,
		"grid":           template.Grid,
		"widgetTemplate": template.WidgetTemplate,
	}
	if err := uc.statePort.UpdateState(ctx, state); err != nil {
		return nil, fmt.Errorf("update state: %w", err)
	}

	return &Agent2ExecuteResponse{
		Template:     &template,
		Usage:        llmResp.Usage,
		LatencyMs:    int(time.Since(start).Milliseconds()),
		LLMCallMs:    llmDuration,
		PromptSent:   userPrompt,
		RawResponse:  llmResp.Text,
		TemplateJSON: jsonStr,
		MetaCount:    state.Current.Meta.Count,
		MetaFields:   state.Current.Meta.Fields,
	}, nil
}

// Note: convertToFormation is defined in pipeline_execute.go and reused here
