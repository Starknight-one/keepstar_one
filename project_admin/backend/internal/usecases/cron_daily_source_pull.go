// Package usecases — CronDailySourcePull is the daily background pass
// that re-syncs every tenant with the catalog flow. It is the third leg
// of the data-freshness tripod alongside webhooks (push) and manual
// curator triggers; cron covers the "tenant changed something at the
// source while no webhook fired" case.
//
// MVP scope: the runner calls UpdateOrchestrator.ManualSync per active
// tenant. ManualSync re-runs apply_v2 against whatever sits in the inbox
// (filled by webhooks since the last apply), and triggers narrow
// discovery on any mapping_miss. A future iteration will also pull
// directly from the source API (Shopify Bulk Operation, Sheets fetch,
// Amazon SP-API) — that work is followup, not in this milestone, because
// each source brings its own async / paging story.
package usecases

import (
	"context"
	"fmt"
	"time"

	"keepstar-admin/internal/logger"
)

// CronDailySourcePull is the runnable; main.go wraps it in a time.Ticker
// when KEEPSTAR_DAILY_SOURCE_PULL=1.
type CronDailySourcePull struct {
	orchestrator *UpdateOrchestrator
	listTenants  ListActiveTenantsFunc
	log          *logger.Logger
}

// ListActiveTenantsFunc returns the IDs of tenants that should be polled.
// Provided as a callback so the runner doesn't need to know the storage
// shape — main.go injects a closure backed by a SQL query.
type ListActiveTenantsFunc func(ctx context.Context) ([]string, error)

func NewCronDailySourcePull(orchestrator *UpdateOrchestrator, listTenants ListActiveTenantsFunc, log *logger.Logger) *CronDailySourcePull {
	return &CronDailySourcePull{
		orchestrator: orchestrator,
		listTenants:  listTenants,
		log:          log,
	}
}

// RunOnce iterates active tenants and calls ManualSync per tenant. Errors
// per tenant are logged but do not abort the pass — one bad tenant must
// not block the rest.
func (r *CronDailySourcePull) RunOnce(ctx context.Context) error {
	if r == nil || r.orchestrator == nil || r.listTenants == nil {
		return fmt.Errorf("cron daily source pull: not fully wired")
	}
	tenants, err := r.listTenants(ctx)
	if err != nil {
		return fmt.Errorf("cron daily source pull: list tenants: %w", err)
	}
	start := time.Now()
	ok, failed := 0, 0
	for _, tid := range tenants {
		if tid == "" {
			continue
		}
		tCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		_, syncErr := r.orchestrator.ManualSync(tCtx, tid)
		cancel()
		if syncErr != nil {
			failed++
			r.log.Warn("cron_daily_source_pull_tenant_failed", "tenant", tid, "error", syncErr)
			continue
		}
		ok++
	}
	r.log.Info("cron_daily_source_pull_done",
		"tenants_total", len(tenants),
		"ok", ok,
		"failed", failed,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}
