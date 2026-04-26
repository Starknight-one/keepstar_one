// Package usecases — Discovery orchestrator (M4c).
//
// Wraps MetadataHarvest + AutoMapTier1 + DiscoveryAgent + ValidateArtifact +
// persistence into one Run() call so the handler stays a thin auth layer.
//
// Lives next to the agent itself rather than under handler/ because every
// step is pure usecase work; the handler only does HTTP framing.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
	"keepstar-admin/internal/units"
)

// DiscoveryUseCase is the orchestrator. Holds the pieces the agent needs
// + the persistence target so the handler doesn't have to know about
// MappingArtifactPort directly.
type DiscoveryUseCase struct {
	staging         ports.ShopifyStagingPort
	variants        ports.MasterVariantsPort
	mappingArtifact ports.MappingArtifactPort
	resolver        units.AliasResolver
	agent           *DiscoveryAgent
	log             *logger.Logger
}

func NewDiscoveryUseCase(
	staging ports.ShopifyStagingPort,
	variants ports.MasterVariantsPort,
	mappingArtifact ports.MappingArtifactPort,
	resolver units.AliasResolver,
	agent *DiscoveryAgent,
	log *logger.Logger,
) *DiscoveryUseCase {
	return &DiscoveryUseCase{
		staging:         staging,
		variants:        variants,
		mappingArtifact: mappingArtifact,
		resolver:        resolver,
		agent:           agent,
		log:             log,
	}
}

// DiscoveryRunResult is the wire-shape returned to the dev-trigger handler.
// Carries the validation report and the transcript (truncated by Discover)
// so the operator can eyeball what the agent did without poking the DB.
type DiscoveryRunResult struct {
	TenantID         string                       `json:"tenantId"`
	Status           domain.MappingArtifactStatus `json:"status"`
	StopReason       string                       `json:"stopReason"`
	TurnsUsed        int                          `json:"turnsUsed"`
	InputTokens      int                          `json:"inputTokens"`
	OutputTokens     int                          `json:"outputTokens"`
	DurationMs       int64                        `json:"durationMs"`
	FieldMappingSize int                          `json:"fieldMappingSize"`
	CategorySize     int                          `json:"categorySize"`
	TemplateCount    int                          `json:"templateCount"`
	Validation       *ValidationReport            `json:"validation,omitempty"`
	Transcript       []TranscriptEntry            `json:"transcript,omitempty"`
}

// Run executes the full discovery pipeline. Idempotent: re-runs replace the
// previous artifact (artifact_version bumps via the adapter). Caller is
// responsible for ensuring staging has been populated (DumpToStaging in 4a).
func (uc *DiscoveryUseCase) Run(ctx context.Context, tenantID string) (*DiscoveryRunResult, error) {
	if tenantID == "" {
		return nil, errors.New("discovery run: tenantID required")
	}
	if uc.agent == nil {
		return nil, errors.New("discovery run: agent not configured")
	}

	start := time.Now()
	out := &DiscoveryRunResult{TenantID: tenantID}

	// Step 1: meta-report from staging.
	harvest := NewMetadataHarvest(uc.staging, uc.log)
	report, err := harvest.Run(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("meta harvest: %w", err)
	}
	if report.TotalProducts == 0 {
		return nil, errors.New("staging is empty — run dump-to-staging first")
	}

	// Step 2: Tier 1 auto-mapping (deterministic).
	tier1 := AutoMapTier1(report, uc.resolver)

	// Step 3: agent loop.
	agentRes, err := uc.agent.Discover(ctx, tenantID, report, tier1)
	if err != nil {
		return nil, fmt.Errorf("agent: %w", err)
	}
	out.Status = agentRes.Status
	out.StopReason = agentRes.StopReason
	out.TurnsUsed = agentRes.TurnsUsed
	out.InputTokens = agentRes.InputTokens
	out.OutputTokens = agentRes.OutputTokens
	out.Transcript = agentRes.Transcript

	// If the agent didn't commit, persist a needs_human_review marker so
	// the curator UI can pick it up. Use the partial builder snapshot if
	// available, else write an empty-but-statused record.
	if agentRes.Artifact == nil {
		// Salvage everything the agent already proposed via propose_*. A
		// budget overrun shouldn't throw away 10 propose_field_mappings.
		var marker *domain.MappingArtifact
		if agentRes.PartialBuilder != nil {
			marker = agentRes.PartialBuilder.Build(
				"PARTIAL — agent stopped before commit (reason: "+agentRes.StopReason+")",
				[]string{"gtin", "vendor+sku", "vendor+title+axes", "embedding"},
				"master_with_variants",
			)
			marker.Status = domain.MappingArtifactStatusNeedsHumanReview
		} else {
			marker = &domain.MappingArtifact{
				Version:      1,
				Status:       domain.MappingArtifactStatusNeedsHumanReview,
				AgentNotes:   "agent did not commit; reason: " + agentRes.StopReason,
				FieldMapping: tier1.FieldMapping,
			}
		}
		out.FieldMappingSize = len(marker.FieldMapping)
		out.CategorySize = len(marker.CategoryMapping)
		out.TemplateCount = len(marker.MasterTemplates)
		if uc.mappingArtifact != nil {
			schema := &domain.TenantCatalogSchema{
				TenantID:        tenantID,
				MappingArtifact: *marker,
				Status:          marker.Status,
				DiscoveredAt:    time.Now().UTC(),
			}
			if err := uc.mappingArtifact.SaveMappingArtifact(ctx, schema); err != nil {
				uc.log.Error("discovery_save_marker_failed", "tenant", tenantID, "error", err)
			}
		}
		out.DurationMs = time.Since(start).Milliseconds()
		return out, nil
	}

	// Step 4: validation pass.
	validation, err := ValidateArtifact(ctx, tenantID, agentRes.Artifact, uc.staging, uc.resolver, uc.log)
	if err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	out.Validation = validation

	// Apply the validation's recommended status (overrides what the agent
	// claimed). E.g. agent said "active" but validation found <80% coverage.
	agentRes.Artifact.Status = validation.RecommendedStatus
	out.Status = validation.RecommendedStatus

	// Step 5: persist.
	if uc.mappingArtifact != nil {
		now := time.Now().UTC()
		schema := &domain.TenantCatalogSchema{
			TenantID:        tenantID,
			MappingArtifact: *agentRes.Artifact,
			Status:          validation.RecommendedStatus,
			DiscoveredAt:    now,
			ValidatedAt:     &now,
		}
		if err := uc.mappingArtifact.SaveMappingArtifact(ctx, schema); err != nil {
			return nil, fmt.Errorf("save artifact: %w", err)
		}
	}

	out.FieldMappingSize = len(agentRes.Artifact.FieldMapping)
	out.CategorySize = len(agentRes.Artifact.CategoryMapping)
	out.TemplateCount = len(agentRes.Artifact.MasterTemplates)
	out.DurationMs = time.Since(start).Milliseconds()
	uc.log.Info("discovery_completed",
		"tenant", tenantID,
		"status", string(out.Status),
		"turns", out.TurnsUsed,
		"tokens", out.InputTokens+out.OutputTokens,
		"field_mappings", out.FieldMappingSize,
	)
	return out, nil
}
