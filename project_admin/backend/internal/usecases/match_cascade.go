// Package usecases — Match cascade (M4 spec §4.6).
//
// Decides what existing master_variant (if any) an incoming Shopify variant
// should link to. Falls through 4 deterministic steps before giving up and
// telling the caller to mint a new master. The cascade reads from the
// MasterVariantsPort and (for step 4) the EmbeddingPort; it doesn't write
// anything itself — the harvester acts on the returned MatchResult.
//
// Why a cascade and not just embedding for everything: GTIN exact and SKU
// exact are deterministic, cheap, and unambiguous when they hit. Fuzzy
// title + axes match catches the "same product, slightly different listing
// text" case at near-zero cost. Embedding is the expensive long-tail
// catch-all and only runs when the first three steps strike out.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// Threshold defaults — overridable via NewMatchCascadeWithThresholds for tests.
const (
	defaultFuzzyThreshold     = 0.85
	defaultEmbeddingThreshold = 0.92
	defaultFuzzyLimit         = 5
	defaultEmbeddingLimit     = 5
)

// VariantInput is what the harvester hands the cascade for one Shopify
// variant. We deliberately keep this narrower than the full MasterVariant
// domain type — the harvester picks out the fields the cascade needs
// (title, vendor, SKU, GTINs, axes) and leaves dimension/material/etc.
// for the post-match write step.
type VariantInput struct {
	TenantID      string
	ProductTitle  string            // parent product.title
	Vendor        string
	SKU           string
	GTINs         []string
	Axes          map[string]string // {"size":"236ml","color":"Silver"}
	Description   string            // for embedding text
	Brand         string            // duplicate of Vendor — kept for embedding text variations
	EmbeddingText string            // optional — caller can override the canonical embed text
}

// MatchCascade orchestrates the 4-step lookup. Composable: pass nil
// embedClient to skip step 4 (useful in tests).
type MatchCascade struct {
	variants ports.MasterVariantsPort
	embedder ports.EmbeddingPort
	log      *logger.Logger

	fuzzyThreshold     float64
	embeddingThreshold float64
	fuzzyLimit         int
	embeddingLimit     int
}

func NewMatchCascade(variants ports.MasterVariantsPort, embedder ports.EmbeddingPort, log *logger.Logger) *MatchCascade {
	return &MatchCascade{
		variants:           variants,
		embedder:           embedder,
		log:                log,
		fuzzyThreshold:     defaultFuzzyThreshold,
		embeddingThreshold: defaultEmbeddingThreshold,
		fuzzyLimit:         defaultFuzzyLimit,
		embeddingLimit:     defaultEmbeddingLimit,
	}
}

// NewMatchCascadeWithThresholds is a test/tuning constructor — production
// should stay on the spec defaults via NewMatchCascade.
func NewMatchCascadeWithThresholds(variants ports.MasterVariantsPort, embedder ports.EmbeddingPort, log *logger.Logger, fuzzy, embed float64, fuzzyLimit, embedLimit int) *MatchCascade {
	mc := NewMatchCascade(variants, embedder, log)
	if fuzzy > 0 {
		mc.fuzzyThreshold = fuzzy
	}
	if embed > 0 {
		mc.embeddingThreshold = embed
	}
	if fuzzyLimit > 0 {
		mc.fuzzyLimit = fuzzyLimit
	}
	if embedLimit > 0 {
		mc.embeddingLimit = embedLimit
	}
	return mc
}

// Match runs the cascade and returns the disposition. Errors are returned
// only for I/O / DB failures — "no match" is a normal NewMaster outcome,
// not an error.
func (mc *MatchCascade) Match(ctx context.Context, in VariantInput) (*domain.MatchResult, error) {
	if mc.variants == nil {
		return nil, errors.New("match cascade: variants port is nil")
	}

	// --- Step 1: GTIN exact ---------------------------------------------------
	if len(in.GTINs) > 0 {
		hits, err := mc.variants.FindByGTIN(ctx, in.GTINs)
		if err != nil {
			return nil, fmt.Errorf("gtin lookup: %w", err)
		}
		switch len(hits) {
		case 1:
			return &domain.MatchResult{
				Outcome:         domain.MatchOutcomeLinked,
				MasterVariantID: hits[0].ID,
				Confidence:      domain.MatchConfidenceGTINExact,
				Reason:          "GTIN exact: " + strings.Join(in.GTINs, ","),
			}, nil
		case 0:
			// fall through
		default:
			// Multiple variants share a GTIN — data quality issue. Push to
			// curator review with all candidates listed.
			cands := make([]domain.MatchCandidate, 0, len(hits))
			for _, h := range hits {
				cands = append(cands, domain.MatchCandidate{
					MasterVariantID: h.ID,
					Reason:          "gtin",
					Score:           1.0,
				})
			}
			return &domain.MatchResult{
				Outcome:    domain.MatchOutcomeReviewQueue,
				Reason:     fmt.Sprintf("GTIN collision (%d variants share %v)", len(hits), in.GTINs),
				Candidates: cands,
			}, nil
		}
	}

	// --- Step 2: Vendor + SKU exact -------------------------------------------
	if in.Vendor != "" && in.SKU != "" {
		hits, err := mc.variants.FindByVendorAndSKU(ctx, in.Vendor, in.SKU)
		if err != nil {
			return nil, fmt.Errorf("vendor+sku lookup: %w", err)
		}
		if len(hits) == 1 {
			return &domain.MatchResult{
				Outcome:         domain.MatchOutcomeLinked,
				MasterVariantID: hits[0].ID,
				Confidence:      domain.MatchConfidenceSKUExact,
				Reason:          fmt.Sprintf("Vendor+SKU exact: %s / %s", in.Vendor, in.SKU),
			}, nil
		}
		// >1 here means the same vendor+SKU is on multiple variants — also a
		// data-quality issue but rarer than GTIN collisions; falls through to
		// fuzzy in case axes disambiguate.
	}

	// --- Step 3: Fuzzy similarity (vendor + name) + axes match ----------------
	if in.Vendor != "" && in.ProductTitle != "" {
		hits, err := mc.variants.FindByVendorAndFuzzyName(ctx, in.Vendor, in.ProductTitle, mc.fuzzyThreshold, mc.fuzzyLimit)
		if err != nil {
			return nil, fmt.Errorf("fuzzy lookup: %w", err)
		}
		// Walk top-down — the SQL already orders by similarity DESC.
		for _, h := range hits {
			if axesMatch(in.Axes, h.Axes) {
				return &domain.MatchResult{
					Outcome:         domain.MatchOutcomeLinked,
					MasterVariantID: h.ID,
					Confidence:      domain.MatchConfidenceFuzzyHigh,
					Reason: fmt.Sprintf("Fuzzy title (sim=%.2f) + axes match: %s",
						h.MatchedScore, in.ProductTitle),
				}, nil
			}
		}
	}

	// --- Step 4: Embedding similarity -----------------------------------------
	if mc.embedder != nil {
		text := mc.composeEmbedText(in)
		if text != "" {
			vecs, err := mc.embedder.Embed(ctx, []string{text})
			if err != nil {
				// Embedding failures shouldn't block the import — log and
				// fall through to NewMaster. Spec emphasizes pipeline robustness.
				mc.log.Info("match_cascade_embed_failed", "error", err.Error())
			} else if len(vecs) == 1 && len(vecs[0]) > 0 {
				hits, err := mc.variants.FindByEmbedding(ctx, vecs[0], mc.embeddingThreshold, mc.embeddingLimit)
				if err != nil {
					return nil, fmt.Errorf("embedding lookup: %w", err)
				}
				if len(hits) > 0 {
					// Embedding is rarely "obviously the same product" — push
					// to review queue rather than auto-link, even on a high
					// score. Spec §4.6 step 4.
					cands := make([]domain.MatchCandidate, 0, len(hits))
					for _, h := range hits {
						cands = append(cands, domain.MatchCandidate{
							MasterVariantID: h.ID,
							Reason:          "embedding",
							Score:           h.MatchedScore,
						})
					}
					return &domain.MatchResult{
						Outcome:    domain.MatchOutcomeReviewQueue,
						Reason:     fmt.Sprintf("Embedding similarity (top score %.3f)", hits[0].MatchedScore),
						Candidates: cands,
					}, nil
				}
			}
		}
	}

	// --- Step 5: Nothing matched → caller mints a new master ------------------
	return &domain.MatchResult{
		Outcome:    domain.MatchOutcomeNewMaster,
		Confidence: domain.MatchConfidenceUnverified,
		Reason:     "No existing variant matched; new master will be created",
	}, nil
}

// composeEmbedText builds the text we send through the embedder for a
// fresh-from-Shopify variant. Caller can override via EmbeddingText if it
// already has a richer composition (e.g. with metafield blob).
func (mc *MatchCascade) composeEmbedText(in VariantInput) string {
	if in.EmbeddingText != "" {
		return in.EmbeddingText
	}
	parts := make([]string, 0, 4)
	if in.Brand != "" {
		parts = append(parts, in.Brand)
	} else if in.Vendor != "" {
		parts = append(parts, in.Vendor)
	}
	if in.ProductTitle != "" {
		parts = append(parts, in.ProductTitle)
	}
	if in.Description != "" {
		parts = append(parts, in.Description)
	}
	for k, v := range in.Axes {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}

// axesMatch returns true when every axis from `in` is satisfied by the
// candidate's axes. Empty `in` axes match anything (we don't require all
// candidate axes to be present in the input — extra axes on candidate is
// allowed). Comparison is case-insensitive on values; key comparison is
// also case-insensitive but normalized to lowercase up front.
func axesMatch(input, candidate map[string]string) bool {
	if len(input) == 0 {
		return true
	}
	if len(candidate) == 0 {
		return false
	}
	candidateLC := make(map[string]string, len(candidate))
	for k, v := range candidate {
		candidateLC[strings.ToLower(k)] = strings.ToLower(v)
	}
	for k, v := range input {
		got, ok := candidateLC[strings.ToLower(k)]
		if !ok {
			return false
		}
		if got != strings.ToLower(v) {
			return false
		}
	}
	return true
}
