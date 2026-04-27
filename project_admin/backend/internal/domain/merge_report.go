package domain

import "time"

// MergeReportStatus tracks the lifecycle of a single merge report.
type MergeReportStatus string

const (
	MergeReportStatusPending  MergeReportStatus = "pending"
	MergeReportStatusReviewed MergeReportStatus = "reviewed"
	MergeReportStatusApplied  MergeReportStatus = "applied"
	MergeReportStatusPartial  MergeReportStatus = "partial"   // some proposals applied, some rejected
	MergeReportStatusReverted MergeReportStatus = "reverted"  // an applied report was rolled back
	MergeReportStatusSuperseded MergeReportStatus = "superseded" // newer report replaces this one
)

// MergeProposalAction is what the applier proposes to do with one listing.
type MergeProposalAction string

const (
	// MergeActionLinkExisting links the listing to an existing master_variant.
	MergeActionLinkExisting MergeProposalAction = "link_existing"
	// MergeActionNewMaster creates a new master_product + variant from this listing.
	MergeActionNewMaster MergeProposalAction = "new_master"
	// MergeActionVariantOfExisting creates a new variant on an existing master_product.
	MergeActionVariantOfExisting MergeProposalAction = "variant_of_existing"
	// MergeActionNeedsReview signals the score band sat between auto-link and skip.
	MergeActionNeedsReview MergeProposalAction = "needs_review"
	// MergeActionSkip means JunkRules or "no identifier" rejected the listing.
	MergeActionSkip MergeProposalAction = "skip"
	// MergeActionAlreadyLinked means listing.master_variant_id or listing.master_product_id
	// is already set (curator merged it earlier, OR legacy importer pre-linked). The
	// applier MUST NOT propose to re-merge — emitting AlreadyLinked is a no-op marker
	// that keeps the proposal traceable in the report without affecting active counters.
	MergeActionAlreadyLinked MergeProposalAction = "already_linked"
)

// MergeProposalStatus tracks per-proposal lifecycle within a report.
type MergeProposalStatus string

const (
	MergeProposalStatusPending  MergeProposalStatus = "pending"
	MergeProposalStatusApproved MergeProposalStatus = "approved"
	MergeProposalStatusRejected MergeProposalStatus = "rejected"
	MergeProposalStatusApplied  MergeProposalStatus = "applied"
	MergeProposalStatusFailed   MergeProposalStatus = "failed"
)

// MergeProposal is the per-listing decision the applier emits. Each proposal
// is a self-contained "what to do with this listing" — applied independently
// (one transaction per proposal at execute time, so partial-apply is natural).
type MergeProposal struct {
	ID            string                  `json:"id"` // surrogate, not in DB until report saved
	ListingID     string                  `json:"listingId"`
	ListingName   string                  `json:"listingName"`
	ListingVendor string                  `json:"listingVendor,omitempty"`
	Action        MergeProposalAction     `json:"action"`
	Status        MergeProposalStatus     `json:"status"`

	// Target filled when Action ∈ {link_existing, variant_of_existing}.
	TargetMasterProductID string `json:"targetMasterProductId,omitempty"`
	TargetMasterVariantID string `json:"targetMasterVariantId,omitempty"`

	// ProposedMaster filled when Action = new_master. Carries the shape the
	// applier would create (name, brand, vertical, tier1/tier2, variant info).
	// Curator can edit fields here in the review UI before approving.
	ProposedMaster *ProposedMaster `json:"proposedMaster,omitempty"`

	// ProposedVariant filled when Action = variant_of_existing.
	ProposedVariant *ProposedVariant `json:"proposedVariant,omitempty"`

	// FieldDecisions is the per-field plan when Action ∈ {link_existing,
	// variant_of_existing}: for each tier-2 / candidate-eligible field, what
	// to do with the value (inherit master / propagate listing → master /
	// keep listing override / skip).
	FieldDecisions []FieldDecision `json:"fieldDecisions,omitempty"`

	// MatchEvidence carries the cascade outcome that justified Action.
	MatchEvidence *MatchEvidence `json:"matchEvidence,omitempty"`

	// SkipReason is set when Action = skip — references a rule from JunkRules
	// or one of the canned reasons ("no_identifier", "vendor_skip", "junk_axis").
	SkipReason string `json:"skipReason,omitempty"`

	// RollbackData is populated AFTER the proposal applies — snapshot of the
	// pre-state so a Revert can undo. Empty until status=applied.
	RollbackData map[string]interface{} `json:"rollbackData,omitempty"`
}

// ProposedMaster is the shape of a new master_product the applier would create.
type ProposedMaster struct {
	Name        string                 `json:"name"`
	Brand       string                 `json:"brand"`
	Vertical    string                 `json:"vertical"`
	Description string                 `json:"description,omitempty"`
	ImageURL    string                 `json:"imageUrl,omitempty"`
	Tier2       map[string]interface{} `json:"tier2,omitempty"`
	Variant     ProposedVariant        `json:"variant"`
}

// ProposedVariant is the shape of a new master_variant.
type ProposedVariant struct {
	SKU      string            `json:"sku,omitempty"`
	GTINs    []string          `json:"gtins,omitempty"`
	WeightG  *int              `json:"weightG,omitempty"`
	VolumeML *int              `json:"volumeMl,omitempty"`
	Color    string            `json:"color,omitempty"`
	Size     string            `json:"size,omitempty"`
	Material string            `json:"material,omitempty"`
	Axes     map[string]string `json:"axes,omitempty"`
	ImageURL string            `json:"imageUrl,omitempty"`
}

// FieldDecision is one row in the per-field plan for a link/variant proposal.
type FieldDecision struct {
	Field        string      `json:"field"`         // dotted path: "tier2.scent", "name", "brand"
	Source       string      `json:"source"`        // "listing" | "master" | "candidate"
	ProposedValue interface{} `json:"proposedValue,omitempty"`
	MasterValue  interface{} `json:"masterValue,omitempty"` // current master value (for "conflict" UX)
	ListingValue interface{} `json:"listingValue,omitempty"`
	Action       string      `json:"action"`        // "inherit" | "propagate_to_master" | "keep_master" | "override_listing" | "skip"
	Reason       string      `json:"reason,omitempty"`
}

// MatchEvidence is the cascade outcome that drove a link/variant decision.
type MatchEvidence struct {
	Strategy   string  `json:"strategy"`            // "gtin_exact" | "vendor_sku_exact" | "fuzzy_title" | "embedding" | "no_match"
	Score      float64 `json:"score"`               // 0..1
	Confidence string  `json:"confidence"`          // "high" | "medium" | "low"
	Reasoning  string  `json:"reasoning,omitempty"` // 1-line explanation
}

// MergeReport is the aggregate produced by one applier run for one tenant.
// One row per run; supersedes prior reports for the tenant. Per-proposal
// status changes (approved/rejected/applied) live in the Proposals JSONB array
// — we don't normalize proposals into a separate table because:
//   1. They're always read together (per-tenant flow)
//   2. The applier writes them once; mutations are bulk approve/reject by curator
type MergeReport struct {
	ID           int64             `json:"id"`
	TenantID     string            `json:"tenantId"`
	GeneratedAt  time.Time         `json:"generatedAt"`
	SchemaVersion int              `json:"schemaVersion"` // matches MappingArtifact.Version at gen time
	Status       MergeReportStatus `json:"status"`

	// AgentMetaSummary captures the artifact's agent_notes + cost breakdown
	// at report-gen time — useful so the curator UI can show "this report
	// was based on a $0.40 / 20-turn discovery from 2 hours ago".
	AgentMetaSummary string `json:"agentMetaSummary,omitempty"`

	// Counters surfaced for one-line UI summaries without re-walking proposals.
	TotalListings      int `json:"totalListings"`
	AutoLinkCount      int `json:"autoLinkCount"`
	NewMasterCount     int `json:"newMasterCount"`
	NeedsReviewCount   int `json:"needsReviewCount"`
	SkipCount          int `json:"skipCount"`
	AlreadyLinkedCount int `json:"alreadyLinkedCount"`

	// Proposals — full per-listing detail. JSONB column.
	Proposals []MergeProposal `json:"proposals"`

	AppliedAt *time.Time `json:"appliedAt,omitempty"`
	AppliedBy string     `json:"appliedBy,omitempty"`
}
