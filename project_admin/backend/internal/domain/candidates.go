package domain

import "time"

// CandidateStatus tracks lifecycle of staged attributes/categories
// before promotion to typed columns.
type CandidateStatus string

const (
	CandidateStatusPending   CandidateStatus = "pending"
	CandidateStatusPromoted  CandidateStatus = "promoted"
	CandidateStatusDismissed CandidateStatus = "dismissed"
	CandidateStatusMerged    CandidateStatus = "merged"
)

// AttributeCandidate is a tenant-imported field that doesn't yet exist as a
// typed column on any master schema. Curator promotes it when seen_in_tenants
// >= 2 (spec §1.5, §4.5). Embedding includes candidate values so search works
// before promotion.
type AttributeCandidate struct {
	ID                string            `json:"id"`
	Key               string            `json:"key"`
	Vertical          string            `json:"vertical"`
	SeenInTenants     int               `json:"seenInTenants"`
	SampleValues      []string          `json:"sampleValues"` // small JSON array, just for curator's eyes
	ProposedType      string            `json:"proposedType,omitempty"` // text|number|enum|bool|array
	AgentMeta         string            `json:"agentMeta,omitempty"`
	Embedding         []float32         `json:"-"` // never serialized
	Status            CandidateStatus   `json:"status"`
	MergedIntoKey     string            `json:"mergedIntoKey,omitempty"`
	PromotedToColumn  string            `json:"promotedToColumn,omitempty"` // e.g. "master_cosmetics.scent"
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
}

// CategoryCandidate is a new master category (taxonomy node) suggested by
// tenant data, awaiting curator promotion to master_categories.
type CategoryCandidate struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	ProposedParent string          `json:"proposedParent,omitempty"` // path in master taxonomy
	SeenInTenants  int             `json:"seenInTenants"`
	Vertical       string          `json:"vertical"`
	Status         CandidateStatus `json:"status"`
	PromotedToID   string          `json:"promotedToId,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
}

// JunkClassification of a detected addon/upsell variant.
type JunkClassification string

const (
	JunkClassificationPending        JunkClassification = "pending"
	JunkClassificationConfirmedAddon JunkClassification = "confirmed_addon"
	JunkClassificationFalsePositive  JunkClassification = "false_positive"
)

// JunkCandidate is a variant that the heuristic flagged as a likely addon
// (gift wrap, engraving, warranty). Spec §4.7. Curator confirms or rejects.
type JunkCandidate struct {
	ID              string                 `json:"id"`
	TenantID        string                 `json:"tenantId"`
	ListingID       string                 `json:"listingId"`
	MasterVariantID string                 `json:"masterVariantId,omitempty"`
	DetectedReason  map[string]interface{} `json:"detectedReason"` // {"axis_name":"gift wrap","price_delta":5}
	Classification  JunkClassification     `json:"classification"`
	ClassifiedAt    *time.Time             `json:"classifiedAt,omitempty"`
	ClassifiedBy    string                 `json:"classifiedBy,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
}
