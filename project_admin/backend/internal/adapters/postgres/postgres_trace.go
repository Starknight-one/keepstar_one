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
	LastActivityAt time.Time  `json:"lastActivityAt"`
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
