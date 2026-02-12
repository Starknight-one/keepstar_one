package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

const apiURL = "https://api.anthropic.com/v1/messages"

// Client implements ports.LLMPort for Anthropic API
type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
	log        *slog.Logger
}

// NewClient creates a new Anthropic client
func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		log:        slog.Default(),
	}
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Chat sends a message to Anthropic and returns the response
func (c *Client) Chat(ctx context.Context, message string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("anthropic API key not configured")
	}

	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 1024,
		Messages: []anthropicMessage{
			{Role: "user", Content: message},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if anthropicResp.Error != nil {
		return "", fmt.Errorf("API error: %s", anthropicResp.Error.Message)
	}

	if len(anthropicResp.Content) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return anthropicResp.Content[0].Text, nil
}

// ChatWithUsage sends a message and returns response with usage stats
func (c *Client) ChatWithUsage(ctx context.Context, systemPrompt, userMessage string) (*ports.ChatResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("anthropic API key not configured")
	}

	// Use the tool request format which supports system prompt
	reqBody := anthropicToolRequest{
		Model:     c.model,
		MaxTokens: 1024,
		System:    systemPrompt,
		Messages: []anthropicToolMsg{
			{Role: "user", Content: userMessage},
		},
		// No tools - just text response
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	var anthroResp anthropicToolResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthroResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if anthroResp.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", anthroResp.Error.Message)
	}

	// Extract text from content
	var text string
	for _, block := range anthroResp.Content {
		if block.Type == "text" {
			text = block.Text
			break
		}
	}

	usage := domain.LLMUsage{
		InputTokens:  anthroResp.Usage.InputTokens,
		OutputTokens: anthroResp.Usage.OutputTokens,
		TotalTokens:  anthroResp.Usage.InputTokens + anthroResp.Usage.OutputTokens,
		Model:        c.model,
	}
	usage.CostUSD = usage.CalculateCost()

	return &ports.ChatResponse{
		Text:  text,
		Usage: usage,
	}, nil
}

// Tool calling types

type anthropicToolRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicToolMsg `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicToolMsg struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []contentBlock
}

type contentBlock struct {
	Type      string                 `json:"type"` // "text", "tool_use", "tool_result"
	Text      string                 `json:"text,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   string                 `json:"content,omitempty"`
	IsError   bool                   `json:"is_error,omitempty"`
}

type anthropicToolResponse struct {
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ChatWithTools sends messages with tool definitions, returns potential tool calls
func (c *Client) ChatWithTools(
	ctx context.Context,
	systemPrompt string,
	messages []domain.LLMMessage,
	tools []domain.ToolDefinition,
) (*domain.LLMResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("anthropic API key not configured")
	}

	// Convert domain messages to Anthropic format
	anthroMsgs := make([]anthropicToolMsg, 0, len(messages))
	for _, msg := range messages {
		anthroMsgs = append(anthroMsgs, convertToAnthropicMessage(msg))
	}

	// Convert tools
	anthroTools := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		anthroTools = append(anthroTools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	reqBody := anthropicToolRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  anthroMsgs,
		Tools:     anthroTools,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Span instrumentation via httptrace
	sc := domain.SpanFromContext(ctx)
	stage := domain.StageFromContext(ctx)
	reqSize := len(jsonBody)

	var endLLM func(...string)
	var endTTFB func(...string)
	if sc != nil && stage != "" {
		endLLM = sc.Start(stage + ".llm")
		endTTFB = sc.Start(stage + ".llm.ttfb")
		trace := &httptrace.ClientTrace{
			GotFirstResponseByte: func() {
				if endTTFB != nil {
					endTTFB()
					endTTFB = nil
				}
			},
		}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if endTTFB != nil {
			endTTFB()
		}
		if endLLM != nil {
			endLLM()
		}
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Read body span
	var endBody func(...string)
	if sc != nil && stage != "" {
		endBody = sc.Start(stage + ".llm.body")
	}

	var anthroResp anthropicToolResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthroResp); err != nil {
		if endBody != nil {
			endBody()
		}
		if endLLM != nil {
			endLLM()
		}
		return nil, fmt.Errorf("decode response: %w", err)
	}

	respSize := 0
	if anthroResp.Usage.InputTokens > 0 {
		respSize = anthroResp.Usage.OutputTokens * 4 // rough estimate
	}

	if endBody != nil {
		endBody()
	}
	if endLLM != nil {
		detail := fmt.Sprintf("%dKB→%dKB", reqSize/1024, respSize/1024)
		endLLM(detail)
	}

	if anthroResp.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", anthroResp.Error.Message)
	}

	// Convert response
	result := &domain.LLMResponse{
		StopReason: anthroResp.StopReason,
	}

	// Parse usage
	result.Usage = domain.LLMUsage{
		InputTokens:  anthroResp.Usage.InputTokens,
		OutputTokens: anthroResp.Usage.OutputTokens,
		TotalTokens:  anthroResp.Usage.InputTokens + anthroResp.Usage.OutputTokens,
		Model:        c.model,
	}
	result.Usage.CostUSD = result.Usage.CalculateCost()

	for _, block := range anthroResp.Content {
		switch block.Type {
		case "text":
			result.Text = block.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, domain.ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}

	return result, nil
}

// ChatWithToolsCached sends messages with prompt caching enabled.
// Note: Prompt caching is GA since Dec 2024 — no beta header needed.
// Go's encoding/json.Marshal sorts map keys deterministically (since Go 1.12),
// so tool InputSchema serialization is cache-stable without extra handling.
func (c *Client) ChatWithToolsCached(
	ctx context.Context,
	systemPrompt string,
	messages []domain.LLMMessage,
	tools []domain.ToolDefinition,
	cacheConfig *ports.CacheConfig,
) (*domain.LLMResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("anthropic API key not configured")
	}

	// If no cache config, fall back to non-cached version
	if cacheConfig == nil {
		return c.ChatWithTools(ctx, systemPrompt, messages, tools)
	}

	// Convert tools with cache_control on last element
	anthroTools := make([]anthropicToolWithCache, 0, len(tools))
	for i, t := range tools {
		tool := anthropicToolWithCache{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
		// Add cache_control to the last tool
		if cacheConfig.CacheTools && i == len(tools)-1 {
			tool.CacheControl = &cacheControl{Type: "ephemeral"}
		}
		anthroTools = append(anthroTools, tool)
	}

	// Build system as array with cache_control
	systemBlocks := []contentBlockWithCache{
		{
			Type: "text",
			Text: systemPrompt,
		},
	}
	if cacheConfig.CacheSystem {
		systemBlocks[0].CacheControl = &cacheControl{Type: "ephemeral"}
	}

	// Convert domain messages to Anthropic format
	anthroMsgs := make([]anthropicToolMsg, 0, len(messages))
	for _, msg := range messages {
		anthroMsgs = append(anthroMsgs, convertToAnthropicMessage(msg))
	}

	// Add cache_control to the last message in conversation history (second-to-last overall)
	// so that new user messages don't invalidate the cache
	if cacheConfig.CacheConversation && len(anthroMsgs) > 1 {
		lastHistoryIdx := len(anthroMsgs) - 2 // second-to-last = end of history
		markMessageCacheControl(&anthroMsgs[lastHistoryIdx])
	}

	reqBody := anthropicCachedRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemBlocks,
		Messages:  anthroMsgs,
		Tools:     anthroTools,
	}

	// Set tool_choice if specified
	if cacheConfig.ToolChoice != "" && cacheConfig.ToolChoice != "auto" {
		if cacheConfig.ToolChoice == "any" {
			reqBody.ToolChoice = &toolChoiceConfig{Type: "any"}
		} else if strings.HasPrefix(cacheConfig.ToolChoice, "tool:") {
			reqBody.ToolChoice = &toolChoiceConfig{
				Type: "tool",
				Name: strings.TrimPrefix(cacheConfig.ToolChoice, "tool:"),
			}
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Span instrumentation via httptrace
	sc := domain.SpanFromContext(ctx)
	stage := domain.StageFromContext(ctx)
	reqSize := len(jsonBody)
	ttfbStart := time.Now()

	var endLLM func(...string)
	var endTTFB func(...string)
	if sc != nil && stage != "" {
		endLLM = sc.Start(stage + ".llm")
		endTTFB = sc.Start(stage + ".llm.ttfb")
		trace := &httptrace.ClientTrace{
			GotFirstResponseByte: func() {
				ttfbDuration := time.Since(ttfbStart)
				if ttfbDuration > 10*time.Second {
					c.log.Warn("slow LLM TTFB", "stage", stage, "duration", ttfbDuration)
				}
				if endTTFB != nil {
					endTTFB()
					endTTFB = nil
				}
			},
		}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if endTTFB != nil {
			endTTFB()
		}
		if endLLM != nil {
			endLLM()
		}
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Read body span
	var endBody func(...string)
	if sc != nil && stage != "" {
		endBody = sc.Start(stage + ".llm.body")
	}

	var anthroResp anthropicCachedResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthroResp); err != nil {
		if endBody != nil {
			endBody()
		}
		if endLLM != nil {
			endLLM()
		}
		return nil, fmt.Errorf("decode response: %w", err)
	}

	respSize := 0
	if anthroResp.Usage.OutputTokens > 0 {
		respSize = anthroResp.Usage.OutputTokens * 4
	}

	if endBody != nil {
		endBody()
	}
	if endLLM != nil {
		detail := fmt.Sprintf("%dKB→%dKB", reqSize/1024, respSize/1024)
		endLLM(detail)
	}

	if anthroResp.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", anthroResp.Error.Message)
	}

	// Convert response with cache metrics
	result := &domain.LLMResponse{
		StopReason: anthroResp.StopReason,
	}

	result.Usage = domain.LLMUsage{
		InputTokens:              anthroResp.Usage.InputTokens,
		OutputTokens:             anthroResp.Usage.OutputTokens,
		TotalTokens:              anthroResp.Usage.InputTokens + anthroResp.Usage.OutputTokens + anthroResp.Usage.CacheCreationInputTokens + anthroResp.Usage.CacheReadInputTokens,
		Model:                    c.model,
		CacheCreationInputTokens: anthroResp.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     anthroResp.Usage.CacheReadInputTokens,
	}
	result.Usage.CostUSD = result.Usage.CalculateCost()

	for _, block := range anthroResp.Content {
		switch block.Type {
		case "text":
			result.Text = block.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, domain.ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}

	return result, nil
}

// markMessageCacheControl adds cache_control to a message
// For text content, wraps in content block array with cache_control
// For tool_use/tool_result blocks, preserves all fields while adding cache_control
func markMessageCacheControl(msg *anthropicToolMsg) {
	switch content := msg.Content.(type) {
	case string:
		// Wrap string content in array with cache_control
		msg.Content = []contentBlockWithCache{
			{
				Type:         "text",
				Text:         content,
				CacheControl: &cacheControl{Type: "ephemeral"},
			},
		}
	case []contentBlock:
		// Already an array — use contentBlockFullCache to preserve all fields (id, name, input, etc.)
		if len(content) > 0 {
			blocks := make([]contentBlockFullCache, 0, len(content))
			for i, b := range content {
				cached := contentBlockFullCache{
					Type:      b.Type,
					Text:      b.Text,
					ID:        b.ID,
					Name:      b.Name,
					Input:     b.Input,
					ToolUseID: b.ToolUseID,
					Content:   b.Content,
					IsError:   b.IsError,
				}
				if i == len(content)-1 {
					cached.CacheControl = &cacheControl{Type: "ephemeral"}
				}
				blocks = append(blocks, cached)
			}
			msg.Content = blocks
		}
	}
}

// convertToAnthropicMessage converts domain message to Anthropic format
func convertToAnthropicMessage(msg domain.LLMMessage) anthropicToolMsg {
	if msg.ToolResult != nil {
		// Tool result message
		return anthropicToolMsg{
			Role: "user",
			Content: []contentBlock{{
				Type:      "tool_result",
				ToolUseID: msg.ToolResult.ToolUseID,
				Content:   msg.ToolResult.Content,
				IsError:   msg.ToolResult.IsError,
			}},
		}
	}

	if len(msg.ToolCalls) > 0 {
		// Assistant message with tool calls
		blocks := make([]contentBlock, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			blocks = append(blocks, contentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Name,
				Input: tc.Input,
			})
		}
		return anthropicToolMsg{Role: "assistant", Content: blocks}
	}

	// Simple text message
	return anthropicToolMsg{Role: msg.Role, Content: msg.Content}
}
