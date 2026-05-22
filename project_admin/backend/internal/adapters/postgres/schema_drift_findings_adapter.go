package postgres

import (
	"context"
	"fmt"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type SchemaDriftFindingsAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewSchemaDriftFindingsAdapter(client *Client, log *logger.Logger) *SchemaDriftFindingsAdapter {
	return &SchemaDriftFindingsAdapter{client: client, log: log}
}

func (a *SchemaDriftFindingsAdapter) Upsert(ctx context.Context, in *ports.DriftFindingUpsert) error {
	if in == nil || in.TenantID == "" || in.ApplyRunID == "" || in.FieldName == "" {
		return fmt.Errorf("drift finding upsert: missing required fields")
	}
	stats := in.Stats
	if len(stats) == 0 {
		stats = []byte("{}")
	}
	action := in.SuggestedAction
	if len(action) == 0 {
		action = []byte("null")
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.schema_drift_findings
			(tenant_id, apply_run_id, field_name, type_guess, stats,
			 decision, confidence, suggested_action, status, classified_at)
		VALUES ($1::uuid, $2, $3, NULLIF($4,''), $5::jsonb,
		        NULLIF($6,''), $7, $8::jsonb, 'classified', NOW())
		ON CONFLICT (tenant_id, apply_run_id, field_name) DO UPDATE
		SET type_guess       = EXCLUDED.type_guess,
		    stats            = EXCLUDED.stats,
		    decision         = EXCLUDED.decision,
		    confidence       = EXCLUDED.confidence,
		    suggested_action = EXCLUDED.suggested_action,
		    status           = CASE
		        WHEN schema_drift_findings.status IN ('applied','dismissed') THEN schema_drift_findings.status
		        ELSE 'classified'
		    END,
		    classified_at    = NOW()
	`, in.TenantID, in.ApplyRunID, in.FieldName, in.TypeGuess, string(stats),
		string(in.Decision), in.Confidence, string(action))
	if err != nil {
		return fmt.Errorf("upsert drift finding: %w", err)
	}
	return nil
}

func (a *SchemaDriftFindingsAdapter) List(ctx context.Context, tenantID string, status domain.SchemaDriftStatus, limit int) ([]*domain.SchemaDriftFinding, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows = a.client.pool
	q := `
		SELECT id::text, tenant_id::text, apply_run_id, field_name,
		       COALESCE(type_guess,''), stats,
		       COALESCE(decision,''), COALESCE(confidence,0),
		       COALESCE(suggested_action::text,'null'),
		       status, created_at, classified_at, decided_at
		FROM catalog.schema_drift_findings
		WHERE tenant_id = $1::uuid
		  AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
		LIMIT $3
	`
	r, err := rows.Query(ctx, q, tenantID, string(status), limit)
	if err != nil {
		return nil, fmt.Errorf("list drift findings: %w", err)
	}
	defer r.Close()
	var out []*domain.SchemaDriftFinding
	for r.Next() {
		f := &domain.SchemaDriftFinding{}
		var statsTxt, actionTxt, decisionTxt string
		if err := r.Scan(&f.ID, &f.TenantID, &f.ApplyRunID, &f.FieldName,
			&f.TypeGuess, &statsTxt, &decisionTxt, &f.Confidence, &actionTxt,
			&f.Status, &f.CreatedAt, &f.ClassifiedAt, &f.DecidedAt); err != nil {
			return nil, fmt.Errorf("scan drift finding: %w", err)
		}
		f.Stats = []byte(statsTxt)
		f.SuggestedAction = []byte(actionTxt)
		f.Decision = domain.SchemaDriftDecision(decisionTxt)
		out = append(out, f)
	}
	return out, r.Err()
}

func (a *SchemaDriftFindingsAdapter) Get(ctx context.Context, id string) (*domain.SchemaDriftFinding, error) {
	f := &domain.SchemaDriftFinding{}
	var statsTxt, actionTxt, decisionTxt string
	err := a.client.pool.QueryRow(ctx, `
		SELECT id::text, tenant_id::text, apply_run_id, field_name,
		       COALESCE(type_guess,''), stats::text,
		       COALESCE(decision,''), COALESCE(confidence,0),
		       COALESCE(suggested_action::text,'null'),
		       status, created_at, classified_at, decided_at
		FROM catalog.schema_drift_findings
		WHERE id::text = $1
	`, id).Scan(&f.ID, &f.TenantID, &f.ApplyRunID, &f.FieldName,
		&f.TypeGuess, &statsTxt, &decisionTxt, &f.Confidence, &actionTxt,
		&f.Status, &f.CreatedAt, &f.ClassifiedAt, &f.DecidedAt)
	if err != nil {
		return nil, fmt.Errorf("get drift finding %s: %w", id, err)
	}
	f.Stats = []byte(statsTxt)
	f.SuggestedAction = []byte(actionTxt)
	f.Decision = domain.SchemaDriftDecision(decisionTxt)
	return f, nil
}

func (a *SchemaDriftFindingsAdapter) MarkApplied(ctx context.Context, id string) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.schema_drift_findings
		SET status = 'applied', decided_at = NOW()
		WHERE id::text = $1 AND status NOT IN ('applied','dismissed')
	`, id)
	if err != nil {
		return fmt.Errorf("mark applied: %w", err)
	}
	return nil
}

func (a *SchemaDriftFindingsAdapter) MarkDismissed(ctx context.Context, id string) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.schema_drift_findings
		SET status = 'dismissed', decided_at = NOW()
		WHERE id::text = $1 AND status NOT IN ('applied','dismissed')
	`, id)
	if err != nil {
		return fmt.Errorf("mark dismissed: %w", err)
	}
	return nil
}
