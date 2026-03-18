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
