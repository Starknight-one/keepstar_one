// Package usecases — Merge applier (Phase D2, catalog completion 2026-04-28).
//
// Walks every listing for a tenant, applies the rules in their MappingArtifact
// (BrandMapping, FieldMapping, JunkRules, MatchStrategyConfig, etc.), and emits
// a per-listing MergeProposal. The whole pass is read-only against master —
// nothing writes to master_products / master_variants here. Curator reviews the
// proposals (per-row or bulk) and approves; a separate Apply pass (Phase D3)
// performs the transactional master writes.
//
// Why split GenerateReport from Apply: applying merges is destructive and
// needs rollback snapshots + per-proposal transactions. Decoupling lets the
// curator iterate on what to approve without re-running the agent or the
// match cascade. Re-running GenerateReport after a schema edit is cheap.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
	"keepstar-admin/internal/units"
)

// MergeApplyUseCase orchestrates report generation. Apply step lives on the
// same struct (Phase D3) — keeping them together lets us share dependencies.
type MergeApplyUseCase struct {
	catalog ports.AdminCatalogPort
	schema  ports.MappingArtifactPort
	reports ports.MergeReportsPort
	cascade *MatchCascade
	tx      ports.MergeApplyTxPort // optional — set via WithApplyTx; required for ApplyProposals/Revert
	audit   ports.AuditPort        // optional — set via WithAudit; if nil, applier skips audit writes
	log     *logger.Logger
}

func NewMergeApplyUseCase(
	catalog ports.AdminCatalogPort,
	schema ports.MappingArtifactPort,
	reports ports.MergeReportsPort,
	cascade *MatchCascade,
	log *logger.Logger,
) *MergeApplyUseCase {
	return &MergeApplyUseCase{
		catalog: catalog,
		schema:  schema,
		reports: reports,
		cascade: cascade,
		log:     log,
	}
}

// WithApplyTx wires the destructive write port for Phase D3. Without it,
// ApplyProposals/Revert will refuse to run (they're nil-safe by check).
func (uc *MergeApplyUseCase) WithApplyTx(tx ports.MergeApplyTxPort) *MergeApplyUseCase {
	uc.tx = tx
	return uc
}

// WithAudit wires the audit port for ApplyProposals/Revert. Optional —
// missing audit makes the operations work but un-traceable, which is fine
// for tests but should never be the case in production.
func (uc *MergeApplyUseCase) WithAudit(a ports.AuditPort) *MergeApplyUseCase {
	uc.audit = a
	return uc
}

// GenerateReportResult is the wire shape returned to handlers / CLI.
type GenerateReportResult struct {
	ReportID           int64                    `json:"reportId"`
	Status             domain.MergeReportStatus `json:"status"`
	TotalListings      int                      `json:"totalListings"`
	AutoLinkCount      int                      `json:"autoLinkCount"`
	NewMasterCount     int                      `json:"newMasterCount"`
	NeedsReviewCount   int                      `json:"needsReviewCount"`
	SkipCount          int                      `json:"skipCount"`
	AlreadyLinkedCount int                      `json:"alreadyLinkedCount"`
	DurationMs         int64                    `json:"durationMs"`
}

// GenerateReport reads the active MappingArtifact for the tenant, walks all
// catalog.products rows, and emits one MergeProposal per listing. The returned
// report is persisted (with prior reports for the tenant marked superseded)
// and ready for curator review. Idempotent on re-run.
//
// Cost: zero LLM calls. Pure deterministic Go + match-cascade SQL.
func (uc *MergeApplyUseCase) GenerateReport(ctx context.Context, tenantID string) (*GenerateReportResult, error) {
	if tenantID == "" {
		return nil, errors.New("merge apply: tenantID required")
	}
	start := time.Now()

	// 1. Load the active MappingArtifact. No schema → no rules → can't apply.
	schema, err := uc.schema.GetMappingArtifact(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("load schema: %w", err)
	}
	if schema == nil {
		return nil, errors.New("no mapping artifact for tenant — run discovery first")
	}
	artifact := schema.MappingArtifact

	// 2. List all listings for the tenant. We page through to handle big
	// catalogs without holding everything in memory at once. Each page is
	// processed and its proposals appended to the working slice.
	var proposals []domain.MergeProposal
	const pageSize = 500
	offset := 0
	for {
		filter := domain.AdminProductFilter{
			Limit:  pageSize,
			Offset: offset,
		}
		listings, total, err := uc.catalog.ListProducts(ctx, tenantID, filter)
		if err != nil {
			return nil, fmt.Errorf("list listings (offset=%d): %w", offset, err)
		}
		if len(listings) == 0 {
			break
		}
		for i := range listings {
			p := uc.buildProposal(ctx, &artifact, &listings[i])
			proposals = append(proposals, p)
		}
		offset += len(listings)
		if offset >= total {
			break
		}
	}

	// 3. Aggregate counters for the report header.
	counters := ports.MergeReportCounters{TotalListings: len(proposals)}
	for _, p := range proposals {
		switch p.Action {
		case domain.MergeActionLinkExisting, domain.MergeActionVariantOfExisting:
			counters.AutoLinkCount++
		case domain.MergeActionNewMaster:
			counters.NewMasterCount++
		case domain.MergeActionNeedsReview:
			counters.NeedsReviewCount++
		case domain.MergeActionSkip:
			counters.SkipCount++
		case domain.MergeActionAlreadyLinked:
			counters.AlreadyLinkedCount++
		}
	}

	// 4. Persist as a new report; supersedes the prior pending/reviewed one.
	report := &domain.MergeReport{
		TenantID:         tenantID,
		GeneratedAt:      time.Now().UTC(),
		SchemaVersion:    artifact.Version,
		Status:           domain.MergeReportStatusPending,
		AgentMetaSummary:   artifact.AgentNotes,
		TotalListings:      counters.TotalListings,
		AutoLinkCount:      counters.AutoLinkCount,
		NewMasterCount:     counters.NewMasterCount,
		NeedsReviewCount:   counters.NeedsReviewCount,
		SkipCount:          counters.SkipCount,
		AlreadyLinkedCount: counters.AlreadyLinkedCount,
		Proposals:          proposals,
	}
	id, err := uc.reports.Save(ctx, report)
	if err != nil {
		return nil, fmt.Errorf("save report: %w", err)
	}

	uc.log.Info("merge_report_generated",
		"tenant_id", tenantID, "report_id", id,
		"total", counters.TotalListings,
		"auto_link", counters.AutoLinkCount,
		"new_master", counters.NewMasterCount,
		"needs_review", counters.NeedsReviewCount,
		"skip", counters.SkipCount,
		"already_linked", counters.AlreadyLinkedCount,
		"duration_ms", time.Since(start).Milliseconds())

	return &GenerateReportResult{
		ReportID:           id,
		Status:             report.Status,
		TotalListings:      counters.TotalListings,
		AutoLinkCount:      counters.AutoLinkCount,
		NewMasterCount:     counters.NewMasterCount,
		NeedsReviewCount:   counters.NeedsReviewCount,
		SkipCount:          counters.SkipCount,
		AlreadyLinkedCount: counters.AlreadyLinkedCount,
		DurationMs:         time.Since(start).Milliseconds(),
	}, nil
}

// buildProposal applies the artifact rules to one listing and returns its
// proposal. This is the heart of D2 — every catalog completion decision
// hangs off the rules in the artifact and the match-cascade outcome.
//
// Order of checks (cheapest / most decisive first):
//   1. JunkRules: vendor blacklist / require_identifier / brand_mapping action=skip
//   2. Vertical resolution: BrandMapping or category mapping infers vertical
//   3. Match cascade: GTIN → vendor+SKU → fuzzy → embedding (per MatchStrategyConfig)
//   4. Score → action (auto link / needs_review / new master)
func (uc *MergeApplyUseCase) buildProposal(ctx context.Context, artifact *domain.MappingArtifact, listing *domain.Product) domain.MergeProposal {
	p := domain.MergeProposal{
		// Surrogate id stamped at generation so curator UI can address each
		// proposal in apply/reject calls. Listing id alone would work but
		// would couple the API to listing primary keys.
		ID:            uuid.NewString(),
		ListingID:     listing.ID,
		ListingName:   firstNonEmpty(listing.OriginalName, listing.Name),
		ListingVendor: vendorFromListing(listing),
		Status:        domain.MergeProposalStatusPending,
	}

	// Rule 0 (must come before everything else): if the listing is already
	// merged with master — either curator linked it earlier, or the legacy
	// importer pre-set master_product_id — DO NOT propose anything. Treat as
	// a no-op marker. Skipping this check means re-running the applier on a
	// tenant whose catalog is already merged would generate "create new master"
	// proposals against existing merges and double the master_products on apply.
	if listing.MasterVariantID != "" || listing.MasterProductID != "" {
		p.Action = domain.MergeActionAlreadyLinked
		p.TargetMasterVariantID = listing.MasterVariantID
		// Reuse SkipReason wording so the curator UI has one field to render
		// for "no action required" rows.
		p.SkipReason = "already_linked_to_master"
		return p
	}

	vendor := strings.ToLower(strings.TrimSpace(p.ListingVendor))

	// Rule 1a: brand_mapping action=skip wins outright.
	if bm, ok := artifact.BrandMapping[vendor]; ok && bm.Action == "skip" {
		p.Action = domain.MergeActionSkip
		p.SkipReason = "brand_mapping_skip:" + nonEmpty(bm.Reason, "vendor flagged in BrandMapping")
		return p
	}

	// Rule 1b: junk vendor blacklist.
	if artifact.JunkRules != nil {
		for _, v := range artifact.JunkRules.VendorBlacklist {
			if v == vendor {
				p.Action = domain.MergeActionSkip
				p.SkipReason = "junk_vendor:" + vendor
				return p
			}
		}
	}

	// Rule 1c: junk axis pattern in any variant.
	if artifact.JunkRules != nil && len(artifact.JunkRules.AxisNamePatterns) > 0 {
		if hit := matchesJunkAxis(listing, artifact.JunkRules.AxisNamePatterns); hit != "" {
			p.Action = domain.MergeActionSkip
			p.SkipReason = "junk_axis:" + hit
			return p
		}
	}

	// Rule 1d: require_identifier (default true when JunkRules non-nil).
	requireID := artifact.JunkRules == nil || artifact.JunkRules.RequireIdentifier
	skuPresent, gtinPresent := identifiersFromListing(listing)
	if requireID && !skuPresent && !gtinPresent {
		p.Action = domain.MergeActionSkip
		p.SkipReason = "no_identifier"
		return p
	}

	// Rule 2: resolve vertical. Used to filter the match cascade and to
	// stamp on a new master if we end up creating one.
	vertical := resolveVertical(artifact, vendor, listing)

	// Rule 3: match cascade. Skip embedding step for verticals where master
	// has no coverage (configured by agent in MatchStrategyConfig).
	thresholds := thresholdsFromArtifact(artifact)
	skipEmbedding := false
	if artifact.MatchStrategyConfig != nil {
		for _, v := range artifact.MatchStrategyConfig.EmbeddingDisabledFor {
			if v == vertical {
				skipEmbedding = true
				break
			}
		}
	}
	gtins := gtinsFromListing(listing)
	sku := skuFromListing(listing)
	in := VariantInput{
		TenantID:     listing.TenantID,
		ProductTitle: firstNonEmpty(listing.OriginalName, listing.Name),
		Vendor:       p.ListingVendor,
		SKU:          sku,
		GTINs:        gtins,
		Brand:        p.ListingVendor,
		Description:  listing.Description,
	}
	cascade := uc.cascade
	if skipEmbedding && cascade != nil {
		// Local clone with no embedder — temporarily disable step 4. Rather
		// than mutating the shared cascade, we run our own with embedder=nil.
		cascade = NewMatchCascade(cascade.variants, nil, uc.log)
	}

	var matchResult *domain.MatchResult
	var matchErr error
	if cascade != nil {
		matchResult, matchErr = cascade.Match(ctx, in)
		if matchErr != nil {
			uc.log.Warn("merge_apply_match_failed",
				"listing_id", listing.ID, "error", matchErr.Error())
		}
	}

	// Rule 4: outcome → action.
	switch {
	case matchResult != nil && matchResult.Outcome == domain.MatchOutcomeLinked:
		// Cascade returned a single high-confidence link.
		p.Action = domain.MergeActionLinkExisting
		p.TargetMasterVariantID = matchResult.MasterVariantID
		p.MatchEvidence = &domain.MatchEvidence{
			Strategy:   strategyFromConfidence(matchResult.Confidence),
			Score:      scoreFromConfidence(matchResult.Confidence),
			Confidence: string(matchResult.Confidence),
			Reasoning:  matchResult.Reason,
		}
		// Auto-link only when score clears the configured threshold; otherwise
		// downgrade to needs_review so the curator confirms.
		if p.MatchEvidence.Score < thresholds.AutoLink {
			p.Action = domain.MergeActionNeedsReview
		}
	case matchResult != nil && matchResult.Outcome == domain.MatchOutcomeReviewQueue:
		p.Action = domain.MergeActionNeedsReview
		p.MatchEvidence = &domain.MatchEvidence{
			Strategy:   "ambiguous",
			Confidence: string(matchResult.Confidence),
			Reasoning:  matchResult.Reason,
		}
	case matchResult != nil && matchResult.Outcome == domain.MatchOutcomeNewMaster:
		p.Action = domain.MergeActionNewMaster
		p.MatchEvidence = &domain.MatchEvidence{
			Strategy:  "no_match",
			Reasoning: nonEmpty(matchResult.Reason, "cascade returned no link candidate"),
		}
		p.ProposedMaster = buildProposedMaster(artifact, listing, vendor, vertical)
	default:
		// Cascade unavailable or errored — defensive needs_review.
		p.Action = domain.MergeActionNeedsReview
		reason := "match cascade unavailable"
		if matchErr != nil {
			reason = matchErr.Error()
		}
		p.MatchEvidence = &domain.MatchEvidence{Strategy: "error", Reasoning: reason}
	}

	return p
}

// thresholds are the auto/needs-review/skip cutoff scores.
type matchThresholds struct {
	AutoLink     float64
	NeedsReview  float64
	SkipBelow    float64
}

func thresholdsFromArtifact(a *domain.MappingArtifact) matchThresholds {
	if a.MatchStrategyConfig != nil {
		return matchThresholds{
			AutoLink:    a.MatchStrategyConfig.AutoLinkThreshold,
			NeedsReview: a.MatchStrategyConfig.NeedsReviewThreshold,
			SkipBelow:   a.MatchStrategyConfig.SkipBelow,
		}
	}
	return matchThresholds{AutoLink: 0.90, NeedsReview: 0.50, SkipBelow: 0.30}
}

// strategyFromConfidence maps the cascade's MatchConfidence enum back to a
// strategy label for the proposal. The cascade today doesn't surface raw
// strategy/score; it's derivable from the confidence band it returns.
func strategyFromConfidence(c domain.MatchConfidence) string {
	switch c {
	case domain.MatchConfidenceGTINExact:
		return "gtin_exact"
	case domain.MatchConfidenceSKUExact:
		return "vendor_sku_exact"
	case domain.MatchConfidenceFuzzyHigh:
		return "fuzzy_title"
	case domain.MatchConfidenceEmbedding:
		return "embedding"
	default:
		return string(c)
	}
}

// scoreFromConfidence picks a representative score so the report can sort
// proposals by confidence even though the cascade doesn't return raw numbers.
// Numbers chosen to match the structured thresholds (0.95 GTIN, 0.90 SKU,
// 0.85 fuzzy_high, 0.92 embedding) — agree with cascade defaults.
func scoreFromConfidence(c domain.MatchConfidence) float64 {
	switch c {
	case domain.MatchConfidenceGTINExact:
		return 0.98
	case domain.MatchConfidenceSKUExact:
		return 0.95
	case domain.MatchConfidenceFuzzyHigh:
		return 0.85
	case domain.MatchConfidenceEmbedding:
		return 0.80
	default:
		return 0.5
	}
}

// resolveVertical picks the vertical for a listing from the artifact rules.
// Resolution order — first non-empty wins:
//
//  1. BrandMapping[vendor].Vertical — explicit hint from the discovery agent
//     (required for action="create_new", optional for "link_existing").
//  2. MasterTemplate matched by collection name (CategoryHints).
//  3. MasterTemplate matched by vendor name (rare).
//  4. VerticalUnknown — logged as a soft warning at the call site.
//
// We deliberately do NOT hardcode "cosmetics" anywhere — that broke
// furniture/footwear tenants by stamping the wrong vertical when the agent
// didn't supply a hint. If you see "unknown" stamped on real proposals, the
// fix is to update the discovery prompt or have the curator edit
// BrandMapping[vendor].Vertical, not to reintroduce a fallback here.
func resolveVertical(a *domain.MappingArtifact, vendor string, listing *domain.Product) string {
	// 1. Explicit BrandMapping vertical hint (works for both link_existing and
	// create_new — discovery agent is required to provide it for create_new).
	if bm, ok := a.BrandMapping[vendor]; ok && bm.Vertical != "" {
		return bm.Vertical
	}

	// 2. Collection name → MasterTemplates category hints.
	collections := collectionsFromListing(listing)
	for _, c := range collections {
		lc := strings.ToLower(c)
		for _, t := range a.MasterTemplates {
			for _, hint := range t.CategoryHints {
				if strings.Contains(lc, strings.ToLower(hint)) {
					return t.Vertical
				}
			}
		}
	}

	// 3. Vendor → MasterTemplate match (rare but possible).
	for _, t := range a.MasterTemplates {
		for _, hint := range t.CategoryHints {
			if strings.Contains(strings.ToLower(vendor), strings.ToLower(hint)) {
				return t.Vertical
			}
		}
	}

	return VerticalUnknown
}

// buildProposedMaster constructs the new-master shape the applier would
// create when the cascade has no candidate.
func buildProposedMaster(a *domain.MappingArtifact, listing *domain.Product, vendor, vertical string) *domain.ProposedMaster {
	pm := &domain.ProposedMaster{
		Name:        firstNonEmpty(listing.OriginalName, listing.Name),
		Brand:       p_titleCase(vendor),
		Vertical:    vertical,
		Description: listing.Description,
	}
	// Image: prefer first listing image.
	if len(listing.Images) > 0 {
		pm.ImageURL = listing.Images[0]
	}
	// Variant from listing's first variant.
	pv := domain.ProposedVariant{
		SKU:   skuFromListing(listing),
		GTINs: gtinsFromListing(listing),
	}
	if listing.WeightG != nil {
		pv.WeightG = listing.WeightG
	}
	if listing.VolumeML != nil {
		pv.VolumeML = listing.VolumeML
	}
	pv.Color = listing.Color
	pv.Size = listing.Size
	pv.ImageURL = pm.ImageURL
	pm.Variant = pv
	// Tier-2 from raw_attributes per FieldMapping.
	if listing.RawAttributes != nil {
		pm.Tier2 = extractTier2(a, listing.RawAttributes, vertical)
	}
	return pm
}

// extractTier2 walks raw_attributes and pulls fields the artifact says map to
// tier2.<key>, applying target.Transform on the way (units parse, lowercase,
// shorten, split). Unknown transforms fall back to writing the value 1:1 with
// a warning so we don't silently drop data.
func extractTier2(a *domain.MappingArtifact, raw map[string]interface{}, vertical string) map[string]interface{} {
	out := map[string]interface{}{}
	for sourcePath, target := range a.FieldMapping {
		if !strings.HasPrefix(target.Target, "master.tier2.") &&
			!strings.HasPrefix(target.Target, "tier2.") {
			continue
		}
		// FieldPath under raw_attributes — try both literal and synthetic
		// "metafields[key=X].value" form (the discovery agent tends to emit
		// both flavors).
		val := lookupPath(raw, sourcePath)
		if val == nil {
			continue
		}
		key := strings.TrimPrefix(strings.TrimPrefix(target.Target, "master.tier2."), "tier2.")
		out[key] = applyTransform(val, target.Transform, target.DefaultUnit)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// applyTransform converts a raw value to its canonical tier2 form per the
// agent-supplied transform name. Returns the original value unchanged when:
//   - transform is empty (no transform requested)
//   - transform is unknown (defensive fallback; logged at the call site is
//     impractical here so we just pass through)
//   - the underlying parser fails (bad input shape — keep raw value visible
//     in the curator UI rather than silently dropping it)
//
// Recognised transforms mirror discovery_tools.go:127 — units.{weight|volume|
// length|count}, lowercase, shorten:N, split:DELIM.
func applyTransform(val any, transform, defaultUnit string) any {
	if transform == "" {
		return val
	}
	s, isString := val.(string)

	switch {
	case transform == "units.weight", transform == "units.volume",
		transform == "units.length", transform == "units.count":
		if !isString {
			return val
		}
		opts := units.ParseOpts{}
		if defaultUnit != "" {
			opts.DefaultUnit = units.Canonical(defaultUnit)
		}
		res := units.Parse(s, opts)
		if res.Status == units.ParseStatusOK {
			return res.Value
		}
		return val

	case transform == "lowercase":
		if !isString {
			return val
		}
		return strings.ToLower(s)

	case strings.HasPrefix(transform, "shorten:"):
		if !isString {
			return val
		}
		n, err := strconv.Atoi(strings.TrimPrefix(transform, "shorten:"))
		if err != nil || n <= 0 {
			return val
		}
		runes := []rune(s)
		if len(runes) <= n {
			return s
		}
		return string(runes[:n])

	case strings.HasPrefix(transform, "split:"):
		if !isString {
			return val
		}
		delim := strings.TrimPrefix(transform, "split:")
		if delim == "" {
			return val
		}
		parts := strings.Split(s, delim)
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		return out
	}

	// Unknown transform — pass through. We don't have a logger handle here;
	// callers can detect by spotting non-canonical types in tier2.
	return val
}

// --- helpers -------------------------------------------------------------

func vendorFromListing(l *domain.Product) string {
	if l.Brand != "" {
		return l.Brand
	}
	if l.RawAttributes != nil {
		if v, ok := l.RawAttributes["vendor"].(string); ok {
			return v
		}
	}
	return ""
}

func skuFromListing(l *domain.Product) string {
	if l.SKU != "" {
		return l.SKU
	}
	// Fallback: first variant in raw_attributes.
	if l.RawAttributes != nil {
		if vs, ok := l.RawAttributes["variants"].([]interface{}); ok && len(vs) > 0 {
			if v, ok := vs[0].(map[string]interface{}); ok {
				if s, ok := v["sku"].(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func gtinsFromListing(l *domain.Product) []string {
	if len(l.GTINs) > 0 {
		return l.GTINs
	}
	if l.RawAttributes == nil {
		return nil
	}
	vs, ok := l.RawAttributes["variants"].([]interface{})
	if !ok {
		return nil
	}
	var gtins []string
	for _, v := range vs {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if b, ok := m["barcode"].(string); ok && b != "" {
			gtins = append(gtins, b)
		}
	}
	return gtins
}

func collectionsFromListing(l *domain.Product) []string {
	if l.RawAttributes == nil {
		return nil
	}
	cs, ok := l.RawAttributes["collections"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if t, ok := m["title"].(string); ok && t != "" {
			out = append(out, t)
		}
	}
	return out
}

func identifiersFromListing(l *domain.Product) (skuPresent, gtinPresent bool) {
	skuPresent = skuFromListing(l) != ""
	gtinPresent = len(gtinsFromListing(l)) > 0
	return
}

func matchesJunkAxis(l *domain.Product, patterns []string) string {
	if l.RawAttributes == nil {
		return ""
	}
	vs, ok := l.RawAttributes["variants"].([]interface{})
	if !ok {
		return ""
	}
	for _, v := range vs {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		opts, ok := m["options"].(map[string]interface{})
		if !ok {
			continue
		}
		for k := range opts {
			lk := strings.ToLower(k)
			for _, pat := range patterns {
				if strings.Contains(lk, pat) {
					return pat
				}
			}
		}
	}
	return ""
}

// lookupPath resolves a discovery-agent field path against a raw_attributes
// map. Three syntaxes are supported, matched in order:
//
//  1. Direct key — `vendor`, `product_type` etc.
//  2. Bracket form `metafields[key=X].value` — explicit array walk by key.
//  3. Dot form `metafields.<namespace>.<key>` — Sonnet's preferred shorthand.
//     Walks the metafields array and matches both namespace AND key (so
//     `metafields.custom.wood_type` returns the value of the row where
//     namespace="custom" AND key="wood_type"). Two-segment form
//     `metafields.<key>` (no namespace) matches by key alone.
//
// Returns nil if the path resolves to nothing — caller decides if that means
// "skip" or "use default".
func lookupPath(m map[string]interface{}, path string) interface{} {
	// Direct key.
	if v, ok := m[path]; ok {
		return v
	}
	// metafields[key=X].value form.
	if strings.HasPrefix(path, "metafields[key=") && strings.HasSuffix(path, "].value") {
		key := strings.TrimSuffix(strings.TrimPrefix(path, "metafields[key="), "].value")
		return findMetafield(m, "", key)
	}
	// metafields.<ns>.<key> or metafields.<key>.
	if strings.HasPrefix(path, "metafields.") {
		rest := strings.TrimPrefix(path, "metafields.")
		parts := strings.SplitN(rest, ".", 2)
		switch len(parts) {
		case 1:
			return findMetafield(m, "", parts[0])
		case 2:
			return findMetafield(m, parts[0], parts[1])
		}
	}
	return nil
}

// findMetafield walks raw_attributes.metafields[] and returns the .value of
// the first row matching (namespace, key). Empty namespace means "match key
// regardless of namespace" — used by the legacy bracket form.
func findMetafield(m map[string]interface{}, namespace, key string) interface{} {
	mfs, ok := m["metafields"].([]interface{})
	if !ok {
		return nil
	}
	for _, mf := range mfs {
		rec, ok := mf.(map[string]interface{})
		if !ok {
			continue
		}
		k, _ := rec["key"].(string)
		if k != key {
			continue
		}
		if namespace != "" {
			ns, _ := rec["namespace"].(string)
			if ns != namespace {
				continue
			}
		}
		return rec["value"]
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func nonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

// p_titleCase capitalizes the first letter of each word — good-enough title-
// case for vendor → brand seeding. Underscored to avoid collision with the
// stdlib `strings.Title` deprecation warnings.
func p_titleCase(s string) string {
	if s == "" {
		return s
	}
	words := strings.Fields(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		runes := []rune(w)
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}
