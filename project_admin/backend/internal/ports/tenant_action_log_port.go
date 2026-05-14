package ports

import (
	"context"
	"encoding/json"
	"time"
)

// TenantActionLogPort writes and reads admin.tenant_action_log — the
// per-tenant timeline of catalog events surfaced in curator's "Action Log"
// tab. Separate from the global audit_log (operator actions across tenants).
//
// Known action values (extend as new events appear; no DB CHECK constraint
// so we keep it free-form):
//   inbox_pull, discovery_start, discovery_done,
//   apply, apply_partial, mapping_miss,
//   webhook_received, manual_sync, disconnect
//
// Status values: ok | error | warning
type TenantActionLogPort interface {
	Log(ctx context.Context, e *TenantActionLogEntry) error
	List(ctx context.Context, tenantID string, limit, offset int) ([]*TenantActionLogEntry, error)
}

type TenantActionLogEntry struct {
	ID         int64
	TenantID   string
	Action     string
	Status     string
	Payload    json.RawMessage
	OccurredAt time.Time
}
