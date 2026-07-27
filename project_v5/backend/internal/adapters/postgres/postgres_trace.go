package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar_v5/internal/domain"
)

// TraceAdapter writes per-turn span trees into v5_chat_session_traces.
// Read side lives in Curator (it queries the same Neon DB directly).
type TraceAdapter struct {
	client *Client
}

// NewTraceAdapter constructs the adapter.
func NewTraceAdapter(client *Client) *TraceAdapter {
	return &TraceAdapter{client: client}
}

// SaveTrace inserts a single trace row. Idempotent on
// (session_id, request_id) via ON CONFLICT DO NOTHING — handlers may
// retry or fire the persist twice under unusual flow control, and we
// don't want either to surface as an error.
func (a *TraceAdapter) SaveTrace(ctx context.Context, row domain.TraceRow) error {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.SaveTrace")
		defer end(row.RequestID)
	}

	spansJSON, err := json.Marshal(row.Spans)
	if err != nil {
		return fmt.Errorf("marshal spans: %w", err)
	}
	if row.Status == "" {
		row.Status = "ok"
	}

	const q = `
        INSERT INTO v5_chat_session_traces (
            session_id, request_id, tenant_id, user_query, spans, span_count,
            latency_ms, agent1_ms, agent2_ms,
            tokens_input, tokens_output, tokens_cache_read, tokens_cache_creation,
            cost_usd, status, error_message, mode
        )
        VALUES (
            $1::uuid, $2, $3, $4, $5::jsonb, $6,
            $7, $8, $9,
            $10, $11, $12, $13,
            $14, $15, $16, $17
        )
        ON CONFLICT (session_id, request_id) DO NOTHING
    `
	_, err = a.client.pool.Exec(ctx, q,
		row.SessionID, row.RequestID, row.TenantID, row.UserQuery,
		string(spansJSON), row.SpanCount,
		row.LatencyMs, nullableInt64(row.Agent1Ms), nullableInt64(row.Agent2Ms),
		nullableInt64(row.TokensInput), nullableInt64(row.TokensOutput),
		nullableInt64(row.TokensCacheRead), nullableInt64(row.TokensCacheCreation),
		nullableFloat(row.CostUSD), row.Status, nullableStr(row.ErrorMessage),
		nullableStr(row.Mode),
	)
	if err != nil {
		return fmt.Errorf("insert trace: %w", err)
	}
	return nil
}

// nullableInt64 → nil for 0, value otherwise. Lets pg store NULL
// for «we don't have this metric» rather than a misleading 0.
func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableFloat(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}
