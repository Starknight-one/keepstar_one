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

// MappingArtifact is the declarative output of the agent normalizer (spec §4.3).
// One LLM call per tenant produces this; subsequent imports apply it via plain
// code with no LLM in the hot path.
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
	// vs. variants, what categories to expect. The harvester (4d) creates
	// masters according to these templates; curator promotes Tier 2 columns
	// (e.g. master_furniture) in M11.
	MasterTemplates []MasterTemplateProposal `json:"master_templates,omitempty"`
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
