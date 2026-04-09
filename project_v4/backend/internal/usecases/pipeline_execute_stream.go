package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"keepstar_v4/internal/adapters/anthropic"
	"keepstar_v4/internal/domain"
	engine_v4 "keepstar_v4/internal/engine_v4"
	"keepstar_v4/internal/logger"
	"keepstar_v4/internal/ports"
	"keepstar_v4/internal/tools"
)

// PipelineStreamUseCase orchestrates Agent1 → streaming Agent2 → SSE events.
type PipelineStreamUseCase struct {
	agent1UC    *Agent1ExecuteUseCase
	agent2Stream *Agent2StreamUseCase
	statePort   ports.StatePort
	cachePort   ports.CachePort
	tracePort   ports.TracePort
	engine      *engine_v4.Engine
	log         *logger.Logger
}

// NewPipelineStreamUseCase wires the streaming pipeline orchestrator.
func NewPipelineStreamUseCase(
	client *anthropic.Client,
	statePort ports.StatePort,
	cachePort ports.CachePort,
	tracePort ports.TracePort,
	catalogPort ports.CatalogPort,
	toolRegistry *tools.Registry,
	eng *engine_v4.Engine,
	agent2Stream *Agent2StreamUseCase,
	log *logger.Logger,
) *PipelineStreamUseCase {
	return &PipelineStreamUseCase{
		agent1UC:     NewAgent1ExecuteUseCase(client, statePort, catalogPort, toolRegistry, log),
		agent2Stream: agent2Stream,
		statePort:    statePort,
		cachePort:    cachePort,
		tracePort:    tracePort,
		engine:       eng,
		log:          log,
	}
}

// Execute runs Agent1 synchronously then starts streaming Agent2.
// Events are emitted on `events` in order: session_init → (widget_provisional)* →
// formation_complete. The caller owns the channel and must close it after
// Execute returns.
func (uc *PipelineStreamUseCase) Execute(
	ctx context.Context,
	req PipelineExecuteRequest,
	events chan<- StreamEvent,
) (*PipelineExecuteResponse, error) {
	start := time.Now()

	sc := domain.NewSpanCollector()
	ctx = domain.WithSpanCollector(ctx, sc)
	endPipeline := sc.Start("pipeline.stream")

	// Ensure session exists
	if uc.cachePort != nil {
		if _, err := uc.cachePort.GetSession(ctx, req.SessionID); err == domain.ErrSessionNotFound {
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

	turnID := req.TurnID
	if turnID == "" {
		turnID = uuid.New().String()
	}

	// Emit session_init immediately so frontend can show skeletons
	events <- StreamEvent{
		Kind:      StreamEventSessionInit,
		SessionID: req.SessionID,
		TurnID:    turnID,
	}

	// Agent1 (blocking — tool call must complete before Agent2 starts)
	agent1Resp, err := uc.agent1UC.Execute(ctx, Agent1ExecuteRequest{
		SessionID:  req.SessionID,
		Query:      req.Query,
		TenantSlug: req.TenantSlug,
		TurnID:     turnID,
	})
	if err != nil {
		events <- StreamEvent{Kind: StreamEventError, Error: fmt.Sprintf("agent1: %v", err)}
		return nil, err
	}

	microcontext := buildMicrocontext(agent1Resp)

	// Agent2 streaming
	agent2Resp, err := uc.agent2Stream.Execute(ctx, Agent2ExecuteRequest{
		SessionID:     req.SessionID,
		TurnID:        turnID,
		TenantSlug:    req.TenantSlug,
		UserQuery:     req.Query,
		Microcontext:  microcontext,
		ScreenContext: req.ScreenContext,
	}, events)
	if err != nil {
		return nil, err
	}

	endPipeline()

	resp := &PipelineExecuteResponse{
		Agent1Ms:      agent1Resp.LatencyMs,
		Agent1Usage:   agent1Resp.Usage,
		Agent1LLMMs:   agent1Resp.LLMCallMs,
		Agent1ToolMs:  agent1Resp.ToolExecuteMs,
		ToolCalled:    agent1Resp.ToolName,
		ToolInput:     agent1Resp.ToolInput,
		ToolResult:    agent1Resp.ToolResult,
		ProductsFound: agent1Resp.ProductsFound,
		TotalMs:       int(time.Since(start).Milliseconds()),
	}
	if agent2Resp != nil {
		resp.Formation = agent2Resp.Formation
		resp.Agent2Ms = agent2Resp.LatencyMs
		resp.Agent2Usage = agent2Resp.Usage
		resp.Agent2LLMMs = agent2Resp.LLMCallMs
		resp.Agent2Prompt = agent2Resp.PromptSent
		resp.Agent2RawResp = agent2Resp.RawResponse
		resp.MetaCount = agent2Resp.MetaCount
		resp.MetaFields = agent2Resp.MetaFields
	}
	return resp, nil
}
