package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/ports"
	"keepstar/internal/tools"
)

// PipelineExecuteRequest is the input for the full pipeline
type PipelineExecuteRequest struct {
	SessionID  string
	Query      string
	TenantSlug string // Tenant context (default: "nike")
	TurnID     string // Turn ID for delta grouping
}

// PipelineExecuteResponse is the output from the full pipeline
type PipelineExecuteResponse struct {
	Formation   *domain.FormationWithData
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
	tracePort ports.TracePort
	log       *logger.Logger
}

// NewPipelineExecuteUseCase creates the pipeline orchestrator
// NOTE: logger is required because Agent1ExecuteUseCase needs it
func NewPipelineExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
	cachePort ports.CachePort,
	tracePort ports.TracePort,
	toolRegistry *tools.Registry,
	log *logger.Logger,
) *PipelineExecuteUseCase {
	return &PipelineExecuteUseCase{
		agent1UC:  NewAgent1ExecuteUseCase(llm, statePort, toolRegistry, log),
		agent2UC:  NewAgent2ExecuteUseCase(llm, statePort, toolRegistry, log),
		statePort: statePort,
		cachePort: cachePort,
		tracePort: tracePort,
		log:       log,
	}
}

// Execute runs the full pipeline: query → Agent 1 → Agent 2 → Formation
func (uc *PipelineExecuteUseCase) Execute(ctx context.Context, req PipelineExecuteRequest) (*PipelineExecuteResponse, error) {
	start := time.Now()

	// Create SpanCollector for waterfall timeline
	sc := domain.NewSpanCollector()
	ctx = domain.WithSpanCollector(ctx, sc)
	endPipeline := sc.Start("pipeline")

	// Prepare trace
	trace := &domain.PipelineTrace{
		ID:        uuid.New().String(),
		SessionID: req.SessionID,
		Query:     req.Query,
		Timestamp: time.Now(),
	}

	// Ensure session exists (required for FK constraint on state table)
	if uc.cachePort != nil {
		_, err := uc.cachePort.GetSession(ctx, req.SessionID)
		if err == domain.ErrSessionNotFound {
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

	// Generate TurnID for delta grouping
	turnID := req.TurnID
	if turnID == "" {
		turnID = uuid.New().String()
	}
	trace.TurnID = turnID

	// Step 1: Agent 1 (Tool Caller)
	agent1Resp, err := uc.agent1UC.Execute(ctx, Agent1ExecuteRequest{
		SessionID:  req.SessionID,
		Query:      req.Query,
		TenantSlug: req.TenantSlug,
		TurnID:     turnID,
	})
	if err != nil {
		trace.Error = fmt.Sprintf("agent1: %v", err)
		endPipeline()
		trace.Spans = sc.Spans()
		trace.TotalMs = int(time.Since(start).Milliseconds())
		uc.recordTrace(ctx, trace)
		return nil, fmt.Errorf("agent 1: %w", err)
	}

	// Fill Agent1 trace
	trace.Agent1 = &domain.AgentTrace{
		Name:              "agent1",
		LLMMs:             agent1Resp.LLMCallMs,
		ToolMs:            agent1Resp.ToolExecuteMs,
		TotalMs:           agent1Resp.LatencyMs,
		StopReason:        agent1Resp.StopReason,
		Model:             agent1Resp.Usage.Model,
		InputTokens:       agent1Resp.Usage.InputTokens,
		OutputTokens:      agent1Resp.Usage.OutputTokens,
		CacheRead:         agent1Resp.Usage.CacheReadInputTokens,
		CacheWrite:        agent1Resp.Usage.CacheCreationInputTokens,
		CostUSD:           agent1Resp.Usage.CostUSD,
		SystemPrompt:      agent1Resp.SystemPrompt,
		SystemPromptChars: agent1Resp.SystemPromptChars,
		MessageCount:      agent1Resp.MessageCount,
		ToolDefCount:      agent1Resp.ToolDefCount,
		ToolName:          agent1Resp.ToolName,
		ToolInput:         agent1Resp.ToolInput,
		ToolResult:        agent1Resp.ToolResult,
		ToolBreakdown:     agent1Resp.ToolMetadata,
	}

	// Snapshot state after Agent1
	if midState, err := uc.statePort.GetState(ctx, req.SessionID); err == nil {
		deltas, _ := uc.statePort.GetDeltas(ctx, req.SessionID)
		snapshot := &domain.StateSnapshot{
			ProductCount: len(midState.Current.Data.Products),
			ServiceCount: len(midState.Current.Data.Services),
			Fields:       midState.Current.Meta.Fields,
			Aliases:      midState.Current.Meta.Aliases,
			HasTemplate:  midState.Current.Template != nil,
			DeltaCount:   len(deltas),
		}
		// Include detailed delta traces for this turn
		for _, d := range deltas {
			if d.TurnID != "" && d.TurnID != turnID {
				continue // only deltas from current turn
			}
			dt := domain.DeltaTrace{
				Step:      d.Step,
				ActorID:   d.ActorID,
				DeltaType: string(d.DeltaType),
				Path:      d.Path,
				Tool:      d.Action.Tool,
				Count:     d.Result.Count,
				Fields:    d.Result.Fields,
			}
			snapshot.Deltas = append(snapshot.Deltas, dt)
		}
		trace.StateAfterAgent1 = snapshot
	}

	// Step 2: Agent 2 (Template Builder) - triggered after Agent 1
	agent2Resp, err := uc.agent2UC.Execute(ctx, Agent2ExecuteRequest{
		SessionID: req.SessionID,
		TurnID:    turnID,
		UserQuery: req.Query,
	})
	if err != nil {
		trace.Error = fmt.Sprintf("agent2: %v", err)
		endPipeline()
		trace.Spans = sc.Spans()
		trace.TotalMs = int(time.Since(start).Milliseconds())
		trace.CostUSD = agent1Resp.Usage.CostUSD
		uc.recordTrace(ctx, trace)
		return nil, fmt.Errorf("agent 2: %w", err)
	}

	// Fill Agent2 trace
	trace.Agent2 = &domain.AgentTrace{
		Name:         "agent2",
		LLMMs:        agent2Resp.LLMCallMs,
		TotalMs:      agent2Resp.LatencyMs,
		Model:        agent2Resp.Usage.Model,
		InputTokens:  agent2Resp.Usage.InputTokens,
		OutputTokens: agent2Resp.Usage.OutputTokens,
		CacheRead:    agent2Resp.Usage.CacheReadInputTokens,
		CacheWrite:   agent2Resp.Usage.CacheCreationInputTokens,
		CostUSD:      agent2Resp.Usage.CostUSD,
		ToolName:     agent2Resp.ToolName,
		ToolResult:   agent2Resp.RawResponse,
		PromptSent:   agent2Resp.PromptSent,
		RawResponse:  agent2Resp.RawResponse,
	}

	// Step 3: Get formation from state (built by Agent 2 tool call)
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Get formation from Agent 2 response (built by tool) or state
	var formation *domain.FormationWithData
	if agent2Resp.Formation != nil {
		formation = agent2Resp.Formation
	} else if formationData, ok := state.Current.Template["formation"]; ok {
		if f, ok := formationData.(*domain.FormationWithData); ok {
			formation = f
		} else {
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

	// Fill formation trace
	if formation != nil {
		ft := &domain.FormationTrace{
			Mode:        string(formation.Mode),
			WidgetCount: len(formation.Widgets),
		}
		if formation.Grid != nil {
			ft.Cols = formation.Grid.Cols
		}
		if len(formation.Widgets) > 0 {
			for _, atom := range formation.Widgets[0].Atoms {
				if atom.Slot == domain.AtomSlotTitle {
					if s, ok := atom.Value.(string); ok {
						ft.FirstWidget = s
					}
					break
				}
			}
		}
		trace.FormationResult = ft
	}

	// Finalize and record trace
	endPipeline()
	trace.Spans = sc.Spans()
	trace.TotalMs = int(time.Since(start).Milliseconds())
	trace.CostUSD = agent1Resp.Usage.CostUSD + agent2Resp.Usage.CostUSD
	uc.recordTrace(ctx, trace)

	return &PipelineExecuteResponse{
		Formation:     formation,
		Agent1Ms:      agent1Resp.LatencyMs,
		Agent2Ms:      agent2Resp.LatencyMs,
		TotalMs:       int(time.Since(start).Milliseconds()),
		Agent1Usage:   agent1Resp.Usage,
		Agent2Usage:   agent2Resp.Usage,
		Agent1LLMMs:   agent1Resp.LLMCallMs,
		Agent1ToolMs:  agent1Resp.ToolExecuteMs,
		ToolCalled:    agent1Resp.ToolName,
		ToolInput:     agent1Resp.ToolInput,
		ToolResult:    agent1Resp.ToolResult,
		ProductsFound: agent1Resp.ProductsFound,
		Agent2LLMMs:   agent2Resp.LLMCallMs,
		Agent2Prompt:  agent2Resp.PromptSent,
		Agent2RawResp: agent2Resp.RawResponse,
		TemplateJSON:  agent2Resp.TemplateJSON,
		MetaCount:     agent2Resp.MetaCount,
		MetaFields:    agent2Resp.MetaFields,
	}, nil
}

// recordTrace saves trace if tracePort is available
func (uc *PipelineExecuteUseCase) recordTrace(ctx context.Context, trace *domain.PipelineTrace) {
	if uc.tracePort == nil {
		return
	}
	if err := uc.tracePort.Record(ctx, trace); err != nil {
		uc.log.Error("trace_record_failed", "error", err)
	}
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
