package postgres

import (
	"context"
	"fmt"
)

// migrationV5StateInit creates the V5 state schema in one shot. V4 evolved
// through 8 incremental ALTERs; we bake the final shape directly so a fresh
// V5 environment runs one DDL.
const migrationV5StateInit = `
CREATE TABLE IF NOT EXISTS v5_chat_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_v5_chat_sessions_tenant_id
    ON v5_chat_sessions(tenant_id);

CREATE TABLE IF NOT EXISTS v5_chat_session_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES v5_chat_sessions(id) ON DELETE CASCADE,
    current_data JSONB DEFAULT '{}',
    current_meta JSONB DEFAULT '{}',
    current_template JSONB,
    view_mode VARCHAR(20) DEFAULT 'grid',
    view_focused JSONB,
    view_stack JSONB DEFAULT '[]',
    conversation_history JSONB DEFAULT '[]',
    agent2_history JSONB DEFAULT '[]',
    actions JSONB DEFAULT '{"likedIds":[],"cartItems":[]}',
    step INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(session_id)
);

CREATE INDEX IF NOT EXISTS idx_v5_chat_session_state_session_id
    ON v5_chat_session_state(session_id);

CREATE TABLE IF NOT EXISTS v5_chat_session_deltas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES v5_chat_sessions(id) ON DELETE CASCADE,
    step INTEGER NOT NULL,
    trigger VARCHAR(20) NOT NULL,
    source VARCHAR(20) DEFAULT 'llm',
    actor_id VARCHAR(50),
    delta_type VARCHAR(20) DEFAULT 'add',
    path VARCHAR(100),
    action JSONB NOT NULL,
    result JSONB NOT NULL,
    template JSONB,
    turn_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(session_id, step)
);

CREATE INDEX IF NOT EXISTS idx_v5_chat_session_deltas_session_id
    ON v5_chat_session_deltas(session_id);
CREATE INDEX IF NOT EXISTS idx_v5_chat_session_deltas_session_step
    ON v5_chat_session_deltas(session_id, step);
CREATE INDEX IF NOT EXISTS idx_v5_chat_session_deltas_source
    ON v5_chat_session_deltas(session_id, source);
`

// RunStateMigrations applies the V5 state schema. Idempotent (CREATE IF NOT EXISTS).
func (c *Client) RunStateMigrations(ctx context.Context) error {
	if _, err := c.pool.Exec(ctx, migrationV5StateInit); err != nil {
		return fmt.Errorf("v5 state migration: %w", err)
	}
	return nil
}
