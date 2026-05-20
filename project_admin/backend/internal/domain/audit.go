package domain

import "time"

// ActorKind identifies who performed the audited action.
type ActorKind string

const (
	ActorKindUser    ActorKind = "user"
	ActorKindSystem  ActorKind = "system"
	ActorKindAgent   ActorKind = "agent"
	ActorKindCurator ActorKind = "curator"
	ActorKindAPI     ActorKind = "api"
)

// EntityKind identifies what was audited.
type EntityKind string

const (
	EntityKindListing       EntityKind = "listing"
	EntityKindMasterProduct EntityKind = "master_product"
	EntityKindMasterVariant EntityKind = "master_variant"
	EntityKindCategory      EntityKind = "category"
	EntityKindCandidate     EntityKind = "candidate"
	EntityKindAPIKey        EntityKind = "api_key"
)

const (
	AuditActionClassify AuditAction = "classify"
)

// AuditAction is what happened to the entity.
type AuditAction string

const (
	AuditActionCreate      AuditAction = "create"
	AuditActionUpdate      AuditAction = "update"
	AuditActionDelete      AuditAction = "delete"
	AuditActionRename      AuditAction = "rename"
	AuditActionPromote     AuditAction = "promote"
	AuditActionMerge       AuditAction = "merge"
	AuditActionMergeRevert AuditAction = "merge_revert"
	// Approval lifecycle actions for the pending-approval staging flow.
	// AuditActionApprove fires when curator approves a pending master or a
	// pending field-level change; AuditActionReject when they reject;
	// AuditActionExpire is emitted by the sweeper that transitions
	// 30-day-stale pending rows to status='expired'.
	AuditActionApprove AuditAction = "approve"
	AuditActionReject  AuditAction = "reject"
	AuditActionExpire  AuditAction = "expire"
)

// FieldChange records a single field's old → new transition. Used in
// human-action audit entries (per-field diff).
type FieldChange struct {
	Old interface{} `json:"old"`
	New interface{} `json:"new"`
}

// AuditEntry is one row in catalog.audit_log. Spec §3.7:
//   - System bulk ops (importer updated 7234 prices) → ONE row with
//     AggregateMeta {batch_id, count, source}.
//   - Human edits (user renamed product) → detailed row with FieldChanges.
type AuditEntry struct {
	ID            int64                  `json:"id"`
	TenantID      string                 `json:"tenantId,omitempty"` // NULL for system-wide actions
	ActorKind     ActorKind              `json:"actorKind"`
	ActorID       string                 `json:"actorId,omitempty"` // user.id, "agent:harvester", etc.
	EntityKind    EntityKind             `json:"entityKind"`
	EntityID      string                 `json:"entityId"`
	Action        AuditAction            `json:"action"`
	FieldChanges  map[string]FieldChange `json:"fieldChanges,omitempty"`
	AggregateMeta map[string]interface{} `json:"aggregateMeta,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
}
