package domain

import (
	"encoding/json"
	"time"
)

// PipelineTrace captures the full trace of one pipeline execution
type PipelineTrace struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Query     string    `json:"query"`
	TurnID    string    `json:"turnId"`
	Timestamp time.Time `json:"timestamp"`

	// Pipeline context
	Microcontext  string         `json:"microcontext,omitempty"`  // Agent1→Agent2 bridge signal
	ScreenContext *ScreenContext `json:"screenContext,omitempty"` // Frontend viewport state

	// Agent1
	Agent1 *AgentTrace `json:"agent1,omitempty"`

	// State snapshot after Agent1
	StateAfterAgent1 *StateSnapshot `json:"stateAfterAgent1,omitempty"`

	// Agent2
	Agent2 *AgentTrace `json:"agent2,omitempty"`

	// State snapshot after Agent2
	StateAfterAgent2 *StateSnapshot `json:"stateAfterAgent2,omitempty"`

	// Formation result
	FormationResult *FormationTrace `json:"formationResult,omitempty"`

	// Pipeline totals
	TotalMs int     `json:"totalMs"`
	Error   string  `json:"error,omitempty"`
	CostUSD float64 `json:"costUsd"`

	// Span waterfall
	Spans []Span `json:"spans,omitempty"`
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
	EnrichedQuery     string `json:"enrichedQuery,omitempty"` // User query with <state> context (Agent1)
	MessageCount      int    `json:"messageCount,omitempty"`
	ToolDefCount      int    `json:"toolDefCount,omitempty"`

	// Tool
	ToolName      string                 `json:"toolName,omitempty"`
	ToolInput     string                 `json:"toolInput,omitempty"`
	ToolResult    string                 `json:"toolResult,omitempty"`
	ToolBreakdown map[string]interface{} `json:"toolBreakdown,omitempty"` // Internal tool breakdown (normalize, fallback, etc.)

	// LLM reasoning text (before tool_use)
	RawReasoning string `json:"rawReasoning,omitempty"`

	// Agent2-specific
	PromptSent  string `json:"promptSent,omitempty"`
	RawResponse string `json:"rawResponse,omitempty"`
}

// ScreenContext represents the current UI state from the frontend
type ScreenContext struct {
	Mode        string   `json:"mode"`
	WidgetCount int      `json:"widgetCount"`
	Fields      []string `json:"fields"`
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

	// Enriched state sections
	Meta            *StateMeta `json:"meta,omitempty"`
	ViewMode        string     `json:"viewMode,omitempty"`
	ViewFocused     *EntityRef `json:"viewFocused,omitempty"`
	ViewStackLen    int        `json:"viewStackLen,omitempty"`
	LikedCount      int        `json:"likedCount,omitempty"`
	CartCount       int        `json:"cartCount,omitempty"`
	TemplateSummary string     `json:"templateSummary,omitempty"`
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
	Mode        string          `json:"mode"`
	WidgetCount int             `json:"widgetCount"`
	Cols        int             `json:"cols,omitempty"`
	FirstWidget string          `json:"firstWidget,omitempty"` // name of first widget for quick check
	Widgets     []WidgetTrace   `json:"widgets,omitempty"`
	FullJSON    json.RawMessage `json:"fullJson,omitempty"` // full formation JSON (up to 100KB)
}

// WidgetTrace captures key info about a single widget
type WidgetTrace struct {
	ID        string `json:"id"`
	Template  string `json:"template"`
	Size      string `json:"size,omitempty"`
	AtomCount int    `json:"atomCount"`
	EntityRef string `json:"entityRef,omitempty"`
}
