package domain

// In-memory types passed between the discovery-pipeline usecases. Nothing
// here is persisted as-is — the meta-report feeds the discovery agent and
// auto-mapper; match cascade and junk signals feed the harvester directly.
//
// Spec: docs/New features/admin_catalog_design_2026-04-23.md §4.1, §4.6, §4.7.

// FieldStats summarizes one field across the staged catalog. Computed by
// metadata_harvest.go in pure Go; consumed by the discovery agent (as
// describe_field tool output) and by auto_map_tier1.go (as a heuristic).
//
// TopValues is bounded — top-10 by frequency. NumericMin/NumericMax are
// only set when InferredType is "int" or "float". Language is a single best
// guess ("en" / "non-en") — agent uses this to skip non-English fields per
// spec §1.14 (English-only MVP).
type FieldStats struct {
	Name          string         `json:"name"`
	Path          string         `json:"path"`            // dotted path: "variants.options.value"
	OwnerType     string         `json:"ownerType"`       // "product" | "variant" | "metafield" | "collection"
	Frequency     int            `json:"frequency"`       // # products with this field non-empty
	EmptyRate     float64        `json:"emptyRate"`       // 1.0 - (frequency / totalProducts)
	InferredType  string         `json:"inferredType"`    // string | int | float | bool | array | json
	TopValues     []ValueCount   `json:"topValues,omitempty"`
	NumericMin    *float64       `json:"numericMin,omitempty"`
	NumericMax    *float64       `json:"numericMax,omitempty"`
	Language      string         `json:"language,omitempty"` // "en" | "non-en"
	DistinctCount int            `json:"distinctCount"`
}

// ValueCount is one entry in TopValues — the value plus how often it
// appeared. Strings are truncated to 80 chars before storage to keep the
// meta-report compact (spec ~1-2KB target).
type ValueCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// MetaReport is the bounded summary of a tenant's catalog produced before
// the discovery agent runs. The agent reads this as initial context (in the
// system prompt) and queries deeper via tools when it needs more.
//
// TotalProducts and TotalVariants are simple counters from staging. Fields
// is the per-field analysis. Vendors / ProductTypes / Tags come from the
// shop_references staging row. CollectionTree is built from the navigation
// menu when present (spec §4.1 step 4).
type MetaReport struct {
	TenantID       string         `json:"tenantId"`
	TotalProducts  int            `json:"totalProducts"`
	TotalVariants  int            `json:"totalVariants"`
	Fields         []FieldStats   `json:"fields"`
	Vendors        []string       `json:"vendors,omitempty"`
	ProductTypes   []string       `json:"productTypes,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	CollectionTree []CollectionNode `json:"collectionTree,omitempty"`
	MetafieldDefs  []MetafieldDefSummary `json:"metafieldDefs,omitempty"`
}

// CollectionNode is one node in the harvested collection tree. Built from
// the storefront navigation menu first (richest hierarchy), with collection
// handle prefixes as fallback when the merchant doesn't curate a menu.
type CollectionNode struct {
	ExternalID  string           `json:"externalId"`
	Handle      string           `json:"handle"`
	Title       string           `json:"title"`
	Kind        string           `json:"kind"` // category | showcase | promo
	Children    []CollectionNode `json:"children,omitempty"`
}

// MetafieldDefSummary is a compact view of one declared metafield definition,
// grouped under the meta-report so the agent doesn't need a separate tool to
// see what's formally declared vs. ad-hoc.
type MetafieldDefSummary struct {
	OwnerType string `json:"ownerType"` // PRODUCT | PRODUCTVARIANT | COLLECTION
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
}

// MatchConfidence labels the cascade step that produced a match. Higher
// confidence = more deterministic step. unverified = agent created a new
// master because nothing matched.
type MatchConfidence string

const (
	MatchConfidenceGTINExact   MatchConfidence = "gtin_exact"
	MatchConfidenceSKUExact    MatchConfidence = "sku_exact"
	MatchConfidenceFuzzyHigh   MatchConfidence = "fuzzy_high"
	MatchConfidenceEmbedding   MatchConfidence = "embedding_match"
	MatchConfidenceUnverified  MatchConfidence = "unverified"
)

// MatchOutcome is the disposition of a single cascade attempt. Linked +
// MasterVariantID set means we found an existing variant. ReviewQueue=true
// means multiple high-similarity candidates and curator must pick. NewMaster
// means nothing similar enough — caller will create a fresh master.
type MatchOutcome string

const (
	MatchOutcomeLinked      MatchOutcome = "linked"
	MatchOutcomeReviewQueue MatchOutcome = "review_queue"
	MatchOutcomeNewMaster   MatchOutcome = "new_master"
)

// MatchResult is what match_cascade.MatchVariant returns. Caller decides
// whether to write a listing (Linked) or stash for curator (ReviewQueue) or
// create a new master_product+variant pair (NewMaster).
type MatchResult struct {
	Outcome         MatchOutcome    `json:"outcome"`
	MasterVariantID string          `json:"masterVariantId,omitempty"`
	Confidence      MatchConfidence `json:"confidence,omitempty"`
	Reason          string          `json:"reason,omitempty"`           // human-readable e.g. "GTIN exact: 5901234123457"
	Candidates      []MatchCandidate `json:"candidates,omitempty"`      // populated only on review_queue
}

// MatchCandidate is one option presented to the curator when multiple
// variants matched closely. The curator picks one or rejects all.
type MatchCandidate struct {
	MasterVariantID string  `json:"masterVariantId"`
	Reason          string  `json:"reason"`            // why this matched: "gtin", "fuzzy 0.91", "embedding 0.93"
	Score           float64 `json:"score"`             // similarity / cosine
}

// JunkSignal is one heuristic indicator that a variant looks like an addon
// rather than a real product. Two or more signals on the same variant push
// it into tenant_variant_candidates_junk for triage. Spec §4.7.
type JunkSignal string

const (
	JunkSignalAxisNamePattern JunkSignal = "axis_name_pattern"  // /(gift wrap|engraving|warranty|...)/i
	JunkSignalNoIdentifiers   JunkSignal = "no_identifiers"     // no SKU, no GTIN
	JunkSignalNoDimensions    JunkSignal = "no_dimensions"      // no weight, no volume
	JunkSignalSmallPriceDelta JunkSignal = "small_price_delta"  // tiny delta vs cheapest sibling, low absolute price
)
