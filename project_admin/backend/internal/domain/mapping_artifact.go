package domain

import "time"

// MappingArtifactStatus tracks lifecycle of a tenant's mapping artifact.
// stale = needs regeneration on next discovery; needs_human_review = agent
// couldn't produce a high-coverage mapping, curator must approve.
type MappingArtifactStatus string

const (
	MappingArtifactStatusActive            MappingArtifactStatus = "active"
	MappingArtifactStatusStale             MappingArtifactStatus = "stale"
	MappingArtifactStatusNeedsHumanReview  MappingArtifactStatus = "needs_human_review"
)

// FieldMappingTarget describes where a tenant's source field lands in our schema.
// Spec §4.3 format example:
//   {"target":"master_variants.volume_ml","transform":"ml_from_string","default_unit":"ml"}
type FieldMappingTarget struct {
	Target      string   `json:"target"`                // dotted path e.g. "master.brand", "candidate:hair_type"
	Transform   string   `json:"transform,omitempty"`   // e.g. "ml_from_string"
	DefaultUnit string   `json:"default_unit,omitempty"`
	Type        string   `json:"type,omitempty"`        // for candidates: "enum"|"text"|"number"
	Samples     []string `json:"samples,omitempty"`
	ShortenTo   string   `json:"shorten_to,omitempty"`  // when listing.original_name → display_name needed
	ShortenMax  int      `json:"shorten_max,omitempty"`
}

// CategoryMappingTarget routes a tenant collection to a master category or
// marks it as showcase/promo (no master mapping).
type CategoryMappingTarget struct {
	Target      string `json:"target,omitempty"`       // "master_category:cleansing" or null
	TenantLabel string `json:"tenant_label"`
	Kind        string `json:"kind,omitempty"`         // "category" | "showcase" | "promo"
}

// MappingArtifact is the declarative output of the agent normalizer
// (originally spec §4.3, extended 2026-04-28 to cover the curator-driven merge
// flow). One LLM call per tenant produces this; subsequent imports apply it
// via plain code with no LLM in the hot path.
//
// The fields below split into two roles. Field/category/master-template
// mapping (FieldMapping/CategoryMapping/MasterTemplates) tells the harvester
// HOW to translate one tenant's raw payload into our schema. The merge-side
// fields (BrandMapping, JunkRules, MatchStrategyConfig) tell the deterministic
// merge applier WHEN to link a listing to an existing master, when to create
// a new one, and when to skip. Both sides come out of the same agent call —
// the agent sees the tenant's catalog digest plus a master overview and emits
// rules that cover the whole flow.
type MappingArtifact struct {
	Version          int                                `json:"version"`
	ValidatedAt      time.Time                          `json:"validated_at"`
	Status           MappingArtifactStatus              `json:"status"`
	FieldMapping     map[string]FieldMappingTarget      `json:"field_mapping"`
	CategoryMapping  map[string]CategoryMappingTarget   `json:"category_mapping,omitempty"`
	MatchStrategy    []string                           `json:"match_strategy"` // e.g. ["gtin","vendor+sku","vendor+title+axes","embedding"]
	VariantStrategy  string                             `json:"variant_strategy"` // e.g. "master_with_variants"
	AgentNotes       string                             `json:"agent_notes,omitempty"`

	// MasterTemplates carries vertical-template proposals from the discovery
	// agent (M4c). For new verticals (e.g. furniture, electronics) the agent
	// proposes the shape of the master schema — what fields belong on master
	// vs. variants, what categories to expect. The applier creates masters
	// according to these templates; curator promotes Tier 2 columns
	// (e.g. master_furniture) by registering fields in master_field_definitions.
	MasterTemplates []MasterTemplateProposal `json:"master_templates,omitempty"`

	// --- Merge-side fields (catalog completion 2026-04-28) ---

	// BrandMapping routes a tenant vendor string to an action: link to an
	// existing master brand, create a new master brand on first match, or
	// skip listings under that vendor entirely (used for internal/junk
	// vendors like "Keepstar Store" gift wraps). Keyed by lowercase-trimmed
	// vendor as it appears in raw_attributes.vendor.
	BrandMapping map[string]BrandMappingTarget `json:"brand_mapping,omitempty"`

	// JunkRules tells the applier which listings to skip without ever trying
	// to match them. Captured separately from FieldMapping because the agent
	// builds them by spotting patterns across many listings, not per-field.
	JunkRules *JunkRules `json:"junk_rules,omitempty"`

	// MatchStrategyConfig is the structured form of MatchStrategy: order of
	// strategies, score thresholds for auto-link / needs-review / skip,
	// per-vertical opt-outs (e.g. embedding off for verticals where master
	// has zero coverage). When non-nil, the applier uses this and ignores
	// the legacy MatchStrategy []string for thresholds. The legacy field
	// stays populated for backwards-compat — older code paths still read it.
	MatchStrategyConfig *MatchStrategyConfig `json:"match_strategy_config,omitempty"`
}

// BrandMappingTarget describes what to do with a tenant vendor.
//   action="link_existing": link to MasterBrand (must already exist on master_products.brand).
//   action="create_new":    first listing under this vendor seeds a new master brand.
//                           Subsequent listings link to it.
//   action="skip":          skip ALL listings under this vendor (junk vendor).
type BrandMappingTarget struct {
	Action      string `json:"action"`                  // "link_existing" | "create_new" | "skip"
	MasterBrand string `json:"master_brand,omitempty"`  // required for action="link_existing"
	Reason      string `json:"reason,omitempty"`        // for action="skip", human-readable
}

// JunkRules are pattern-level skip filters. The applier ANDs the rules with
// the existing junk_detector heuristics (M4b) — if either fires, the listing
// is skipped. Curator can edit these rules in the schema review UI.
type JunkRules struct {
	// VendorBlacklist — exact-match (case-insensitive) lowercase vendor strings.
	VendorBlacklist []string `json:"vendor_blacklist,omitempty"`
	// AxisNamePatterns — substring patterns matched against variant.options keys
	// (e.g. "gift wrap", "warranty", "engraving"). Case-insensitive.
	AxisNamePatterns []string `json:"axis_name_patterns,omitempty"`
	// RequireIdentifier — when true, listings with no SKU AND no GTIN are skipped.
	// Most production catalogs benefit from this; defaults to true if JunkRules
	// is non-nil but field is missing (i.e. the agent omitted it).
	RequireIdentifier bool `json:"require_identifier,omitempty"`
}

// MatchStrategyConfig is the structured replacement for the legacy
// []string match_strategy. The applier follows Order top-down; first
// strategy that returns a match above AutoLinkThreshold wins.
type MatchStrategyConfig struct {
	Order                []string `json:"order"` // e.g. ["gtin_exact","vendor_sku_exact","fuzzy_title","embedding"]
	AutoLinkThreshold    float64  `json:"auto_link_threshold"`     // ≥ this → auto-link (default 0.90)
	NeedsReviewThreshold float64  `json:"needs_review_threshold"`  // [needs_review_threshold, auto_link_threshold) → needs_review (default 0.50)
	SkipBelow            float64  `json:"skip_below"`              // < this → don't even propose; treat as no_match (default 0.30)
	// EmbeddingDisabledFor: skip the embedding strategy entirely for these
	// verticals (e.g. furniture/footwear when master has zero rows for them
	// → embedding distance is meaningless).
	EmbeddingDisabledFor []string `json:"embedding_disabled_for,omitempty"`
}

// MasterTemplateProposal is the discovery agent's design for a new vertical's
// master schema. Lives inside MappingArtifact.MasterTemplates; not promoted
// to its own table — the curator turns these into real columns when they
// promote attribute candidates in M11.
type MasterTemplateProposal struct {
	Vertical      string   `json:"vertical"`        // slug, e.g. "furniture"
	Description   string   `json:"description"`     // why this is a separate vertical
	MasterFields  []string `json:"master_fields"`   // shared across variants
	VariantFields []string `json:"variant_fields"`  // vary per SKU
	CategoryHints []string `json:"category_hints,omitempty"`
}

// TenantCatalogSchema is the persisted wrapper around MappingArtifact —
// one row per tenant in catalog.tenant_catalog_schema.
type TenantCatalogSchema struct {
	TenantID         string                `json:"tenantId"`
	MappingArtifact  MappingArtifact       `json:"mappingArtifact"`
	ArtifactVersion  int                   `json:"artifactVersion"`
	Status           MappingArtifactStatus `json:"status"`
	DiscoveredAt     time.Time             `json:"discoveredAt"`
	ValidatedAt      *time.Time            `json:"validatedAt,omitempty"`
	ReDiscoverAfter  *time.Time            `json:"reDiscoverAfter,omitempty"`
}
