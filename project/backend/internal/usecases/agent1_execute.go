package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/ports"
	"keepstar/internal/prompts"
	"keepstar/internal/tools"
)

// Agent1ExecuteRequest is the input for Agent 1
type Agent1ExecuteRequest struct {
	SessionID  string
	Query      string
	TenantSlug string // Tenant context for search
	TurnID     string // Turn ID for delta grouping
}

// Agent1ExecuteResponse is the output from Agent 1
type Agent1ExecuteResponse struct {
	Delta     *domain.Delta
	Usage     domain.LLMUsage
	LatencyMs int
	// Detailed timing breakdown
	LLMCallMs      int64  `json:"llmCallMs"`
	ToolExecuteMs  int64  `json:"toolExecuteMs"`
	ToolName       string `json:"toolName"`
	ToolInput      string `json:"toolInput"`
	ToolResult     string `json:"toolResult"`
	ProductsFound  int    `json:"productsFound"`
	StopReason     string `json:"stopReason"`
}

// Agent1ExecuteUseCase executes Agent 1 (Tool Caller)
type Agent1ExecuteUseCase struct {
	llm          ports.LLMPort
	statePort    ports.StatePort
	toolRegistry *tools.Registry
	log          *logger.Logger
}

// NewAgent1ExecuteUseCase creates Agent 1 use case
func NewAgent1ExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
	toolRegistry *tools.Registry,
	log *logger.Logger,
) *Agent1ExecuteUseCase {
	return &Agent1ExecuteUseCase{
		llm:          llm,
		statePort:    statePort,
		toolRegistry: toolRegistry,
		log:          log,
	}
}

// Execute runs Agent 1: query → tool call → state update → delta
func (uc *Agent1ExecuteUseCase) Execute(ctx context.Context, req Agent1ExecuteRequest) (*Agent1ExecuteResponse, error) {
	start := time.Now()

	uc.log.Info("agent1_started",
		"session_id", req.SessionID,
		"query", req.Query,
	)

	// Get or create state
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err == domain.ErrSessionNotFound {
		state, err = uc.statePort.CreateState(ctx, req.SessionID)
		uc.log.Debug("state_created", "session_id", req.SessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Set tenant context in state
	if req.TenantSlug != "" {
		if state.Current.Meta.Aliases == nil {
			state.Current.Meta.Aliases = make(map[string]string)
		}
		state.Current.Meta.Aliases["tenant_slug"] = req.TenantSlug
	}

	// Build messages with conversation history
	messages := state.ConversationHistory
	messages = append(messages, domain.LLMMessage{
		Role:    "user",
		Content: req.Query,
	})

	// Get tool definitions
	toolDefs := uc.toolRegistry.GetDefinitions()

	// Call LLM with caching
	llmStart := time.Now()
	llmResp, err := uc.llm.ChatWithToolsCached(
		ctx,
		prompts.Agent1SystemPrompt,
		messages,
		toolDefs,
		&ports.CacheConfig{
			CacheTools:        true,
			CacheSystem:       true,
			CacheConversation: len(messages) > 1, // cache if history exists
		},
	)
	llmDuration := time.Since(llmStart).Milliseconds()

	if err != nil {
		uc.log.Error("llm_call_failed", "error", err, "session_id", req.SessionID)
		return nil, fmt.Errorf("llm call: %w", err)
	}

	// Log LLM usage with cache metrics
	uc.log.LLMUsageWithCache(
		"agent1",
		llmResp.Usage.Model,
		llmResp.Usage.InputTokens,
		llmResp.Usage.OutputTokens,
		llmResp.Usage.CacheCreationInputTokens,
		llmResp.Usage.CacheReadInputTokens,
		llmResp.Usage.CostUSD,
		llmDuration,
	)

	// Process tool calls
	var delta *domain.Delta
	var toolName string
	var toolInput string
	var toolResult string
	var toolDuration int64
	var productsFound int

	if len(llmResp.ToolCalls) > 0 {
		// Execute first tool call (Agent 1 should only call one)
		toolCall := llmResp.ToolCalls[0]
		toolName = toolCall.Name
		// Serialize tool input to JSON string for debug
		if inputBytes, err := json.Marshal(toolCall.Input); err == nil {
			toolInput = string(inputBytes)
		}

		uc.log.Debug("tool_call_received",
			"tool", toolCall.Name,
			"input", toolCall.Input,
			"session_id", req.SessionID,
		)

		toolStart := time.Now()
		result, err := uc.toolRegistry.Execute(ctx, req.SessionID, toolCall)
		toolDuration = time.Since(toolStart).Milliseconds()
		toolResult = result.Content

		if err != nil {
			uc.log.Error("tool_execution_failed", "error", err, "tool", toolCall.Name)
			return nil, fmt.Errorf("tool execute: %w", err)
		}

		uc.log.ToolExecuted(toolCall.Name, req.SessionID, result.Content, toolDuration)

		// Get updated state for delta
		state, _ = uc.statePort.GetState(ctx, req.SessionID)
		productsFound = state.Current.Meta.Count

		// Create delta via DeltaInfo with TurnID (step auto-assigned by AddDelta)
		info := domain.DeltaInfo{
			TurnID:    req.TurnID,
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   "agent1",
			DeltaType: domain.DeltaTypeAdd,
			Path:      "data.products",
			Action: domain.Action{
				Type:   domain.ActionSearch,
				Tool:   toolCall.Name,
				Params: toolCall.Input,
			},
			Result: domain.ResultMeta{
				Count:  state.Current.Meta.Count,
				Fields: state.Current.Meta.Fields,
			},
		}
		delta = info.ToDelta()

		// Save delta (step auto-assigned)
		if _, err := uc.statePort.AddDelta(ctx, req.SessionID, delta); err != nil {
			return nil, fmt.Errorf("add delta: %w", err)
		}
	} else {
		uc.log.Warn("no_tool_call",
			"session_id", req.SessionID,
			"stop_reason", llmResp.StopReason,
			"text", llmResp.Text,
		)
	}

	// Update conversation history via AppendConversation (zone-write, no blob UpdateState)
	newHistory := append(state.ConversationHistory,
		domain.LLMMessage{Role: "user", Content: req.Query},
	)
	if len(llmResp.ToolCalls) > 0 {
		newHistory = append(newHistory,
			domain.LLMMessage{Role: "assistant", ToolCalls: llmResp.ToolCalls},
		)
	}
	if err := uc.statePort.AppendConversation(ctx, req.SessionID, newHistory); err != nil {
		uc.log.Error("append_conversation_failed", "error", err, "session_id", req.SessionID)
	}

	totalDuration := time.Since(start).Milliseconds()

	// Log completion summary
	uc.log.Agent1Completed(
		req.SessionID,
		toolName,
		productsFound,
		llmResp.Usage.TotalTokens,
		llmResp.Usage.CostUSD,
		totalDuration,
	)

	return &Agent1ExecuteResponse{
		Delta:         delta,
		Usage:         llmResp.Usage,
		LatencyMs:     int(totalDuration),
		LLMCallMs:     llmDuration,
		ToolExecuteMs: toolDuration,
		ToolName:      toolName,
		ToolInput:     toolInput,
		ToolResult:    toolResult,
		ProductsFound: productsFound,
		StopReason:    llmResp.StopReason,
	}, nil
}
