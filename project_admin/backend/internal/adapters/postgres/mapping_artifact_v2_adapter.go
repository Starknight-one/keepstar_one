// Package postgres — MappingArtifactV2Adapter writes/reads mapping artifacts
// into catalog.tenant_catalog_schema.mapping_artifact (JSONB). The column is
// overloaded across formats; the version field inside the JSON disambiguates:
//
//	version=1 → legacy MappingArtifact (merge_apply); ignored by this adapter
//	version=2 → MappingArtifactV2; lifted to v3 on read (one-branch)
//	version=3 → MappingArtifactV3; the only format Save() emits
//
// Apply_v2 and discovery_v2 see only MappingArtifactV3 in memory.
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

// Save writes a v3 artifact, force-stamping Version=3 so a v2 caller can't
// accidentally persist with the wrong tag. The row's artifact_version
// counter is bumped each time, monotonic per tenant.
func (a *MappingArtifactV2Adapter) Save(ctx context.Context, tenantID string, artifact *domain.MappingArtifactV3) error {
	if tenantID == "" {
		return fmt.Errorf("artifact save: empty tenant_id")
	}
	if artifact == nil {
		return fmt.Errorf("artifact save: nil artifact")
	}
	artifact.Version = 3
	body, err := json.Marshal(artifact)
	if err != nil {
		return fmt.Errorf("artifact save marshal: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		INSERT INTO catalog.tenant_catalog_schema
			(tenant_id, status, artifact_version, mapping_artifact, discovered_at, validated_at)
		VALUES ($1, 'active', 1, $2::jsonb, NOW(), NOW())
		ON CONFLICT (tenant_id) DO UPDATE
		SET status           = 'active',
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

// Get reads the current artifact and normalizes the result to v3 (or nil
// when the row is a legacy v1 artifact / empty / missing).
func (a *MappingArtifactV2Adapter) Get(ctx context.Context, tenantID string) (*domain.MappingArtifactV3, *ports.MappingArtifactMeta, error) {
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

	switch domain.PeekArtifactVersion(body) {
	case 3:
		var art domain.MappingArtifactV3
		if err := json.Unmarshal(body, &art); err != nil {
			return nil, meta, fmt.Errorf("artifact unmarshal v3: %w", err)
		}
		return &art, meta, nil
	case 2:
		var v2 domain.MappingArtifactV2
		if err := json.Unmarshal(body, &v2); err != nil {
			return nil, meta, fmt.Errorf("artifact unmarshal v2: %w", err)
		}
		return domain.LiftV2ToV3(&v2), meta, nil
	default:
		// Legacy v1 (or unrecognized) — treat as "no v3 artifact".
		return nil, meta, nil
	}
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
