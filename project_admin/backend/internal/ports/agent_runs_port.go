package ports

import (
	"context"
	"encoding/json"
	"time"
)

// AgentRunsPort persists rows of admin.agent_runs — one row per discovery_v2
// invocation. Used by discovery_v2 itself (Start/AppendTool/Finish), by
// update_orchestrator (mapping-miss triggers), and by curator's "Agent Runs"
// tab to show timeline + tool waterfall + cost.
//
// Trigger values: 'first_install' | 'manual' | 'mapping_miss'
// Status values:  'running' | 'success' | 'failed' | 'budget_exhausted'
type AgentRunsPort interface {
	// Start inserts a new row with status='running' and returns its ID.
	Start(ctx context.Context, in *AgentRunStart) (id string, err error)

	// AppendTool records one tool-call result into tools_called[] without
	// rewriting the whole array (Postgres jsonb_set or || append).
	AppendTool(ctx context.Context, runID string, call AgentToolCall) error

	// AddTokens accumulates token counts and dollar cost on top of current
	// values. Called once per LLM round-trip inside discovery_v2.
	AddTokens(ctx context.Context, runID string, tokensIn, tokensOut int, costDeltaUSD float64) error

	// Finish stamps finished_at + final status. ArtifactID is non-empty when
	// the run produced a committed mapping artifact (success path).
	Finish(ctx context.Context, runID string, status string, artifactID string) error

	// GetByID returns the full row including tools_called.
	GetByID(ctx context.Context, runID string) (*AgentRun, error)

	// ListForTenant returns recent runs for a tenant, newest first.
	ListForTenant(ctx context.Context, tenantID string, limit, offset int) ([]*AgentRun, error)
}

// AgentRunStart bundles the inputs needed to open a new run.
type AgentRunStart struct {
	TenantID       string
	Trigger        string          // 'first_install' | 'manual' | 'mapping_miss'
	TriggerPayload json.RawMessage // free-form context (e.g. mapping-miss details)
}

// AgentToolCall is one tool invocation summary — kept small on purpose so
// the tools_called JSONB doesn't bloat. Heavy results (e.g. peek_full_rows)
// store just a count + first-N-keys preview, not the raw payload.
type AgentToolCall struct {
	Name       string          `json:"name"`
	Args       json.RawMessage `json:"args"`
	ResultPrev json.RawMessage `json:"result_preview,omitempty"`
	DurationMS int64           `json:"duration_ms"`
	Error      string          `json:"error,omitempty"`
	At         time.Time       `json:"at"`
}

// AgentRun mirrors one row.
type AgentRun struct {
	ID             string
	TenantID       string
	Trigger        string
	TriggerPayload json.RawMessage
	StartedAt      time.Time
	FinishedAt     *time.Time
	Status         string
	ToolsCalled    json.RawMessage // array of AgentToolCall
	TokensIn       int
	TokensOut      int
	CostUSD        float64
	ArtifactID     *string
}
