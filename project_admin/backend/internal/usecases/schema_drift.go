// Package usecases — schema drift detection. After each apply for a
// tenant, we publish a job to the broker; a worker pulls the job, looks
// at what fields are in the tenant's inbox vs what the current artifact
// maps, and asks the LLM to classify each unmapped field as typo /
// alias / genuinely new. Findings land in catalog.schema_drift_findings;
// the admin "Drift" tab lets the owner accept a suggestion (which
// triggers patch-discovery via B1) or dismiss it.
package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"keepstar-admin/internal/adapters/anthropic"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

const driftTopic = "drift_classify"

// SchemaDriftUseCase wires the producer + worker halves of drift
// detection. PublishJob is called inline by ApplyForTenant (fast — just
// publishes to broker). Classify is called by the worker (slow — LLM
// call).
type SchemaDriftUseCase struct {
	inbox     ports.InboxPort
	artifact  ports.MappingArtifactV2Port
	findings  ports.SchemaDriftFindingsPort
	broker    ports.BrokerPort
	llm       *anthropic.DriftClassifierClient
	actionLog ports.TenantActionLogPort
	log       *logger.Logger
}

func NewSchemaDrift(
	inbox ports.InboxPort,
	artifact ports.MappingArtifactV2Port,
	findings ports.SchemaDriftFindingsPort,
	broker ports.BrokerPort,
	llm *anthropic.DriftClassifierClient,
	actionLog ports.TenantActionLogPort,
	log *logger.Logger,
) *SchemaDriftUseCase {
	return &SchemaDriftUseCase{
		inbox: inbox, artifact: artifact, findings: findings,
		broker: broker, llm: llm, actionLog: actionLog, log: log,
	}
}

// DriftJob is the payload published to and consumed from the broker.
type DriftJob struct {
	TenantID   string `json:"tenant_id"`
	ApplyRunID string `json:"apply_run_id"`
}

// Topic returns the broker topic / stream name used by both producer
// and worker.
func DriftTopic() string { return driftTopic }

// PublishJob is the producer side. Called by ApplyForTenant after a
// successful apply. Best-effort — if broker is unreachable we log and
// return error; caller (apply_v2) logs a warning and continues so the
// apply itself doesn't fail.
func (uc *SchemaDriftUseCase) PublishJob(ctx context.Context, tenantID, applyRunID string) error {
	if uc.broker == nil {
		return nil // broker disabled (no REDIS_URL) — graceful no-op
	}
	job := DriftJob{TenantID: tenantID, ApplyRunID: applyRunID}
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal drift job: %w", err)
	}
	if err := uc.broker.Publish(ctx, driftTopic, payload); err != nil {
		return fmt.Errorf("publish drift job: %w", err)
	}
	return nil
}

// Classify is the worker side. Loads the tenant's artifact + inbox
// fields, computes unmapped, gathers stats, calls LLM, writes findings.
func (uc *SchemaDriftUseCase) Classify(ctx context.Context, tenantID, applyRunID string) error {
	art, _, err := uc.artifact.Get(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("load artifact: %w", err)
	}
	if art == nil {
		// Tenant never went through first_install — nothing to drift against.
		uc.logDriftAction(ctx, tenantID, applyRunID, "drift_no_artifact", "ok", nil)
		return nil
	}

	inboxFields, err := uc.inbox.ListFields(ctx, tenantID, 200)
	if err != nil {
		return fmt.Errorf("list inbox fields: %w", err)
	}

	mapped := mappedKeySet(art)
	unmapped := make([]string, 0, len(inboxFields))
	for _, f := range inboxFields {
		if f == "" {
			continue
		}
		if mapped[f] {
			continue
		}
		unmapped = append(unmapped, f)
	}

	if len(unmapped) == 0 {
		uc.logDriftAction(ctx, tenantID, applyRunID, "drift_no_findings", "ok",
			map[string]any{"inbox_field_count": len(inboxFields)})
		return nil
	}

	// Cap evidence at 20 fields per run — limits prompt size and cost.
	// If a tenant somehow has more, classify the first 20 and leave the
	// rest for the next apply run.
	if len(unmapped) > 20 {
		unmapped = unmapped[:20]
	}

	driftFields, statsByField, err := uc.gatherEvidence(ctx, tenantID, unmapped)
	if err != nil {
		return fmt.Errorf("gather evidence: %w", err)
	}
	if len(driftFields) == 0 {
		uc.logDriftAction(ctx, tenantID, applyRunID, "drift_no_findings", "ok",
			map[string]any{"reason": "all_evidence_empty"})
		return nil
	}

	summary := buildArtifactSummary(art)
	result, err := uc.llm.Classify(ctx, summary, driftFields)
	if err != nil {
		return fmt.Errorf("llm classify: %w", err)
	}

	written := 0
	for _, c := range result.Classifications {
		statsJSON := statsByField[c.Field]
		if len(statsJSON) == 0 {
			statsJSON = []byte("{}")
		}
		upsert := &ports.DriftFindingUpsert{
			TenantID:        tenantID,
			ApplyRunID:      applyRunID,
			FieldName:       c.Field,
			TypeGuess:       typeGuessForField(driftFields, c.Field),
			Stats:           statsJSON,
			Decision:        domain.SchemaDriftDecision(c.Decision),
			Confidence:      c.Confidence,
			SuggestedAction: c.SuggestedAction,
		}
		if err := uc.findings.Upsert(ctx, upsert); err != nil {
			uc.log.Warn("drift_findings_upsert_failed",
				"tenant", tenantID, "field", c.Field, "error", err)
			continue
		}
		written++
	}

	uc.logDriftAction(ctx, tenantID, applyRunID, "drift_classified", "ok",
		map[string]any{
			"unmapped_count":   len(unmapped),
			"findings_written": written,
			"input_tokens":     result.InputTokens,
			"output_tokens":    result.OutputTokens,
		})
	return nil
}

// gatherEvidence pulls FieldStats per unmapped field and assembles the
// DriftField evidence pack the LLM sees. Returns the stats blob alongside
// (keyed by field name) so we can persist it on findings rows.
func (uc *SchemaDriftUseCase) gatherEvidence(ctx context.Context, tenantID string, fields []string) ([]anthropic.DriftField, map[string]json.RawMessage, error) {
	out := make([]anthropic.DriftField, 0, len(fields))
	stats := make(map[string]json.RawMessage, len(fields))
	for _, f := range fields {
		s, err := uc.inbox.FieldStats(ctx, tenantID, f, 20)
		if err != nil {
			uc.log.Warn("drift_fieldstats_failed", "tenant", tenantID, "field", f, "error", err)
			continue
		}
		if s == nil || s.NonNullCount == 0 {
			continue
		}
		df := anthropic.DriftField{
			Name:          f,
			TypeGuess:     GuessFieldType(s),
			NonNullCount:  s.NonNullCount,
			TotalCount:    s.NonNullCount, // FieldStats currently returns non-null as total proxy
			DistinctCount: s.DistinctCount,
			MinNumeric:    s.MinNumeric,
			MaxNumeric:    s.MaxNumeric,
			MinLength:     s.MinLength,
			MaxLength:     s.MaxLength,
			SampleValues:  s.SampleValues,
		}
		out = append(out, df)
		if b, err := json.Marshal(s); err == nil {
			stats[f] = b
		}
	}
	return out, stats, nil
}

func (uc *SchemaDriftUseCase) logDriftAction(ctx context.Context, tenantID, runID, action, status string, payload map[string]any) {
	if uc.actionLog == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["apply_run_id"] = runID
	body, _ := json.Marshal(payload)
	_ = uc.actionLog.Log(ctx, &ports.TenantActionLogEntry{
		TenantID: tenantID,
		Action:   action,
		Status:   status,
		Payload:  body,
	})
}

// mappedKeySet collects all .From keys across all branches of the
// artifact, plus the classifying_field itself. Used to compute
// inbox-fields minus this set = "unmapped".
func mappedKeySet(art *domain.MappingArtifactV3) map[string]bool {
	out := map[string]bool{}
	if art == nil {
		return out
	}
	if art.ClassifyingField != "" {
		out[art.ClassifyingField] = true
	}
	for _, b := range art.Branches {
		for _, r := range b.FieldMap {
			if r.From != "" {
				out[r.From] = true
			}
		}
	}
	return out
}

// buildArtifactSummary projects MappingArtifactV3 into the slim view the
// drift classifier sees. We strip transforms / defaults / notes to keep
// the prompt cheap.
func buildArtifactSummary(art *domain.MappingArtifactV3) *anthropic.DriftArtifactSummary {
	out := &anthropic.DriftArtifactSummary{
		ClassifyingField: art.ClassifyingField,
		Branches:         make([]anthropic.DriftArtifactBranch, 0, len(art.Branches)),
		ClassifyRules:    make([]anthropic.DriftClassifyRuleView, 0, len(art.ClassifyRules)),
	}
	for _, b := range art.Branches {
		bv := anthropic.DriftArtifactBranch{
			Vertical: b.Vertical,
			FieldMap: make([]anthropic.DriftFieldMappingView, 0, len(b.FieldMap)),
		}
		for _, r := range b.FieldMap {
			bv.FieldMap = append(bv.FieldMap, anthropic.DriftFieldMappingView{
				From: r.From,
				To:   r.To,
			})
		}
		out.Branches = append(out.Branches, bv)
	}
	for _, r := range art.ClassifyRules {
		out.ClassifyRules = append(out.ClassifyRules, anthropic.DriftClassifyRuleView{
			When:         r.When,
			ThenVertical: r.ThenVertical,
		})
	}
	return out
}

func typeGuessForField(fields []anthropic.DriftField, name string) string {
	for _, f := range fields {
		if strings.EqualFold(f.Name, name) {
			return f.TypeGuess
		}
	}
	return ""
}
