package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	Usage     domain.LLMUsage
	LatencyMs int
	// Detailed timing breakdown
	LLMCallMs      int64  `json:"llmCallMs"`
	ToolExecuteMs  int64  `json:"toolExecuteMs"`
	ToolName       string `json:"toolName"`
	ToolInput      string `json:"toolInput"`
	ToolResult     string                 `json:"toolResult"`
	ToolMetadata   map[string]interface{} `json:"toolMetadata,omitempty"` // Internal breakdown from tool
	ProductsFound  int                    `json:"productsFound"`
	StopReason     string                 `json:"stopReason"`
	// Prompt breakdown for trace
	SystemPrompt      string `json:"systemPrompt"`
	SystemPromptChars int    `json:"systemPromptChars"`
	EnrichedQuery     string `json:"enrichedQuery,omitempty"` // User query with <state> context
	MessageCount      int    `json:"messageCount"`
	ToolDefCount      int    `json:"toolDefCount"`
}

// Agent1ExecuteUseCase executes Agent 1 (Tool Caller)
type Agent1ExecuteUseCase struct {
	llm          ports.LLMPort
	statePort    ports.StatePort
	catalogPort  ports.CatalogPort
	toolRegistry *tools.Registry
	log          *logger.Logger
}

// NewAgent1ExecuteUseCase creates Agent 1 use case
func NewAgent1ExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
	catalogPort ports.CatalogPort,
	toolRegistry *tools.Registry,
	log *logger.Logger,
) *Agent1ExecuteUseCase {
	return &Agent1ExecuteUseCase{
		llm:          llm,
		statePort:    statePort,
		catalogPort:  catalogPort,
		toolRegistry: toolRegistry,
		log:          log,
	}
}

// Execute runs Agent 1: query → tool call → state update → delta
func (uc *Agent1ExecuteUseCase) Execute(ctx context.Context, req Agent1ExecuteRequest) (*Agent1ExecuteResponse, error) {
	start := time.Now()

	// Span instrumentation
	sc := domain.SpanFromContext(ctx)
	if sc != nil {
		endAgent := sc.Start("agent1")
		defer endAgent()
	}
	ctx = domain.WithStage(ctx, "agent1")

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

	// Update meta counts from actual data (pattern from agent2_execute.go)
	state.Current.Meta.ProductCount = len(state.Current.Data.Products)
	state.Current.Meta.ServiceCount = len(state.Current.Data.Services)

	// Extract current RenderConfig from formation (what is on screen now)
	var currentConfig *domain.RenderConfig
	if state.Current.Template != nil {
		if formationData, ok := state.Current.Template["formation"]; ok {
			if f, ok := formationData.(*domain.FormationWithData); ok && f != nil && f.Config != nil {
				currentConfig = f.Config
			}
		}
	}

	// Load pre-computed catalog digest for tenant context
	var digest *domain.CatalogDigest
	if uc.catalogPort != nil && req.TenantSlug != "" {
		tenant, tenantErr := uc.catalogPort.GetTenantBySlug(ctx, req.TenantSlug)
		if tenantErr == nil && tenant != nil {
			digest, _ = uc.catalogPort.GetCatalogDigest(ctx, tenant.ID)
			// Error is not critical — Agent1 works without digest
		}
	}

	// Build enriched query with state context for LLM (ephemeral, not saved to history)
	enrichedQuery := prompts.BuildAgent1ContextPrompt(state.Current.Meta, currentConfig, req.Query, digest)

	// Build messages with conversation history
	messages := state.ConversationHistory
	messages = append(messages, domain.LLMMessage{
		Role:    "user",
		Content: enrichedQuery,
	})

	// Get data-only tool definitions (Agent1 = data layer, no render tools)
	toolDefs := uc.getAgent1Tools()

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
	var toolName string
	var toolInput string
	var toolResult string
	var toolMetadata map[string]interface{}
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

		var endToolSpan func(...string)
		if sc != nil {
			endToolSpan = sc.Start("agent1.tool")
		}
		toolStart := time.Now()
		result, err := uc.toolRegistry.Execute(ctx, tools.ToolContext{
			SessionID: req.SessionID,
			TurnID:    req.TurnID,
			ActorID:   "agent1",
		}, toolCall)
		toolDuration = time.Since(toolStart).Milliseconds()
		if endToolSpan != nil {
			endToolSpan(toolCall.Name)
		}
		toolResult = result.Content
		toolMetadata = result.Metadata

		if err != nil {
			uc.log.Error("tool_execution_failed", "error", err, "tool", toolCall.Name)
			return nil, fmt.Errorf("tool execute: %w", err)
		}

		uc.log.ToolExecuted(toolCall.Name, req.SessionID, result.Content, toolDuration)

		// Get updated state after tool zone-write
		state, _ = uc.statePort.GetState(ctx, req.SessionID)
		productsFound = state.Current.Meta.Count
	} else {
		uc.log.Warn("no_tool_call",
			"session_id", req.SessionID,
			"stop_reason", llmResp.StopReason,
			"text", llmResp.Text,
		)
	}

	// State update span
	var endState func(...string)
	if sc != nil {
		endState = sc.Start("agent1.state")
	}
	// Update conversation history via AppendConversation (zone-write, no blob UpdateState)
	// Full sequence: user → assistant:tool_use → user:tool_result (required by Anthropic API)
	newHistory := append(state.ConversationHistory,
		domain.LLMMessage{Role: "user", Content: req.Query},
	)
	if len(llmResp.ToolCalls) > 0 {
		newHistory = append(newHistory,
			domain.LLMMessage{Role: "assistant", ToolCalls: llmResp.ToolCalls},
			domain.LLMMessage{
				Role: "user",
				ToolResult: &domain.ToolResult{
					ToolUseID: llmResp.ToolCalls[0].ID,
					Content:   toolResult,
				},
			},
		)
	}
	if err := uc.statePort.AppendConversation(ctx, req.SessionID, newHistory); err != nil {
		uc.log.Error("append_conversation_failed", "error", err, "session_id", req.SessionID)
	}
	if endState != nil {
		endState()
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
		Usage:             llmResp.Usage,
		LatencyMs:         int(totalDuration),
		LLMCallMs:         llmDuration,
		ToolExecuteMs:     toolDuration,
		ToolName:          toolName,
		ToolInput:         toolInput,
		ToolResult:        toolResult,
		ToolMetadata:      toolMetadata,
		ProductsFound:     productsFound,
		StopReason:        llmResp.StopReason,
		SystemPrompt:      prompts.Agent1SystemPrompt,
		SystemPromptChars: len(prompts.Agent1SystemPrompt),
		EnrichedQuery:     enrichedQuery,
		MessageCount:      len(messages),
		ToolDefCount:      len(toolDefs),
	}, nil
}

// getAgent1Tools returns data tools only for Agent 1 (catalog_*)
func (uc *Agent1ExecuteUseCase) getAgent1Tools() []domain.ToolDefinition {
	allTools := uc.toolRegistry.GetDefinitions()
	var agent1Tools []domain.ToolDefinition
	for _, t := range allTools {
		if strings.HasPrefix(t.Name, "catalog_") || strings.HasPrefix(t.Name, "_internal_") {
			agent1Tools = append(agent1Tools, t)
		}
	}
	return agent1Tools
}

// GetToolDefs returns the filtered tool definitions for Agent1 (exported for testing)
func (uc *Agent1ExecuteUseCase) GetToolDefs() []domain.ToolDefinition {
	return uc.getAgent1Tools()
}
