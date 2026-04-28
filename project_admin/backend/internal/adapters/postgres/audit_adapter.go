// Package postgres — append-only audit log writes.
// Spec: docs/New features/admin_catalog_design_2026-04-23.md §3.7.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type AuditAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewAuditAdapter(client *Client, log *logger.Logger) *AuditAdapter {
	return &AuditAdapter{client: client, log: log}
}

var _ ports.AuditPort = (*AuditAdapter)(nil)

// LogSystem writes one row for a bulk action. Use a single batch_id across
// related ops in aggregateMeta to make later analysis easy.
func (a *AuditAdapter) LogSystem(ctx context.Context, tenantID, actorID string, entityKind domain.EntityKind, entityID string, action domain.AuditAction, aggregateMeta map[string]interface{}) error {
	if aggregateMeta == nil {
		aggregateMeta = map[string]interface{}{}
	}
	metaJSON, _ := json.Marshal(aggregateMeta)
	var tenantArg interface{}
	if tenantID != "" {
		tenantArg = tenantID
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.audit_log
			(tenant_id, actor_kind, actor_id, entity_kind, entity_id, action, aggregate_meta)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenantArg, domain.ActorKindSystem, actorID, entityKind, entityID, action, metaJSON)
	if err != nil {
		return fmt.Errorf("audit log_system: %w", err)
	}
	return nil
}

// LogHuman writes one row per human action with field-level diff.
func (a *AuditAdapter) LogHuman(ctx context.Context, tenantID, actorID string, entityKind domain.EntityKind, entityID string, action domain.AuditAction, fieldChanges map[string]domain.FieldChange) error {
	var changesJSON []byte
	if len(fieldChanges) > 0 {
		changesJSON, _ = json.Marshal(fieldChanges)
	}
	var tenantArg interface{}
	if tenantID != "" {
		tenantArg = tenantID
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.audit_log
			(tenant_id, actor_kind, actor_id, entity_kind, entity_id, action, field_changes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenantArg, domain.ActorKindUser, actorID, entityKind, entityID, action, changesJSON)
	if err != nil {
		return fmt.Errorf("audit log_human: %w", err)
	}
	return nil
}

// LogCurator writes one row stamped actor_kind='curator'. Used by the catalog
// merge-apply / revert flow so the audit log preserves the operator role even
// when the curator user id overlaps with a tenant-side user id.
func (a *AuditAdapter) LogCurator(ctx context.Context, tenantID, curatorID string, entityKind domain.EntityKind, entityID string, action domain.AuditAction, fieldChanges map[string]domain.FieldChange, aggregateMeta map[string]interface{}) error {
	var changesJSON, metaJSON []byte
	if len(fieldChanges) > 0 {
		changesJSON, _ = json.Marshal(fieldChanges)
	}
	if len(aggregateMeta) > 0 {
		metaJSON, _ = json.Marshal(aggregateMeta)
	}
	var tenantArg interface{}
	if tenantID != "" {
		tenantArg = tenantID
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.audit_log
			(tenant_id, actor_kind, actor_id, entity_kind, entity_id, action, field_changes, aggregate_meta)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tenantArg, domain.ActorKindCurator, curatorID, entityKind, entityID, action, changesJSON, metaJSON)
	if err != nil {
		return fmt.Errorf("audit log_curator: %w", err)
	}
	return nil
}

func (a *AuditAdapter) ListAuditEntries(ctx context.Context, entityKind domain.EntityKind, entityID string, limit, offset int) ([]domain.AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, COALESCE(tenant_id::text, ''), actor_kind, COALESCE(actor_id, ''),
			entity_kind, entity_id, action, field_changes, aggregate_meta, created_at
		FROM catalog.audit_log
		WHERE entity_kind = $1 AND entity_id = $2
		ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
		entityKind, entityID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list audit_entries: %w", err)
	}
	defer rows.Close()
	var out []domain.AuditEntry
	for rows.Next() {
		var e domain.AuditEntry
		var fieldChanges, aggregateMeta []byte
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ActorKind, &e.ActorID,
			&e.EntityKind, &e.EntityID, &e.Action, &fieldChanges, &aggregateMeta,
			&e.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(fieldChanges, &e.FieldChanges)
		_ = json.Unmarshal(aggregateMeta, &e.AggregateMeta)
		out = append(out, e)
	}
	return out, rows.Err()
}
