package postgres

import (
	"context"
	"fmt"
)

// Entity plane schema (RUNTIME_SPEC.md §3.2, rulings R3/R10/R12/R27/R29).
// public.v5_* namespace, v5-boot-owned, idempotent, additive-only; plain
// tenant_id UUID with no cross-schema FK (R29). v5_events is outbox + audit
// written in the SAME tx as the record write; processed_at is stamped by
// inline dispatch (sweeper is v2, R12).
//
// Statements run ONE PER Exec, not as a single multi-statement batch:
// Neon's proxy currently desyncs a pgx connection after a simple-protocol
// multi-statement query (the batch itself succeeds; the NEXT query on that
// connection hangs forever). Verified 2026-07-27 with a minimal probe —
// single-statement Execs are unaffected. The other migration runners
// (state/preset/theme/trace/component/operation) still use one-batch Exec
// and share this landmine.
var entityMigrationStatements = []string{
	`CREATE TABLE IF NOT EXISTS v5_value_sets (
	  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	  tenant_id UUID NOT NULL,
	  slug VARCHAR(100) NOT NULL, name VARCHAR(255) NOT NULL,
	  "values" JSONB NOT NULL DEFAULT '[]',        -- ordered [{value,label,color?}] (R27)
	  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	  UNIQUE(tenant_id, slug)
	)`,

	`CREATE TABLE IF NOT EXISTS v5_entity_definitions (
	  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	  tenant_id UUID NOT NULL,
	  slug VARCHAR(100) NOT NULL,                  -- "lead"
	  name VARCHAR(255) NOT NULL, name_plural VARCHAR(255),
	  fields JSONB NOT NULL DEFAULT '[]',          -- ordered []FieldDef (domain/entity_plane.go)
	  display JSONB NOT NULL DEFAULT '{}',         -- {titleField, subtitleField, badgeField, defaultSort}
	  status_field VARCHAR(100),
	  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	  UNIQUE(tenant_id, slug)
	)`,

	`CREATE TABLE IF NOT EXISTS v5_entity_records (
	  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	  tenant_id UUID NOT NULL,
	  entity_definition_id UUID NOT NULL REFERENCES v5_entity_definitions(id) ON DELETE CASCADE,
	  entity_slug VARCHAR(100) NOT NULL,           -- denormalized (slug rename rejected once records exist)
	  data JSONB NOT NULL DEFAULT '{}',            -- camelCase keys
	  status VARCHAR(100),                         -- mirror of data[status_field]; hot filter column
	  ref_entity_type VARCHAR(50), ref_entity_id UUID,  -- link into catalog plane ('product')
	  created_by TEXT,                             -- "visitor:<session_id>" | "user:<admin_user_id>"
	  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_v5_entity_records_tenant_entity ON v5_entity_records(tenant_id, entity_slug, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_v5_entity_records_status ON v5_entity_records(tenant_id, entity_slug, status)`,
	`CREATE INDEX IF NOT EXISTS idx_v5_entity_records_ref ON v5_entity_records(tenant_id, ref_entity_id) WHERE ref_entity_id IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_v5_entity_records_data_gin ON v5_entity_records USING gin(data jsonb_path_ops)`,

	// Flat v1 automations: on event [if predicate] run notify. No chains.
	`CREATE TABLE IF NOT EXISTS v5_automations (
	  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	  tenant_id UUID NOT NULL, name VARCHAR(255) NOT NULL,
	  event_type VARCHAR(100) NOT NULL,            -- record.created | record.status_changed
	  entity_slug VARCHAR(100),                    -- NULL = any
	  predicate JSONB,                             -- {field, op: eq|neq|in, value} | NULL
	  operation_slug VARCHAR(100) NOT NULL,        -- v1: 'notify' only
	  operation_params JSONB NOT NULL DEFAULT '{}',
	  enabled BOOLEAN NOT NULL DEFAULT TRUE,
	  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_v5_automations_tenant_event ON v5_automations(tenant_id, event_type) WHERE enabled`,

	// Outbox + audit, written in the SAME tx as the record write.
	`CREATE TABLE IF NOT EXISTS v5_events (
	  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	  tenant_id UUID NOT NULL,
	  event_type VARCHAR(100) NOT NULL,            -- record.created|record.updated|record.status_changed
	  entity_slug VARCHAR(100), record_id UUID,
	  payload JSONB NOT NULL DEFAULT '{}',         -- {actor, snapshot} | {actor, diff:{before,after}}
	  occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	  processed_at TIMESTAMPTZ                     -- stamped by inline dispatch; sweeper is v2 (R12)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_v5_events_unprocessed ON v5_events(id) WHERE processed_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_v5_events_tenant ON v5_events(tenant_id, occurred_at DESC)`,

	`CREATE TABLE IF NOT EXISTS v5_notifications (
	  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	  tenant_id UUID NOT NULL, audience VARCHAR(100) NOT NULL DEFAULT 'crm',
	  title TEXT NOT NULL, body TEXT,
	  entity_slug VARCHAR(100), record_id UUID,
	  read_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
}

// RunEntityMigrations applies the entity-plane schema. Idempotent
// (CREATE IF NOT EXISTS), additive-only, one statement per Exec (see the
// Neon note above).
func (c *Client) RunEntityMigrations(ctx context.Context) error {
	for i, stmt := range entityMigrationStatements {
		if _, err := c.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("v5 entity migration (statement %d): %w", i+1, err)
		}
	}
	return nil
}
