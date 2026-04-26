// Package usecases — Mapping-artifact validation pass (M4c spec §4.1 step 7).
//
// Runs AFTER the discovery agent commits and BEFORE the harvester (4d) does
// real writes. Picks 10-20 random staging records, applies the artifact's
// field_mapping rules, and tallies coverage / parser failures. If coverage
// stays ≥80% and there are no fatal errors, the artifact gets status=active.
// Otherwise it lands as status=needs_human_review for the curator (M11).
//
// We deliberately don't run match cascade or write any DB rows here — that
// belongs in 4d. Validation is read-only and additive: we score the artifact,
// not the catalog.
package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
	"keepstar-admin/internal/units"
)

const (
	validateMaxSamples         = 20
	validateMinCoveragePercent = 80
)

// ValidationReport is the structured score returned by ValidateArtifact.
// Persisted into catalog.tenant_catalog_schema.validation_report so the
// curator UI can display "why was this flagged for review".
type ValidationReport struct {
	SampledRecords      int                   `json:"sampledRecords"`
	CoveragePercent     float64               `json:"coveragePercent"`
	ParserFailures      int                   `json:"parserFailures"`
	ParserAmbiguous     int                   `json:"parserAmbiguous"`
	UnmappedFieldCounts map[string]int        `json:"unmappedFieldCounts,omitempty"`
	FatalErrors         []string              `json:"fatalErrors,omitempty"`
	RecommendedStatus   domain.MappingArtifactStatus `json:"recommendedStatus"`
	GeneratedAt         time.Time             `json:"generatedAt"`
}

// ValidateArtifact reads up to validateMaxSamples products from staging and
// scores how well the artifact handles them. Pure read; no writes anywhere.
func ValidateArtifact(
	ctx context.Context,
	tenantID string,
	artifact *domain.MappingArtifact,
	staging ports.ShopifyStagingPort,
	resolver units.AliasResolver,
	log *logger.Logger,
) (*ValidationReport, error) {
	if artifact == nil {
		return nil, fmt.Errorf("validate: artifact is nil")
	}
	report := &ValidationReport{
		UnmappedFieldCounts: make(map[string]int),
		GeneratedAt:         time.Now().UTC(),
	}

	// Iterate once, score each sample. Stop once we have validateMaxSamples.
	mappedFieldHits, totalFieldOccurrences := 0, 0
	iterErr := staging.IterateProducts(ctx, tenantID, func(_ string, payload json.RawMessage, _ time.Time) error {
		if report.SampledRecords >= validateMaxSamples {
			return errStopIteration
		}
		report.SampledRecords++

		var product map[string]json.RawMessage
		if err := json.Unmarshal(payload, &product); err != nil {
			return nil // skip unparseable; staging shouldn't have these
		}

		// Walk product fields, check against artifact.FieldMapping.
		walkValidate("product", "", product, artifact, resolver, &mappedFieldHits, &totalFieldOccurrences, report)

		// Variants
		if rawV, ok := product["_v2_variants"]; ok {
			var variants []json.RawMessage
			if err := json.Unmarshal(rawV, &variants); err == nil {
				for _, vRaw := range variants {
					var variant map[string]json.RawMessage
					if err := json.Unmarshal(vRaw, &variant); err == nil {
						walkValidate("variant", "", variant, artifact, resolver, &mappedFieldHits, &totalFieldOccurrences, report)
					}
				}
			}
		}
		// Metafields
		if rawMfs, ok := product["_v2_metafields"]; ok {
			var mfs []json.RawMessage
			if err := json.Unmarshal(rawMfs, &mfs); err == nil {
				for _, mfRaw := range mfs {
					var mf map[string]json.RawMessage
					if err := json.Unmarshal(mfRaw, &mf); err == nil {
						walkValidate("metafield", "", mf, artifact, resolver, &mappedFieldHits, &totalFieldOccurrences, report)
					}
				}
			}
		}
		return nil
	})
	if iterErr != nil && !isStopIteration(iterErr) {
		return nil, fmt.Errorf("validate iterate: %w", iterErr)
	}

	if totalFieldOccurrences == 0 {
		// Empty staging — can't meaningfully validate. Mark for review so
		// the curator notices.
		report.RecommendedStatus = domain.MappingArtifactStatusNeedsHumanReview
		report.FatalErrors = append(report.FatalErrors, "no products in staging — cannot validate")
		return report, nil
	}
	report.CoveragePercent = 100.0 * float64(mappedFieldHits) / float64(totalFieldOccurrences)

	// Decide status. Spec §4.1 step 7: ≥80% coverage AND no fatal errors → active.
	if len(report.FatalErrors) == 0 && report.CoveragePercent >= validateMinCoveragePercent {
		report.RecommendedStatus = domain.MappingArtifactStatusActive
	} else {
		report.RecommendedStatus = domain.MappingArtifactStatusNeedsHumanReview
	}

	if log != nil {
		log.Info("artifact_validation_done",
			"tenant", tenantID,
			"samples", report.SampledRecords,
			"coverage_pct", report.CoveragePercent,
			"parser_failed", report.ParserFailures,
			"parser_ambiguous", report.ParserAmbiguous,
			"status", string(report.RecommendedStatus),
		)
	}
	return report, nil
}

// walkValidate iterates a JSON object, counting each leaf as either "mapped"
// (artifact.FieldMapping has an entry for it) or "unmapped" (incremented in
// report.UnmappedFieldCounts). Numeric fields with units transforms are
// re-parsed via the units module to count parser failures/ambiguous.
func walkValidate(
	ownerType, prefix string,
	obj map[string]json.RawMessage,
	artifact *domain.MappingArtifact,
	resolver units.AliasResolver,
	mappedHits *int,
	totalOccurrences *int,
	report *ValidationReport,
) {
	for key, raw := range obj {
		if strings.HasPrefix(key, "_v2_") || key == "__parentId" {
			continue
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		validateValue(ownerType, path, raw, artifact, resolver, mappedHits, totalOccurrences, report)
	}
}

func validateValue(
	ownerType, path string,
	raw json.RawMessage,
	artifact *domain.MappingArtifact,
	resolver units.AliasResolver,
	mappedHits *int,
	totalOccurrences *int,
	report *ValidationReport,
) {
	r := strings.TrimSpace(string(raw))
	if r == "" || r == "null" {
		return
	}
	// Recurse into nested objects/arrays — the artifact's keys mirror the
	// field paths from the meta-report.
	if r[0] == '{' {
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(raw, &nested); err == nil {
			walkValidate(ownerType, path, nested, artifact, resolver, mappedHits, totalOccurrences, report)
		}
		return
	}
	if r[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err == nil {
			for _, e := range arr {
				validateValue(ownerType, path+".[]", e, artifact, resolver, mappedHits, totalOccurrences, report)
			}
		}
		return
	}

	*totalOccurrences++

	target, ok := artifact.FieldMapping[path]
	if !ok {
		report.UnmappedFieldCounts[path]++
		return
	}
	*mappedHits++

	// Validate transforms — only units.* matters for now (others are pure
	// string ops that don't really fail).
	if strings.HasPrefix(target.Transform, "units.") {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			res := units.Parse(s, units.ParseOpts{Resolver: resolver})
			switch res.Status {
			case units.ParseStatusOK:
				// nothing
			case units.ParseStatusAmbiguous:
				report.ParserAmbiguous++
			case units.ParseStatusFailed, units.ParseStatusDualMismatch:
				report.ParserFailures++
			}
		}
	}
}

// isStopIteration mirrors errStopIteration from discovery_tools.go in a
// way that doesn't make this file depend on it transitively. (Keeping the
// dependency direction discovery_agent → tools, not tools → validate.)
func isStopIteration(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "stop iteration"
}
