package postgres

import (
	"context"
	"fmt"
)

// migrationV5OperationInit — operation plane schema (RUNTIME_SPEC.md §3.1,
// rulings R3/R14/R17/R29/R30). All tables live in public.v5_* (no runtime.*
// schema), created by v5 boot, idempotent and additive-only — no DROP, no
// type narrowing — so a Railway image rollback is always safe. Plain
// tenant_id UUID, no cross-schema FK (R29). No ivfflat index v1: the
// library is tens of rows, seq-scan cosine is fine.
//
// The v5_chat_sessions.mode column (R17) rides here rather than the state
// init so the state migration stays untouched; this migration is ordered
// after state in cmd/server/main.go, so the table always exists. A mode
// string is a form's storage key (see domain.PipelineMode) — the column is
// free TEXT, not an enum: forms are data, more will exist.
const migrationV5OperationInit = `
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS v5_operations (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name          TEXT NOT NULL UNIQUE,          -- wire name (R30: no version col)
  kind          TEXT NOT NULL,                 -- query|create_record|update_record|transition_status|
                                               -- schedule_slot|notify|meta|visual|internal|preset_pack
  title         TEXT NOT NULL,
  description   TEXT NOT NULL,                 -- embedded; LLM tool description default
  card          JSONB NOT NULL DEFAULT '{}',   -- {input_summary, does, output_summary, why} — op-card content
  input_schema  JSONB NOT NULL DEFAULT '{}',   -- restricted dialect w/ x-unit, x-sensitive
  output_schema JSONB NOT NULL DEFAULT '{}',
  effects       JSONB NOT NULL DEFAULT '{"reads":[],"writes":[],"emits":[]}',  -- emits: [{event_type, entity_slug}] (R10)
  config_schema JSONB NOT NULL DEFAULT '{}',
  modes         TEXT[] NOT NULL DEFAULT '{storefront}',
  agent         TEXT NOT NULL DEFAULT 'data',  -- data|visual
  min_role      TEXT NOT NULL DEFAULT 'visitor', -- visitor<staff<owner<system (R14)
  auto_enabled  BOOLEAN NOT NULL DEFAULT false,
  embedding     vector(384),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS v5_tenant_operations (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL,                 -- catalog.tenants.id, no FK (R29)
  operation_id  UUID NOT NULL REFERENCES v5_operations(id),
  name          TEXT NOT NULL,                 -- instance wire name, e.g. book_showing
  enabled       BOOLEAN NOT NULL DEFAULT true,
  config        JSONB NOT NULL DEFAULT '{}',   -- validated vs config_schema on write
  description   TEXT,                          -- NULL → template description
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_v5_tenant_operations_tenant ON v5_tenant_operations(tenant_id);

CREATE TABLE IF NOT EXISTS v5_operation_runs (      -- append-only audit; x-sensitive keys REDACTED pre-insert (R6)
  id BIGSERIAL PRIMARY KEY,
  tenant_id UUID NOT NULL, session_id TEXT, turn_id TEXT,
  operation TEXT NOT NULL, kind TEXT NOT NULL, mode TEXT NOT NULL,
  actor_role TEXT NOT NULL, actor_id TEXT,
  input JSONB NOT NULL, output JSONB,
  outcome TEXT NOT NULL,                        -- ok|empty|invalid|denied|error
  error TEXT, latency_ms INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_v5_operation_runs_tenant_time ON v5_operation_runs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_v5_operation_runs_session ON v5_operation_runs(session_id);

-- R17: form (mode) plumbing. Truth = the session row; the pipeline handler
-- reads it into PipelineExecuteRequest.Mode.
ALTER TABLE v5_chat_sessions ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT 'storefront';
`

// RunOperationMigrations applies the operation-plane schema. Idempotent
// (CREATE IF NOT EXISTS / ADD COLUMN IF NOT EXISTS). Must run AFTER
// RunStateMigrations (ALTERs v5_chat_sessions).
func (c *Client) RunOperationMigrations(ctx context.Context) error {
	if _, err := c.pool.Exec(ctx, migrationV5OperationInit); err != nil {
		return fmt.Errorf("v5 operation migration: %w", err)
	}
	return nil
}
