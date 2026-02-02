package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

const apiURL = "https://api.anthropic.com/v1/messages"

// Client implements ports.LLMPort for Anthropic API
type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewClient creates a new Anthropic client
func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
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
