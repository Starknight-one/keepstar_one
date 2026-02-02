package domain

// ToolDefinition describes a tool for the LLM
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ToolCall represents a tool invocation from the LLM
type ToolCall struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"` // "ok", "empty", or error message
	IsError   bool   `json:"is_error,omitempty"`
}

// LLMMessage represents a message in conversation (extended for tools)
type LLMMessage struct {
	Role       string      `json:"role"` // "user", "assistant"
	Content    string      `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`  // For assistant
	ToolResult *ToolResult `json:"tool_result,omitempty"` // For user (tool_result)
}

// LLMResponse represents response from LLM with potential tool calls
type LLMResponse struct {
	Text       string     `json:"text,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"` // "end_turn", "tool_use"
	Usage      LLMUsage   `json:"usage"`
}

// LLMUsage tracks token usage and cost
type LLMUsage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	Model        string  `json:"model"`
	CostUSD      float64 `json:"cost_usd"`
}

// LLM pricing per million tokens (as of 2024)
// Source: https://platform.claude.com/docs/en/about-claude/pricing
var LLMPricing = map[string]struct {
	InputPerMillion  float64
	OutputPerMillion float64
}{
	"claude-haiku-4-5-20251001":  {1.0, 5.0},   // Haiku 4.5
	"claude-sonnet-4-5-20251014": {3.0, 15.0},  // Sonnet 4.5
	"claude-opus-4-5-20251101":   {5.0, 25.0},  // Opus 4.5
	"claude-3-5-sonnet-20241022": {3.0, 15.0},  // Sonnet 3.5
	"claude-3-haiku-20240307":    {0.25, 1.25}, // Haiku 3
}

// CalculateCost calculates USD cost for token usage
func (u *LLMUsage) CalculateCost() float64 {
	pricing, ok := LLMPricing[u.Model]
	if !ok {
		// Default to Haiku pricing if unknown model
		pricing = LLMPricing["claude-haiku-4-5-20251001"]
	}

	inputCost := float64(u.InputTokens) * pricing.InputPerMillion / 1_000_000
	outputCost := float64(u.OutputTokens) * pricing.OutputPerMillion / 1_000_000

	return inputCost + outputCost
}
