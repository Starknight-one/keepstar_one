package domain

// ToolCall represents a tool invocation from the LLM.
type ToolCall struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	ToolUseID string                 `json:"tool_use_id"`
	Content   string                 `json:"content"`
	IsError   bool                   `json:"is_error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// LLMMessage represents a message in conversation (extended for tools).
type LLMMessage struct {
	Role       string      `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
}
