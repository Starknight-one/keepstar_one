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
	Template  *domain.FormationTemplate
	Usage     domain.LLMUsage
	LatencyMs int
	// Detailed timing and data
	LLMCallMs    int64  `json:"llmCallMs"`
	PromptSent   string `json:"promptSent"`
	RawResponse  string `json:"rawResponse"`
	TemplateJSON string `json:"templateJson"`
	MetaCount    int    `json:"metaCount"`
	MetaFields   []string `json:"metaFields"`
}

// Agent2ExecuteUseCase executes Agent 2 (Template Builder)
type Agent2ExecuteUseCase struct {
	llm       ports.LLMPort
	statePort ports.StatePort
}

// NewAgent2ExecuteUseCase creates Agent 2 use case
func NewAgent2ExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
) *Agent2ExecuteUseCase {
	return &Agent2ExecuteUseCase{
		llm:       llm,
		statePort: statePort,
	}
}

// Execute runs Agent 2: meta → LLM → template → save to state
func (uc *Agent2ExecuteUseCase) Execute(ctx context.Context, req Agent2ExecuteRequest) (*Agent2ExecuteResponse, error) {
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
