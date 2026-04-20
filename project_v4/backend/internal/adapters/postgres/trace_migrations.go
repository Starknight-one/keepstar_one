package postgres

import (
	"context"
	"fmt"
)

const migrationPipelineTraces = `
CREATE TABLE IF NOT EXISTS pipeline_traces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id TEXT NOT NULL,
    query TEXT NOT NULL,
    turn_id TEXT,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    trace_data JSONB NOT NULL,
    total_ms INTEGER NOT NULL DEFAULT 0,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    error TEXT
);

CREATE INDEX IF NOT EXISTS idx_pipeline_traces_timestamp
    ON pipeline_traces(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_pipeline_traces_session_id
    ON pipeline_traces(session_id);
`

// RunTraceMigrations creates the pipeline_traces table
func (c *Client) RunTraceMigrations(ctx context.Context) error {
	if _, err := c.pool.Exec(ctx, migrationPipelineTraces); err != nil {
		return fmt.Errorf("trace migration failed: %w", err)
	}
	return nil
}
