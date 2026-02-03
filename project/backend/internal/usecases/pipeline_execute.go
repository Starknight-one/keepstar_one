package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/ports"
	"keepstar/internal/tools"
)

// PipelineExecuteRequest is the input for the full pipeline
type PipelineExecuteRequest struct {
	SessionID string
	Query     string
}

// PipelineExecuteResponse is the output from the full pipeline
type PipelineExecuteResponse struct {
	Formation   *domain.FormationWithData
	Delta       *domain.Delta
	Agent1Ms    int
	Agent2Ms    int
	TotalMs     int
	Agent1Usage domain.LLMUsage
	Agent2Usage domain.LLMUsage
	// Detailed breakdown from Agent 1
	Agent1LLMMs      int64
	Agent1ToolMs     int64
	ToolCalled       string
	ToolInput        string
	ToolResult       string
	ProductsFound    int
	// Detailed breakdown from Agent 2
	Agent2LLMMs      int64
	Agent2Prompt     string
	Agent2RawResp    string
	TemplateJSON     string
	MetaCount        int
	MetaFields       []string
}

// PipelineExecuteUseCase orchestrates Agent 1 → Agent 2 → Formation
type PipelineExecuteUseCase struct {
	agent1UC  *Agent1ExecuteUseCase
	agent2UC  *Agent2ExecuteUseCase
	statePort ports.StatePort
	cachePort ports.CachePort
	log       *logger.Logger
}

// NewPipelineExecuteUseCase creates the pipeline orchestrator
// NOTE: logger is required because Agent1ExecuteUseCase needs it
func NewPipelineExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
	cachePort ports.CachePort,
	toolRegistry *tools.Registry,
	log *logger.Logger,
) *PipelineExecuteUseCase {
	return &PipelineExecuteUseCase{
		agent1UC:  NewAgent1ExecuteUseCase(llm, statePort, toolRegistry, log),
		agent2UC:  NewAgent2ExecuteUseCase(llm, statePort, toolRegistry),
		statePort: statePort,
		cachePort: cachePort,
		log:       log,
	}
}

// Execute runs the full pipeline: query → Agent 1 → Agent 2 → Formation
func (uc *PipelineExecuteUseCase) Execute(ctx context.Context, req PipelineExecuteRequest) (*PipelineExecuteResponse, error) {
	start := time.Now()

	// Ensure session exists (required for FK constraint on state table)
	if uc.cachePort != nil {
		_, err := uc.cachePort.GetSession(ctx, req.SessionID)
		if err == domain.ErrSessionNotFound {
			// Create new session
			session := &domain.Session{
				ID:             req.SessionID,
				Status:         domain.SessionStatusActive,
				Messages:       []domain.Message{},
				StartedAt:      time.Now(),
				LastActivityAt: time.Now(),
			}
			if err := uc.cachePort.SaveSession(ctx, session); err != nil {
				return nil, fmt.Errorf("create session: %w", err)
			}
		}
	}

	// Step 1: Agent 1 (Tool Caller)
	agent1Resp, err := uc.agent1UC.Execute(ctx, Agent1ExecuteRequest{
		SessionID: req.SessionID,
		Query:     req.Query,
	})
	if err != nil {
		return nil, fmt.Errorf("agent 1: %w", err)
	}

	// Step 2: Agent 2 (Template Builder) - triggered after Agent 1
	agent2Resp, err := uc.agent2UC.Execute(ctx, Agent2ExecuteRequest{
		SessionID: req.SessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("agent 2: %w", err)
	}

	// Step 3: Get formation from state (built by Agent 2 tool call)
	// Formation is now built by render_*_preset tool, not ApplyTemplate
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Get formation from Agent 2 response (built by tool) or state
	var formation *domain.FormationWithData
	if agent2Resp.Formation != nil {
		formation = agent2Resp.Formation
	} else if formationData, ok := state.Current.Template["formation"]; ok {
		// Try direct type assertion first (in-memory)
		if f, ok := formationData.(*domain.FormationWithData); ok {
			formation = f
		} else {
			// After DB read, it's map[string]interface{} - convert via JSON
			formation = convertToFormation(formationData)
		}
	}

	// Fallback to legacy ApplyTemplate if tool didn't run (backward compatibility)
	if formation == nil && agent2Resp.Template != nil && len(state.Current.Data.Products) > 0 {
		formation, err = ApplyTemplate(agent2Resp.Template, state.Current.Data.Products)
		if err != nil {
			return nil, fmt.Errorf("apply template: %w", err)
		}
	}

	// Update delta with template (if Agent 2 produced one)
	if agent1Resp.Delta != nil && (agent2Resp.Template != nil || agent2Resp.Formation != nil) {
		agent1Resp.Delta.Template = state.Current.Template
		// Note: delta already saved in Agent 1, template added to state in Agent 2
	}

	return &PipelineExecuteResponse{
		Formation:     formation,
		Delta:         agent1Resp.Delta,
		Agent1Ms:      agent1Resp.LatencyMs,
		Agent2Ms:      agent2Resp.LatencyMs,
		TotalMs:       int(time.Since(start).Milliseconds()),
		Agent1Usage:   agent1Resp.Usage,
		Agent2Usage:   agent2Resp.Usage,
		// Agent 1 details
		Agent1LLMMs:   agent1Resp.LLMCallMs,
		Agent1ToolMs:  agent1Resp.ToolExecuteMs,
		ToolCalled:    agent1Resp.ToolName,
		ToolInput:     agent1Resp.ToolInput,
		ToolResult:    agent1Resp.ToolResult,
		ProductsFound: agent1Resp.ProductsFound,
		// Agent 2 details
		Agent2LLMMs:   agent2Resp.LLMCallMs,
		Agent2Prompt:  agent2Resp.PromptSent,
		Agent2RawResp: agent2Resp.RawResponse,
		TemplateJSON:  agent2Resp.TemplateJSON,
		MetaCount:     agent2Resp.MetaCount,
		MetaFields:    agent2Resp.MetaFields,
	}, nil
}

// convertToFormation converts map[string]interface{} to FormationWithData
// This is needed because after JSON serialization/deserialization from DB,
// the formation becomes a map instead of a typed struct
func convertToFormation(data interface{}) *domain.FormationWithData {
	if data == nil {
		return nil
	}

	// Re-serialize to JSON and back to struct
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	var formation domain.FormationWithData
	if err := json.Unmarshal(jsonBytes, &formation); err != nil {
		return nil
	}

	return &formation
}
