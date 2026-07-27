package domain

import "time"

// TraceRow is the v5_chat_session_traces row shape — one persisted
// span tree per pipeline turn. Lives in domain so both the V5
// adapter (writer) and any future consumer (Curator backend reading
// the same Neon DB) share the contract.
//
// Identity: (SessionID, RequestID) is the natural key — request_id
// is the unique-per-HTTP-call value the logging middleware stamps.
// We don't introduce a separate turn_id today; one HTTP turn = one
// trace row.
//
// Aggregates (LatencyMs, Tokens*, CostUSD, Status) are denormalised
// from Spans for fast list/sort queries on the Curator side. The
// full Spans tree stays in the JSONB column for waterfall
// rendering.
type TraceRow struct {
	ID        string `json:"id,omitempty"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	TenantID  string `json:"tenantId"`
	UserQuery string `json:"userQuery"`
	// Mode is the form the turn ran in (storefront|crm|onboarding, R17) —
	// stamped by the pipeline handler from the session row. Empty = legacy
	// rows persisted before mode plumbing existed (stored as NULL).
	Mode      string `json:"mode,omitempty"`
	Spans     []Span `json:"spans"`
	SpanCount int    `json:"spanCount"`
	LatencyMs int64  `json:"latencyMs"`
	Agent1Ms  int64  `json:"agent1Ms,omitempty"`
	Agent2Ms  int64  `json:"agent2Ms,omitempty"`

	TokensInput         int64 `json:"tokensInput,omitempty"`
	TokensOutput        int64 `json:"tokensOutput,omitempty"`
	TokensCacheRead     int64 `json:"tokensCacheRead,omitempty"`
	TokensCacheCreation int64 `json:"tokensCacheCreation,omitempty"`

	CostUSD      float64   `json:"costUsd,omitempty"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}
