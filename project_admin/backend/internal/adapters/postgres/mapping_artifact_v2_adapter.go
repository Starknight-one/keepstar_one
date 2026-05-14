// Package postgres — MappingArtifactV2Adapter writes/reads MappingArtifactV2
// into the same catalog.tenant_catalog_schema row used by the legacy port.
// The mapping_artifact JSONB column is overloaded: legacy MappingArtifact
// has version=1, the new shape has version=2. apply_v2 only deserializes
// version=2; legacy merge_apply only handles version=1.
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

type MappingArtifactV2Adapter struct {
	client *Client
	log    *logger.Logger
}

func NewMappingArtifactV2Adapter(client *Client, log *logger.Logger) *MappingArtifactV2Adapter {
	return &MappingArtifactV2Adapter{client: client, log: log}
}

var _ ports.MappingArtifactV2Port = (*MappingArtifactV2Adapter)(nil)

func (a *MappingArtifactV2Adapter) Save(ctx context.Context, tenantID string, artifact *domain.MappingArtifactV2) error {
	if tenantID == "" {
		return fmt.Errorf("artifact save: empty tenant_id")
	}
	if artifact == nil {
		return fmt.Errorf("artifact save: nil artifact")
	}
	artifact.Version = 2 // force — the table column is shared with v1
	body, err := json.Marshal(artifact)
	if err != nil {
		return fmt.Errorf("artifact save marshal: %w", err)
	}
	// Bump artifact_version, stamp timestamps, set status validated.
	// ON CONFLICT (tenant_id) DO UPDATE for upsert semantics.
	_, err = a.client.pool.Exec(ctx, `
		INSERT INTO catalog.tenant_catalog_schema
			(tenant_id, status, artifact_version, mapping_artifact, discovered_at, validated_at)
		VALUES ($1, 'validated', 1, $2::jsonb, NOW(), NOW())
		ON CONFLICT (tenant_id) DO UPDATE
		SET status           = 'validated',
		    artifact_version = tenant_catalog_schema.artifact_version + 1,
		    mapping_artifact = EXCLUDED.mapping_artifact,
		    discovered_at    = NOW(),
		    validated_at     = NOW()
	`, tenantID, body)
	if err != nil {
		return fmt.Errorf("artifact save exec: %w", err)
	}
	return nil
}

func (a *MappingArtifactV2Adapter) Get(ctx context.Context, tenantID string) (*domain.MappingArtifactV2, *ports.MappingArtifactMeta, error) {
	if tenantID == "" {
		return nil, nil, fmt.Errorf("artifact get: empty tenant_id")
	}
	var body []byte
	meta := &ports.MappingArtifactMeta{}
	err := a.client.pool.QueryRow(ctx, `
		SELECT mapping_artifact, artifact_version, status, discovered_at, validated_at
		FROM catalog.tenant_catalog_schema
		WHERE tenant_id = $1
	`, tenantID).Scan(&body, &meta.ArtifactVersion, &meta.Status, &meta.DiscoveredAt, &meta.ValidatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("artifact get query: %w", err)
	}
	if len(body) == 0 || string(body) == "null" {
		return nil, meta, nil
	}
	// Peek at version field to decide if this is v2.
	var probe struct {
		Version int `json:"version"`
	}
	_ = json.Unmarshal(body, &probe)
	if probe.Version != 2 {
		// Legacy v1 artifact in this row — return nil body so callers
		// know to treat it as "no v2 artifact yet".
		return nil, meta, nil
	}
	var art domain.MappingArtifactV2
	if err := json.Unmarshal(body, &art); err != nil {
		return nil, meta, fmt.Errorf("artifact unmarshal v2: %w", err)
	}
	return &art, meta, nil
}

func (a *MappingArtifactV2Adapter) MarkStale(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("artifact mark stale: empty tenant_id")
	}
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.tenant_catalog_schema
		SET status = 'stale'
		WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		return fmt.Errorf("artifact mark stale: %w", err)
	}
	return nil
}
