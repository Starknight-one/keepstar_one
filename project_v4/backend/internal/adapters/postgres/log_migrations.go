package postgres

import (
	"context"
	"fmt"
)

// RunLogMigrations creates the request_logs table and indexes
func (c *Client) RunLogMigrations(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS request_logs (
			id TEXT PRIMARY KEY,
			timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			service TEXT NOT NULL DEFAULT 'chat',
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			status INTEGER NOT NULL,
			duration_ms BIGINT NOT NULL,
			session_id TEXT,
			tenant_slug TEXT,
			user_id TEXT,
			error TEXT,
			spans JSONB,
			metadata JSONB
		)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_timestamp ON request_logs (timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_session ON request_logs (session_id) WHERE session_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_errors ON request_logs (timestamp) WHERE status >= 400`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_service ON request_logs (service, timestamp)`,
	}

	for _, q := range queries {
		if _, err := c.pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("log migration: %w", err)
		}
	}
	return nil
}
