package postgres

import (
	"context"
	"fmt"
)

const migrationChatSessionState = `
CREATE TABLE IF NOT EXISTS chat_session_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    current_data JSONB DEFAULT '{}',
    current_meta JSONB DEFAULT '{}',
    current_template JSONB,
    step INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(session_id)
);
`

const migrationChatSessionDeltas = `
CREATE TABLE IF NOT EXISTS chat_session_deltas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    step INTEGER NOT NULL,
    trigger VARCHAR(20) NOT NULL,
    action JSONB NOT NULL,
    result JSONB NOT NULL,
    template JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(session_id, step)
);
`

const migrationStateIndexes = `
CREATE INDEX IF NOT EXISTS idx_chat_session_state_session_id
    ON chat_session_state(session_id);
CREATE INDEX IF NOT EXISTS idx_chat_session_deltas_session_id
    ON chat_session_deltas(session_id);
CREATE INDEX IF NOT EXISTS idx_chat_session_deltas_session_step
    ON chat_session_deltas(session_id, step);
`

// RunStateMigrations executes state-related migrations
func (c *Client) RunStateMigrations(ctx context.Context) error {
	migrations := []string{
		migrationChatSessionState,
		migrationChatSessionDeltas,
		migrationStateIndexes,
	}

	for i, migration := range migrations {
		if _, err := c.pool.Exec(ctx, migration); err != nil {
			return fmt.Errorf("state migration %d failed: %w", i+1, err)
		}
	}

	return nil
}
