package operations

import (
	"context"
	"sync"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// specCache memoises per-tenant operation resolution: the resolved tenant
// row, the enabled instances, and lazily-materialized per-tenant specs
// (Executor.SpecForTenant — the CatalogSearchSchemaForDigest pattern
// generalized). One entry per tenant slug.
//
// TTL safety net (§6.1): entries older than ttl are treated as a miss, so
// a missed best-effort invalidation heals within CACHE_TTL_SECONDS.
// InvalidateTenant (R15 scope=agent1) drops the entry immediately.
type specCache struct {
	mu      sync.Mutex
	entries map[string]*tenantEntry
	ttl     time.Duration
}

func newSpecCache(ttl time.Duration) *specCache {
	if ttl <= 0 {
		ttl = defaultSpecTTL
	}
	return &specCache{entries: make(map[string]*tenantEntry), ttl: ttl}
}

// get returns a live entry or nil (miss / expired).
func (c *specCache) get(tenantSlug string) *tenantEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[tenantSlug]
	if !ok {
		return nil
	}
	if time.Since(e.builtAt) > c.ttl {
		delete(c.entries, tenantSlug)
		return nil
	}
	return e
}

func (c *specCache) put(tenantSlug string, e *tenantEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[tenantSlug] = e
}

func (c *specCache) invalidate(tenantSlug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, tenantSlug)
}

func (c *specCache) invalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*tenantEntry)
}

// tenantEntry is one tenant's resolved operation surface.
type tenantEntry struct {
	slug      string
	tenant    *domain.Tenant                      // nil when resolution failed → template-only fail-open
	instances map[string]domain.OperationInstance // enabled instances by instance wire name
	builtAt   time.Time

	mu    sync.Mutex
	specs map[string]*domain.OperationSpec // materialized per wire name, lazy
}

// specFor materializes (and memoises) the per-tenant spec for one wire
// name. inst is nil for auto-enabled template operations. Fail-open: any
// SpecForTenant error falls back to the executor's static Template().
func (e *tenantEntry) specFor(ctx context.Context, name string, ex ports.Executor, inst *domain.OperationInstance) *domain.OperationSpec {
	e.mu.Lock()
	if s, ok := e.specs[name]; ok {
		e.mu.Unlock()
		return s
	}
	e.mu.Unlock()

	spec := ex.Template()
	if e.tenant != nil {
		var cfg map[string]any
		if inst != nil {
			cfg = inst.Config
		}
		if s, err := ex.SpecForTenant(ctx, *e.tenant, cfg); err == nil {
			spec = s
		}
	}
	if inst != nil {
		// Instance wire identity: the LLM sees the instance name (e.g.
		// book_showing), the tenant's description when set.
		spec.Name = inst.Name
		if inst.Description != "" {
			spec.Description = inst.Description
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if s, ok := e.specs[name]; ok {
		return s // lost the race — keep the first materialization (byte-stable)
	}
	e.specs[name] = &spec
	return &spec
}
