package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"keepstar_v4/internal/adapters/anthropic"
	"keepstar_v4/internal/domain"
	engine_v4 "keepstar_v4/internal/engine_v4"
	"keepstar_v4/internal/logger"
	"keepstar_v4/internal/ports"
	"keepstar_v4/internal/prompts"
	"keepstar_v4/internal/tools"
)

// StreamEventKind classifies streaming pipeline events.
type StreamEventKind string

const (
	StreamEventSessionInit      StreamEventKind = "session_init"
	StreamEventWidgetProvisional StreamEventKind = "widget_provisional"
	StreamEventWidgetFinalized   StreamEventKind = "widget_finalized"
	StreamEventFormationComplete StreamEventKind = "formation_complete"
	StreamEventError             StreamEventKind = "error"
)

// StreamEvent is emitted by Agent2StreamUseCase and the streaming pipeline.
type StreamEvent struct {
	Kind      StreamEventKind         `json:"kind"`
	SessionID string                  `json:"sessionId,omitempty"`
	TurnID    string                  `json:"turnId,omitempty"`
	Index     int                     `json:"index,omitempty"`    // widget index (provisional/finalized)
	Widgets   []domain.Widget         `json:"widgets,omitempty"`  // partial or finalized widget batch
	Formation *domain.FormationWithData `json:"formation,omitempty"` // final
	Usage     *domain.LLMUsage        `json:"usage,omitempty"`
	Error     string                  `json:"error,omitempty"`
}

// Agent2StreamUseCase runs Agent2 with SSE streaming from Anthropic.
// Emits WidgetProvisional events as soon as each widget's ops arrive, then a
// FormationComplete event with the finalized formation (cross-widget
// constraints applied, tree IDs stamped, state persisted).
type Agent2StreamUseCase struct {
	client       *anthropic.Client
	statePort    ports.StatePort
	toolRegistry *tools.Registry
	engine       *engine_v4.Engine
	log          *logger.Logger
	fieldDefPort ports.FieldDefinitionPort
}

// NewAgent2StreamUseCase builds the streaming Agent2 orchestrator.
func NewAgent2StreamUseCase(
	client *anthropic.Client,
	statePort ports.StatePort,
	toolRegistry *tools.Registry,
	engine *engine_v4.Engine,
	log *logger.Logger,
	fieldDefPort ports.FieldDefinitionPort,
) *Agent2StreamUseCase {
	return &Agent2StreamUseCase{
		client:       client,
		statePort:    statePort,
		toolRegistry: toolRegistry,
		engine:       engine,
		log:          log,
		fieldDefPort: fieldDefPort,
	}
}

// Execute runs the streaming Agent2 path and emits events on `events`. The
// caller (pipeline or handler) owns the channel and is responsible for closing
// it. This method returns when the LLM stream is fully consumed and the
// final tool execution has persisted state. Errors are reported both via the
// return value and a StreamEventError on the channel.
func (uc *Agent2StreamUseCase) Execute(
	ctx context.Context,
	req Agent2ExecuteRequest,
	events chan<- StreamEvent,
) (*Agent2ExecuteResponse, error) {
	start := time.Now()

	sc := domain.SpanFromContext(ctx)
	if sc != nil {
		endAgent := sc.Start("agent2.stream")
		defer endAgent()
	}
	ctx = domain.WithStage(ctx, "agent2")

	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}
	state.Current.Meta.ProductCount = len(state.Current.Data.Products)
	state.Current.Meta.ServiceCount = len(state.Current.Data.Services)

	// No data → emit empty state message (same as non-streaming), no streaming.
	if state.Current.Meta.ProductCount == 0 && state.Current.Meta.ServiceCount == 0 {
		emptyResp := &Agent2ExecuteResponse{LatencyMs: int(time.Since(start).Milliseconds())}
		return emptyResp, nil
	}

	// Data delta for context
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

	// Current formation tree for Agent2 context
	var currentConfig *domain.RenderConfig
	var formationTree map[string]interface{}
	if state.Current.Template != nil {
		if formationData, ok := state.Current.Template["formation"]; ok {
			var f *domain.FormationWithData
			if typed, ok := formationData.(*domain.FormationWithData); ok {
				f = typed
			} else {
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

	allDeltas, _ := uc.statePort.GetDeltas(ctx, req.SessionID)

	var screenCtx *prompts.ScreenContext
	if req.ScreenContext != nil {
		screenCtx = &prompts.ScreenContext{
			Mode:        req.ScreenContext.Mode,
			WidgetCount: req.ScreenContext.WidgetCount,
			Fields:      req.ScreenContext.Fields,
		}
	}

	systemPrompt := prompts.Agent2ToolSystemPrompt
	fieldLabels := uc.loadFieldLabels(ctx, req.TenantSlug, state)
	userPrompt := prompts.BuildAgent2ToolPrompt(
		state.Current.Meta, state.View, req.UserQuery, dataDelta,
		currentConfig, allDeltas, req.Microcontext, screenCtx, fieldLabels, formationTree,
	)

	var messages []domain.LLMMessage
	if len(state.Agent2History) > 0 {
		historyLimit := 4
		startIdx := len(state.Agent2History) - historyLimit
		if startIdx < 0 {
			startIdx = 0
		}
		messages = append(messages, state.Agent2History[startIdx:]...)
	}
	messages = append(messages, domain.LLMMessage{Role: "user", Content: userPrompt})

	// Tool defs — same filter as non-streaming
	allToolDefs := uc.toolRegistry.GetDefinitions()
	var toolDefs []domain.ToolDefinition
	for _, t := range allToolDefs {
		if strings.HasPrefix(t.Name, "visual_") {
			toolDefs = append(toolDefs, t)
		}
	}

	// Prepare entity data once (for provisional widget rendering)
	var entityData []map[string]interface{}
	entityType := "product"
	if len(state.Current.Data.Services) > 0 && len(state.Current.Data.Products) == 0 {
		entityType = "service"
	}
	for _, p := range state.Current.Data.Products {
		entityData = append(entityData, tools.ProductToMap(p))
	}
	for _, s := range state.Current.Data.Services {
		entityData = append(entityData, tools.ServiceToMap(s))
	}

	// Streaming decoder and widget counter
	decoder := tools.NewStreamingToolDecoder()
	widgetIdx := 0

	emitDecoded := func(decodedEvents []tools.DecodedEvent) {
		for _, ev := range decodedEvents {
			if ev.Kind == tools.DecodeError {
				uc.log.Warn("stream_decode_error", "error", ev.Err)
				continue
			}
			if ev.Kind != tools.WidgetComplete {
				continue
			}
			// Partial engine pass over this widget group.
			partial := uc.engine.ExecuteWidgetPartial(engine_v4.PartialInput{
				Ops:        ev.Ops,
				Data:       entityData,
				EntityType: entityType,
			})
			if len(partial.Widgets) == 0 {
				continue
			}
			select {
			case events <- StreamEvent{
				Kind:      StreamEventWidgetProvisional,
				SessionID: req.SessionID,
				TurnID:    req.TurnID,
				Index:     widgetIdx,
				Widgets:   partial.Widgets,
			}:
			case <-ctx.Done():
				return
			}
			widgetIdx++
		}
	}

	// Streaming callbacks — feed decoder, emit partial render events.
	callbacks := &anthropic.StreamCallbacks{
		OnInputJSONDelta: func(blockIndex int, partialJSON string) error {
			emitDecoded(decoder.Feed(partialJSON))
			return nil
		},
		OnContentBlockStop: func(blockIndex int) error {
			emitDecoded(decoder.Flush())
			return nil
		},
	}

	llmStart := time.Now()
	llmResp, err := uc.client.ChatWithToolsCachedStream(
		ctx,
		systemPrompt,
		messages,
		toolDefs,
		&ports.CacheConfig{CacheTools: true, CacheSystem: true, ToolChoice: "any"},
		callbacks,
	)
	llmDuration := time.Since(llmStart).Milliseconds()
	if err != nil {
		events <- StreamEvent{Kind: StreamEventError, Error: err.Error()}
		return nil, fmt.Errorf("llm stream: %w", err)
	}

	uc.log.LLMUsageWithCache(
		"agent2.stream", llmResp.Usage.Model,
		llmResp.Usage.InputTokens, llmResp.Usage.OutputTokens,
		llmResp.Usage.CacheCreationInputTokens, llmResp.Usage.CacheReadInputTokens,
		llmResp.Usage.CostUSD, llmDuration,
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

	// Now run the real tool execution (writes state, applies cross-widget
	// constraints, stamps IDs, builds tree map). Reuse the same path as
	// non-streaming so there is one source of truth for persisted state.
	for _, toolCall := range llmResp.ToolCalls {
		response.ToolCalled = true
		response.ToolName = toolCall.Name
		if inputJSON, err := json.Marshal(toolCall.Input); err == nil {
			response.ToolInput = string(inputJSON)
		}
		result, err := uc.toolRegistry.Execute(ctx, tools.ToolContext{
			SessionID:  req.SessionID,
			TurnID:     req.TurnID,
			ActorID:    "agent2",
			TenantSlug: req.TenantSlug,
			UserQuery:  req.UserQuery,
		}, toolCall)
		if err != nil {
			events <- StreamEvent{Kind: StreamEventError, Error: err.Error()}
			return nil, fmt.Errorf("tool execute: %w", err)
		}
		response.RawResponse = result.Content
		if result.IsError {
			events <- StreamEvent{Kind: StreamEventError, Error: result.Content}
			return nil, fmt.Errorf("tool error: %s", result.Content)
		}
		agent2Messages := append(state.Agent2History,
			domain.LLMMessage{Role: "assistant", ToolCalls: llmResp.ToolCalls},
			domain.LLMMessage{Role: "user", ToolResult: &domain.ToolResult{
				ToolUseID: toolCall.ID, Content: result.Content,
			}},
		)
		if err := uc.statePort.AppendAgent2History(ctx, req.SessionID, agent2Messages); err != nil {
			uc.log.Error("append_agent2_history_failed", "error", err)
		}
	}

	// Fetch finalized formation from state and emit FormationComplete.
	state2, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state after tool: %w", err)
	}
	if formationData, ok := state2.Current.Template["formation"]; ok {
		if f, ok := formationData.(*domain.FormationWithData); ok {
			response.Formation = f
		} else {
			response.Formation = convertToFormation(formationData)
		}
	}

	events <- StreamEvent{
		Kind:      StreamEventFormationComplete,
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		Formation: response.Formation,
		Usage:     &llmResp.Usage,
	}

	return response, nil
}

// loadFieldLabels mirrors Agent2ExecuteUseCase.loadFieldLabels.
func (uc *Agent2StreamUseCase) loadFieldLabels(ctx context.Context, tenantSlug string, state *domain.SessionState) map[string]string {
	if uc.fieldDefPort == nil || tenantSlug == "" {
		return nil
	}
	labels := make(map[string]string)
	if state.Current.Meta.ProductCount > 0 {
		if defs, err := uc.fieldDefPort.ListFieldDefinitions(ctx, tenantSlug, domain.EntityTypeProduct); err == nil {
			for _, d := range defs {
				if d.Label != "" {
					labels[d.FieldName] = d.Label
				}
			}
		}
	}
	if state.Current.Meta.ServiceCount > 0 {
		if defs, err := uc.fieldDefPort.ListFieldDefinitions(ctx, tenantSlug, domain.EntityTypeService); err == nil {
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
