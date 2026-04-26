// Package domain — minimal value types for curator. Mirrors a small subset
// of admin/domain (AttributeCandidate, JunkCandidate, AuditEntry) — we only
// need the read-shape for curator UI; admin owns the canonical types.
package domain

import "time"

type CuratorUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

type AttributeCandidate struct {
	ID               string    `json:"id"`
	Key              string    `json:"key"`
	Vertical         string    `json:"vertical"`
	SeenInTenants    int       `json:"seenInTenants"`
	SampleValues     []string  `json:"sampleValues"`
	ProposedType     string    `json:"proposedType,omitempty"`
	AgentMeta        string    `json:"agentMeta,omitempty"`
	Status           string    `json:"status"`
	PromotedToColumn string    `json:"promotedToColumn,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}

type CategoryCandidate struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ProposedParent string    `json:"proposedParent,omitempty"`
	SeenInTenants  int       `json:"seenInTenants"`
	Vertical       string    `json:"vertical"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
}

type JunkCandidate struct {
	ID             string                 `json:"id"`
	TenantID       string                 `json:"tenantId"`
	ListingID      string                 `json:"listingId"`
	DetectedReason map[string]interface{} `json:"detectedReason"`
	Classification string                 `json:"classification"`
	ClassifiedBy   string                 `json:"classifiedBy,omitempty"`
	CreatedAt      time.Time              `json:"createdAt"`
}

type AuditEntry struct {
	ID            int64                  `json:"id"`
	TenantID      string                 `json:"tenantId,omitempty"`
	ActorKind     string                 `json:"actorKind"`
	ActorID       string                 `json:"actorId,omitempty"`
	EntityKind    string                 `json:"entityKind"`
	EntityID      string                 `json:"entityId"`
	Action        string                 `json:"action"`
	FieldChanges  map[string]interface{} `json:"fieldChanges,omitempty"`
	AggregateMeta map[string]interface{} `json:"aggregateMeta,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
}
