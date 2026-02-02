package usecases

import (
	"context"
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
	Formation *domain.FormationWithData
	Delta     *domain.Delta
	Agent1Ms  int
	Agent2Ms  int
	TotalMs   int
}

// PipelineExecuteUseCase orchestrates Agent 1 → Agent 2 → Formation
type PipelineExecuteUseCase struct {
	agent1UC  *Agent1ExecuteUseCase
	agent2UC  *Agent2ExecuteUseCase
	statePort ports.StatePort
	log       *logger.Logger
}

// NewPipelineExecuteUseCase creates the pipeline orchestrator
// NOTE: logger is required because Agent1ExecuteUseCase needs it
func NewPipelineExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
	toolRegistry *tools.Registry,
	log *logger.Logger,
) *PipelineExecuteUseCase {
	return &PipelineExecuteUseCase{
		agent1UC:  NewAgent1ExecuteUseCase(llm, statePort, toolRegistry, log),
		agent2UC:  NewAgent2ExecuteUseCase(llm, statePort),
		statePort: statePort,
		log:       log,
	}
}

// Execute runs the full pipeline: query → Agent 1 → Agent 2 → Formation
func (uc *PipelineExecuteUseCase) Execute(ctx context.Context, req PipelineExecuteRequest) (*PipelineExecuteResponse, error) {
	start := time.Now()

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

	// Step 3: Apply template to data
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	var formation *domain.FormationWithData
	if agent2Resp.Template != nil && len(state.Current.Data.Products) > 0 {
		formation, err = ApplyTemplate(agent2Resp.Template, state.Current.Data.Products)
		if err != nil {
			return nil, fmt.Errorf("apply template: %w", err)
		}
	}

	// Update delta with template (if Agent 2 produced one)
	if agent1Resp.Delta != nil && agent2Resp.Template != nil {
		agent1Resp.Delta.Template = state.Current.Template
		// Note: delta already saved in Agent 1, template added to state in Agent 2
	}

	return &PipelineExecuteResponse{
		Formation: formation,
		Delta:     agent1Resp.Delta,
		Agent1Ms:  agent1Resp.LatencyMs,
		Agent2Ms:  agent2Resp.LatencyMs,
		TotalMs:   int(time.Since(start).Milliseconds()),
	}, nil
}
