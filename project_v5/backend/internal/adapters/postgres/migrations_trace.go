package postgres

import (
	"context"
	"fmt"
)

// migrationV5TraceInit — chunk 13. Per-pipeline-turn trace persistence
// so Curator UI (and the deferred /debug/traces UI) can inspect spans
// + tokens + cost across all tenants.
//
// Schema decisions (see plans/chunk-13-curator-chats.md):
//   - request_id (from logging middleware) is the per-turn identity;
//     we don't introduce a separate turn_id today.
//   - spans land as JSONB (variable-length, nested via parent_id).
//     Aggregates that drive list/sort queries (latency, cost, tokens,
//     status) live in their own columns to avoid scanning JSONB on
//     every Curator page load.
//   - cost_usd is NUMERIC(10,6) — fits ~9999.999999 USD, plenty of
//     headroom for per-turn costs that are typically under $0.05.
//   - status='ok'|'error' is a small enum stored as VARCHAR(20)
//     mirroring v5_chat_session_deltas.trigger.
//   - Partial index on status='error' so Curator's «show me error
//     turns» query is index-only.
const migrationV5TraceInit = `
CREATE TABLE IF NOT EXISTS v5_chat_session_traces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES v5_chat_sessions(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    user_query TEXT NOT NULL,
    spans JSONB NOT NULL,
    span_count INTEGER NOT NULL,
    latency_ms BIGINT NOT NULL,
    agent1_ms BIGINT,
    agent2_ms BIGINT,
    tokens_input BIGINT,
    tokens_output BIGINT,
    tokens_cache_read BIGINT,
    tokens_cache_creation BIGINT,
    cost_usd NUMERIC(10,6),
    status VARCHAR(20) NOT NULL DEFAULT 'ok',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(session_id, request_id)
);

CREATE INDEX IF NOT EXISTS idx_v5_traces_tenant_created
    ON v5_chat_session_traces(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_v5_traces_session_created
    ON v5_chat_session_traces(session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_v5_traces_status_created
    ON v5_chat_session_traces(status, created_at DESC) WHERE status = 'error';

-- Runtime v1 (RUNTIME_SPEC.md §3.3, R17): the form (mode) the turn ran in —
-- onboarding|storefront|crm, stamped by the pipeline handler from the session
-- row. NULL = legacy rows from before the column existed.
ALTER TABLE v5_chat_session_traces ADD COLUMN IF NOT EXISTS mode TEXT;
`

// RunTraceMigrations applies the V5 chunk-13 trace schema. Idempotent.
func (c *Client) RunTraceMigrations(ctx context.Context) error {
	if _, err := c.pool.Exec(ctx, migrationV5TraceInit); err != nil {
		return fmt.Errorf("v5 trace migration: %w", err)
	}
	return nil
}
