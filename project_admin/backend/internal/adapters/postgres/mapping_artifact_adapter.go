// Package postgres — per-tenant mapping artifact storage.
// Spec: docs/New features/admin_catalog_design_2026-04-23.md §1.6.1, §3.4.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type MappingArtifactAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewMappingArtifactAdapter(client *Client, log *logger.Logger) *MappingArtifactAdapter {
	return &MappingArtifactAdapter{client: client, log: log}
}

var _ ports.MappingArtifactPort = (*MappingArtifactAdapter)(nil)

// SaveMappingArtifact upserts. If the row exists, version is bumped by caller
// (passed in via schema.ArtifactVersion). DiscoveredAt updated only if NULL.
func (a *MappingArtifactAdapter) SaveMappingArtifact(ctx context.Context, schema *domain.TenantCatalogSchema) error {
	if schema.TenantID == "" {
		return errors.New("mapping_artifact: tenant_id required")
	}
	artifactJSON, err := json.Marshal(schema.MappingArtifact)
	if err != nil {
		return fmt.Errorf("marshal mapping_artifact: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		INSERT INTO catalog.tenant_catalog_schema
			(tenant_id, mapping_artifact, artifact_version, status, discovered_at, validated_at, re_discover_after)
		VALUES ($1, $2, $3, $4, COALESCE($5, NOW()), $6, $7)
		ON CONFLICT (tenant_id) DO UPDATE SET
			mapping_artifact = EXCLUDED.mapping_artifact,
			artifact_version = EXCLUDED.artifact_version,
			status = EXCLUDED.status,
			validated_at = EXCLUDED.validated_at,
			re_discover_after = EXCLUDED.re_discover_after`,
		schema.TenantID, artifactJSON, schema.ArtifactVersion, schema.Status,
		schema.DiscoveredAt, schema.ValidatedAt, schema.ReDiscoverAfter)
	if err != nil {
		return fmt.Errorf("upsert mapping_artifact: %w", err)
	}
	return nil
}

func (a *MappingArtifactAdapter) GetMappingArtifact(ctx context.Context, tenantID string) (*domain.TenantCatalogSchema, error) {
	var s domain.TenantCatalogSchema
	var artifact []byte
	err := a.client.pool.QueryRow(ctx, `
		SELECT tenant_id, mapping_artifact, artifact_version, status,
			discovered_at, validated_at, re_discover_after
		FROM catalog.tenant_catalog_schema WHERE tenant_id = $1`, tenantID).Scan(
		&s.TenantID, &artifact, &s.ArtifactVersion, &s.Status,
		&s.DiscoveredAt, &s.ValidatedAt, &s.ReDiscoverAfter,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get mapping_artifact: %w", err)
	}
	if err := json.Unmarshal(artifact, &s.MappingArtifact); err != nil {
		return nil, fmt.Errorf("unmarshal mapping_artifact: %w", err)
	}
	return &s, nil
}

func (a *MappingArtifactAdapter) MarkStale(ctx context.Context, tenantID string) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.tenant_catalog_schema SET status = 'stale' WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return fmt.Errorf("mark mapping_artifact stale: %w", err)
	}
	return nil
}

// MarkAllStaleForVertical iterates all artifacts whose mapping_artifact contains
// any field_mapping target referencing the given vertical. Naive scan via
// JSONB containment. Acceptable until we have many tenants.
func (a *MappingArtifactAdapter) MarkAllStaleForVertical(ctx context.Context, vertical string) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.tenant_catalog_schema SET status = 'stale'
		WHERE mapping_artifact @> jsonb_build_object('field_mapping',
			jsonb_build_object('any', jsonb_build_object('vertical', $1::text)))
		   OR mapping_artifact::text ILIKE $2`,
		vertical, "%"+vertical+"%")
	if err != nil {
		return fmt.Errorf("mark all stale for vertical: %w", err)
	}
	return nil
}
