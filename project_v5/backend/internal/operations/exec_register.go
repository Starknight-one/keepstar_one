package operations

import (
	"net/http"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// EntityExecutorDeps wires the six native executors (§4.2). Writer is the
// entity write path (*usecases.EntityWrite via the EntityWriter interface);
// Embedding and HTTPClient may be nil (FTS-only catalog search, default
// webhook client).
type EntityExecutorDeps struct {
	State         ports.StatePort
	Catalog       ports.CatalogPort
	Entities      ports.EntityPort
	Embedding     ports.EmbeddingPort
	Writer        EntityWriter
	Notifications ports.NotificationStore
	HTTPClient    *http.Client
}

// RegisterEntityExecutors plugs the six executors into the registry under
// their template wire names (boot wiring, main.go). Their seed rows ride
// the same SeedTemplates pass as everything else; none are auto-enabled —
// tenants reach them through v5_tenant_operations instances only (§3.1).
func RegisterEntityExecutors(reg ports.OperationRegistry, d EntityExecutorDeps) {
	reg.RegisterExecutor(domain.KindQuery, NewQueryExecutor(d.State, d.Catalog, d.Entities, d.Embedding))
	reg.RegisterExecutor(domain.KindCreateRecord, NewCreateRecordExecutor(d.Writer))
	reg.RegisterExecutor(domain.KindUpdateRecord, NewUpdateRecordExecutor(d.Writer))
	reg.RegisterExecutor(domain.KindTransitionStatus, NewTransitionStatusExecutor(d.Writer))
	reg.RegisterExecutor(domain.KindScheduleSlot, NewScheduleSlotExecutor(d.Writer))
	reg.RegisterExecutor(domain.KindNotify, NewNotifyExecutor(d.Notifications, d.HTTPClient))
}
