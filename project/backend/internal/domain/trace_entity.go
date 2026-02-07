package domain

import "time"

// PipelineTrace captures the full trace of one pipeline execution
type PipelineTrace struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Query     string    `json:"query"`
	TurnID    string    `json:"turnId"`
	Timestamp time.Time `json:"timestamp"`

	// Agent1
	Agent1 *AgentTrace `json:"agent1,omitempty"`

	// State snapshot after Agent1
	StateAfterAgent1 *StateSnapshot `json:"stateAfterAgent1,omitempty"`

	// Agent2
	Agent2 *AgentTrace `json:"agent2,omitempty"`

	// Formation result
	FormationResult *FormationTrace `json:"formationResult,omitempty"`

	// Pipeline totals
	TotalMs int     `json:"totalMs"`
	Error   string  `json:"error,omitempty"`
	CostUSD float64 `json:"costUsd"`
}

// AgentTrace captures one agent's execution
type AgentTrace struct {
	Name     string `json:"name"` // "agent1" or "agent2"
	LLMMs    int64  `json:"llmMs"`
	ToolMs   int64  `json:"toolMs,omitempty"`
	TotalMs  int    `json:"totalMs"`
	StopReason string `json:"stopReason,omitempty"`

	// LLM
	Model        string  `json:"model"`
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CacheRead    int     `json:"cacheRead,omitempty"`
	CacheWrite   int     `json:"cacheWrite,omitempty"`
	CostUSD      float64 `json:"costUsd"`

	// Prompt breakdown
	SystemPrompt      string `json:"systemPrompt,omitempty"`
	SystemPromptChars int    `json:"systemPromptChars,omitempty"`
	MessageCount      int    `json:"messageCount,omitempty"`
	ToolDefCount      int    `json:"toolDefCount,omitempty"`

	// Tool
	ToolName      string                 `json:"toolName,omitempty"`
	ToolInput     string                 `json:"toolInput,omitempty"`
	ToolResult    string                 `json:"toolResult,omitempty"`
	ToolBreakdown map[string]interface{} `json:"toolBreakdown,omitempty"` // Internal tool breakdown (normalize, fallback, etc.)

	// Agent2-specific
	PromptSent  string `json:"promptSent,omitempty"`
	RawResponse string `json:"rawResponse,omitempty"`
}

// StateSnapshot captures state at a point in the pipeline
type StateSnapshot struct {
	ProductCount int               `json:"productCount"`
	ServiceCount int               `json:"serviceCount"`
	Fields       []string          `json:"fields,omitempty"`
	Aliases      map[string]string `json:"aliases,omitempty"`
	HasTemplate  bool              `json:"hasTemplate"`
	DeltaCount   int               `json:"deltaCount"`

	// Detailed delta log for this turn
	Deltas []DeltaTrace `json:"deltas,omitempty"`
}

// DeltaTrace captures one delta's key info for the trace
type DeltaTrace struct {
	Step      int      `json:"step"`
	ActorID   string   `json:"actorId"`
	DeltaType string   `json:"deltaType"` // add, remove, update
	Path      string   `json:"path"`      // data.products, view.mode, etc.
	Tool      string   `json:"tool,omitempty"`
	Count     int      `json:"count,omitempty"`
	Fields    []string `json:"fields,omitempty"`
}

// FormationTrace captures the formation output
type FormationTrace struct {
	Mode        string `json:"mode"`
	WidgetCount int    `json:"widgetCount"`
	Cols        int    `json:"cols,omitempty"`
	FirstWidget string `json:"firstWidget,omitempty"` // name of first widget for quick check
}
