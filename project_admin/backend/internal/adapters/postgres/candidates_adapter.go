// Package postgres — staging tables for promotion workflow + junk detection.
// Spec: docs/New features/admin_catalog_design_2026-04-23.md §3.5, §3.6, §4.5, §4.7.
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

type CandidatesAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewCandidatesAdapter(client *Client, log *logger.Logger) *CandidatesAdapter {
	return &CandidatesAdapter{client: client, log: log}
}

var _ ports.CandidatesPort = (*CandidatesAdapter)(nil)

// --- Attribute candidates ---

// UpsertAttributeCandidate inserts new or appends to sample_values + bumps
// seen_in_tenants. UNIQUE on (key, vertical). Sample value is appended only
// if not already in the array (cap to 50 to avoid runaway).
func (a *CandidatesAdapter) UpsertAttributeCandidate(ctx context.Context, key, vertical, sampleValue, agentMeta string) error {
	if key == "" || vertical == "" {
		return errors.New("attribute_candidates: key + vertical required")
	}
	samplesJSON, _ := json.Marshal([]string{sampleValue})
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.master_attribute_candidates (key, vertical, sample_values, agent_meta)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key, vertical) DO UPDATE SET
			seen_in_tenants = catalog.master_attribute_candidates.seen_in_tenants + 1,
			sample_values = (
				SELECT jsonb_agg(DISTINCT v) FROM jsonb_array_elements_text(
					catalog.master_attribute_candidates.sample_values || EXCLUDED.sample_values
				) v LIMIT 50
			),
			agent_meta = COALESCE(NULLIF(EXCLUDED.agent_meta, ''), catalog.master_attribute_candidates.agent_meta),
			updated_at = NOW()`,
		key, vertical, samplesJSON, agentMeta)
	if err != nil {
		return fmt.Errorf("upsert attribute_candidate: %w", err)
	}
	return nil
}

func (a *CandidatesAdapter) ListAttributeCandidates(ctx context.Context, vertical string, status domain.CandidateStatus, minSeen int) ([]domain.AttributeCandidate, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, key, vertical, seen_in_tenants, sample_values, proposed_type,
			COALESCE(agent_meta, ''), status,
			COALESCE(merged_into_key, ''), COALESCE(promoted_to_column, ''),
			created_at, updated_at
		FROM catalog.master_attribute_candidates
		WHERE ($1 = '' OR vertical = $1)
			AND ($2 = '' OR status = $2)
			AND seen_in_tenants >= $3
		ORDER BY seen_in_tenants DESC, key`,
		vertical, string(status), minSeen)
	if err != nil {
		return nil, fmt.Errorf("list attribute_candidates: %w", err)
	}
	defer rows.Close()
	var out []domain.AttributeCandidate
	for rows.Next() {
		var c domain.AttributeCandidate
		var samples []byte
		var proposedType *string
		if err := rows.Scan(&c.ID, &c.Key, &c.Vertical, &c.SeenInTenants, &samples, &proposedType,
			&c.AgentMeta, &c.Status, &c.MergedIntoKey, &c.PromotedToColumn,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(samples, &c.SampleValues)
		if proposedType != nil {
			c.ProposedType = *proposedType
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *CandidatesAdapter) MarkAttributeCandidatePromoted(ctx context.Context, id, promotedToColumn string) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.master_attribute_candidates
		SET status = 'promoted', promoted_to_column = $1, updated_at = NOW()
		WHERE id = $2`, promotedToColumn, id)
	if err != nil {
		return fmt.Errorf("mark candidate promoted: %w", err)
	}
	return nil
}

// --- Category candidates ---

func (a *CandidatesAdapter) UpsertCategoryCandidate(ctx context.Context, name, proposedParent, vertical string) error {
	if name == "" || vertical == "" {
		return errors.New("category_candidates: name + vertical required")
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.master_category_candidates (name, proposed_parent, vertical)
		VALUES ($1, $2, $3)
		ON CONFLICT (name, vertical) DO UPDATE SET
			seen_in_tenants = catalog.master_category_candidates.seen_in_tenants + 1,
			proposed_parent = COALESCE(NULLIF(EXCLUDED.proposed_parent, ''), catalog.master_category_candidates.proposed_parent)`,
		name, proposedParent, vertical)
	if err != nil {
		return fmt.Errorf("upsert category_candidate: %w", err)
	}
	return nil
}

func (a *CandidatesAdapter) ListCategoryCandidates(ctx context.Context, vertical string, status domain.CandidateStatus, minSeen int) ([]domain.CategoryCandidate, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, name, COALESCE(proposed_parent, ''), seen_in_tenants, vertical, status,
			COALESCE(promoted_to_id::text, ''), created_at
		FROM catalog.master_category_candidates
		WHERE ($1 = '' OR vertical = $1)
			AND ($2 = '' OR status = $2)
			AND seen_in_tenants >= $3
		ORDER BY seen_in_tenants DESC, name`,
		vertical, string(status), minSeen)
	if err != nil {
		return nil, fmt.Errorf("list category_candidates: %w", err)
	}
	defer rows.Close()
	var out []domain.CategoryCandidate
	for rows.Next() {
		var c domain.CategoryCandidate
		if err := rows.Scan(&c.ID, &c.Name, &c.ProposedParent, &c.SeenInTenants, &c.Vertical, &c.Status,
			&c.PromotedToID, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *CandidatesAdapter) MarkCategoryCandidatePromoted(ctx context.Context, id, promotedToID string) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.master_category_candidates
		SET status = 'promoted', promoted_to_id = $1
		WHERE id = $2`, promotedToID, id)
	if err != nil {
		return fmt.Errorf("mark category_candidate promoted: %w", err)
	}
	return nil
}

// --- Junk candidates ---

func (a *CandidatesAdapter) InsertJunkCandidate(ctx context.Context, jc *domain.JunkCandidate) error {
	if jc.TenantID == "" || jc.ListingID == "" {
		return errors.New("junk_candidates: tenant_id + listing_id required")
	}
	if jc.DetectedReason == nil {
		jc.DetectedReason = map[string]interface{}{}
	}
	reasonJSON, _ := json.Marshal(jc.DetectedReason)
	if jc.Classification == "" {
		jc.Classification = domain.JunkClassificationPending
	}
	var masterVariantID interface{}
	if jc.MasterVariantID != "" {
		masterVariantID = jc.MasterVariantID
	}
	// Idempotent on (tenant_id, listing_id) — no-op if already pending.
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.tenant_variant_candidates_junk
			(tenant_id, listing_id, master_variant_id, detected_reason, classification)
		SELECT $1, $2, $3, $4, $5
		WHERE NOT EXISTS (
			SELECT 1 FROM catalog.tenant_variant_candidates_junk
			WHERE tenant_id = $1 AND listing_id = $2
		)`,
		jc.TenantID, jc.ListingID, masterVariantID, reasonJSON, jc.Classification)
	if err != nil {
		return fmt.Errorf("insert junk_candidate: %w", err)
	}
	return nil
}

func (a *CandidatesAdapter) ListJunkCandidates(ctx context.Context, tenantID string, status domain.JunkClassification) ([]domain.JunkCandidate, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, tenant_id, listing_id, COALESCE(master_variant_id::text, ''),
			detected_reason, classification, classified_at, COALESCE(classified_by, ''), created_at
		FROM catalog.tenant_variant_candidates_junk
		WHERE tenant_id = $1 AND ($2 = '' OR classification = $2)
		ORDER BY created_at DESC`,
		tenantID, string(status))
	if err != nil {
		return nil, fmt.Errorf("list junk_candidates: %w", err)
	}
	defer rows.Close()
	var out []domain.JunkCandidate
	for rows.Next() {
		var jc domain.JunkCandidate
		var reason []byte
		if err := rows.Scan(&jc.ID, &jc.TenantID, &jc.ListingID, &jc.MasterVariantID,
			&reason, &jc.Classification, &jc.ClassifiedAt, &jc.ClassifiedBy, &jc.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(reason, &jc.DetectedReason)
		out = append(out, jc)
	}
	return out, rows.Err()
}

func (a *CandidatesAdapter) ClassifyJunkCandidate(ctx context.Context, id string, classification domain.JunkClassification, classifiedBy string) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.tenant_variant_candidates_junk
		SET classification = $1, classified_at = NOW(), classified_by = $2
		WHERE id = $3`, classification, classifiedBy, id)
	if err != nil {
		return fmt.Errorf("classify junk_candidate: %w", err)
	}
	return nil
}

func (a *CandidatesAdapter) CountJunkPending(ctx context.Context, tenantID string) (int, error) {
	var n int
	err := a.client.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM catalog.tenant_variant_candidates_junk
		WHERE tenant_id = $1 AND classification = 'pending'`, tenantID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("count junk pending: %w", err)
	}
	return n, nil
}
