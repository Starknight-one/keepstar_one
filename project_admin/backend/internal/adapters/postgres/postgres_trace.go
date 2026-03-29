package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TraceAdapter provides read-only access to pipeline_traces table.
type TraceAdapter struct {
	client *Client
}

// NewTraceAdapter creates a new TraceAdapter.
func NewTraceAdapter(client *Client) *TraceAdapter {
	return &TraceAdapter{client: client}
}

// TraceListItem is a summary row for the traces list.
type TraceListItem struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId"`
	Query     string          `json:"query"`
	Timestamp time.Time       `json:"timestamp"`
	TotalMs   int             `json:"totalMs"`
	CostUSD   float64         `json:"costUsd"`
	Error     *string         `json:"error"`
	TraceData json.RawMessage `json:"traceData"`
}

// List returns paginated trace summaries.
func (a *TraceAdapter) List(ctx context.Context, limit, offset int) ([]TraceListItem, int, error) {
	// Count total
	var total int
	err := a.client.pool.QueryRow(ctx, `SELECT count(*) FROM pipeline_traces`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count traces: %w", err)
	}

	rows, err := a.client.pool.Query(ctx,
		`SELECT id, session_id, query, timestamp, total_ms, cost_usd, error, trace_data
		 FROM pipeline_traces
		 ORDER BY timestamp DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list traces: %w", err)
	}
	defer rows.Close()

	var items []TraceListItem
	for rows.Next() {
		var item TraceListItem
		if err := rows.Scan(&item.ID, &item.SessionID, &item.Query, &item.Timestamp, &item.TotalMs, &item.CostUSD, &item.Error, &item.TraceData); err != nil {
			return nil, 0, fmt.Errorf("scan trace: %w", err)
		}
		items = append(items, item)
	}

	return items, total, nil
}

// KillSession marks a chat session as closed.
func (a *TraceAdapter) KillSession(ctx context.Context, sessionID string) error {
	_, err := a.client.pool.Exec(ctx,
		`UPDATE chat_sessions SET status = 'closed', ended_at = NOW(), updated_at = NOW() WHERE id = $1`,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("kill session: %w", err)
	}
	return nil
}

// ListSessions returns active chat sessions.
func (a *TraceAdapter) ListSessions(ctx context.Context) ([]SessionInfo, error) {
	rows, err := a.client.pool.Query(ctx,
		`SELECT id, status, tenant_id, started_at, last_activity_at
		 FROM chat_sessions
		 ORDER BY last_activity_at DESC
		 LIMIT 50`,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var items []SessionInfo
	for rows.Next() {
		var s SessionInfo
		if err := rows.Scan(&s.ID, &s.Status, &s.TenantID, &s.StartedAt, &s.LastActivityAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		items = append(items, s)
	}
	return items, nil
}

// SessionInfo is a summary of a chat session.
type SessionInfo struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	TenantID       *string    `json:"tenantId"`
	StartedAt      time.Time  `json:"startedAt"`
	EndedAt        *time.Time `json:"endedAt"`
	LastActivityAt time.Time  `json:"lastActivityAt"`
}

// SessionListItem is a session row with aggregated stats from traces.
type SessionListItem struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	TenantID       *string    `json:"tenantId"`
	StartedAt      time.Time  `json:"startedAt"`
	EndedAt        *time.Time `json:"endedAt"`
	LastActivityAt time.Time  `json:"lastActivityAt"`
	TotalCostUSD   float64    `json:"totalCostUsd"`
	TraceCount     int        `json:"traceCount"`
	TotalTokens    int        `json:"totalTokens"`
	ActionCount    int        `json:"actionCount"`
	FirstQuery     *string    `json:"firstQuery"`
}

// UserActionItem is a user action from chat_session_deltas.
type UserActionItem struct {
	ID        string          `json:"id"`
	Step      int             `json:"step"`
	ActorID   string          `json:"actorId"`
	DeltaType string          `json:"deltaType"`
	Path      string          `json:"path"`
	Action    json.RawMessage `json:"action"`
	CreatedAt time.Time       `json:"createdAt"`
}

// Get returns the full trace data for a single trace.
func (a *TraceAdapter) Get(ctx context.Context, id string) (*TraceListItem, error) {
	var item TraceListItem
	err := a.client.pool.QueryRow(ctx,
		`SELECT id, session_id, query, timestamp, total_ms, cost_usd, error, trace_data
		 FROM pipeline_traces WHERE id = $1`, id,
	).Scan(&item.ID, &item.SessionID, &item.Query, &item.Timestamp, &item.TotalMs, &item.CostUSD, &item.Error, &item.TraceData)
	if err != nil {
		return nil, fmt.Errorf("get trace: %w", err)
	}
	return &item, nil
}

// ListSessionsWithStats returns sessions with aggregated trace stats.
func (a *TraceAdapter) ListSessionsWithStats(ctx context.Context, limit, offset int) ([]SessionListItem, int, error) {
	var total int
	err := a.client.pool.QueryRow(ctx, `SELECT count(*) FROM chat_sessions`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count sessions: %w", err)
	}

	rows, err := a.client.pool.Query(ctx, `
		SELECT
			cs.id, cs.status, cs.tenant_id, cs.started_at, cs.ended_at, cs.last_activity_at,
			COALESCE(t.total_cost, 0) AS total_cost_usd,
			COALESCE(t.trace_count, 0) AS trace_count,
			COALESCE(t.total_tokens, 0) AS total_tokens,
			COALESCE(a.action_count, 0) AS action_count,
			t.first_query
		FROM chat_sessions cs
		LEFT JOIN LATERAL (
			SELECT
				SUM(pt.cost_usd) AS total_cost,
				COUNT(*) AS trace_count,
				SUM(
					COALESCE((pt.trace_data->'agent1'->>'inputTokens')::int, 0) +
					COALESCE((pt.trace_data->'agent1'->>'outputTokens')::int, 0) +
					COALESCE((pt.trace_data->'agent2'->>'inputTokens')::int, 0) +
					COALESCE((pt.trace_data->'agent2'->>'outputTokens')::int, 0)
				) AS total_tokens,
				(SELECT query FROM pipeline_traces WHERE session_id = cs.id::text ORDER BY timestamp ASC LIMIT 1) AS first_query
			FROM pipeline_traces pt
			WHERE pt.session_id = cs.id::text
		) t ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS action_count
			FROM chat_session_deltas d
			WHERE d.session_id = cs.id AND d.source = 'user'
		) a ON true
		ORDER BY cs.last_activity_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list sessions with stats: %w", err)
	}
	defer rows.Close()

	var items []SessionListItem
	for rows.Next() {
		var s SessionListItem
		if err := rows.Scan(&s.ID, &s.Status, &s.TenantID, &s.StartedAt, &s.EndedAt, &s.LastActivityAt,
			&s.TotalCostUSD, &s.TraceCount, &s.TotalTokens, &s.ActionCount, &s.FirstQuery); err != nil {
			return nil, 0, fmt.Errorf("scan session: %w", err)
		}
		items = append(items, s)
	}
	return items, total, nil
}

// GetSession returns a single session by ID.
func (a *TraceAdapter) GetSession(ctx context.Context, id string) (*SessionInfo, error) {
	var s SessionInfo
	err := a.client.pool.QueryRow(ctx,
		`SELECT id, status, tenant_id, started_at, ended_at, last_activity_at
		 FROM chat_sessions WHERE id = $1`, id,
	).Scan(&s.ID, &s.Status, &s.TenantID, &s.StartedAt, &s.EndedAt, &s.LastActivityAt)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &s, nil
}

// ListBySession returns all traces for a given session, ordered chronologically.
func (a *TraceAdapter) ListBySession(ctx context.Context, sessionID string) ([]TraceListItem, error) {
	rows, err := a.client.pool.Query(ctx,
		`SELECT id, session_id, query, timestamp, total_ms, cost_usd, error, trace_data
		 FROM pipeline_traces
		 WHERE session_id = $1
		 ORDER BY timestamp ASC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list by session: %w", err)
	}
	defer rows.Close()

	var items []TraceListItem
	for rows.Next() {
		var item TraceListItem
		if err := rows.Scan(&item.ID, &item.SessionID, &item.Query, &item.Timestamp, &item.TotalMs, &item.CostUSD, &item.Error, &item.TraceData); err != nil {
			return nil, fmt.Errorf("scan trace: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

// ListUserActions returns user-initiated deltas for a session.
func (a *TraceAdapter) ListUserActions(ctx context.Context, sessionID string) ([]UserActionItem, error) {
	rows, err := a.client.pool.Query(ctx,
		`SELECT id, step, COALESCE(actor_id, ''), COALESCE(delta_type, ''), COALESCE(path, ''), action, created_at
		 FROM chat_session_deltas
		 WHERE session_id = $1 AND source = 'user'
		 ORDER BY step ASC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list user actions: %w", err)
	}
	defer rows.Close()

	var items []UserActionItem
	for rows.Next() {
		var item UserActionItem
		if err := rows.Scan(&item.ID, &item.Step, &item.ActorID, &item.DeltaType, &item.Path, &item.Action, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user action: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}
