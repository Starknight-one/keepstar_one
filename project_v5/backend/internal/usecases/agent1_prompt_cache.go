package usecases

import (
	"context"
	"sync"
	"time"

	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/prompts"
)

// Agent1PromptCache memoises the assembled Agent1 system prompt per tenant.
// The prompt body is the static `prompts.Agent1SystemPrompt` plus a per-tenant
// <catalog> digest block (CatalogPort.BuildCatalogDigest). The digest is the
// expensive part — multiple aggregate SQL queries — so caching here avoids
// re-running them on every Agent1 turn.
//
// Cache key: tenant slug.
// Value: the fully-assembled prompt + built-at timestamp.
//
// Failure mode: any error fetching tenant or digest returns the base prompt
// unchanged (fail-open).
type Agent1PromptCache struct {
	catalog ports.CatalogPort
	store   sync.Map // tenantSlug → *agent1PromptCacheEntry
}

type agent1PromptCacheEntry struct {
	prompt  string
	builtAt time.Time
}

// NewAgent1PromptCache constructs a cache backed by the given catalog port.
func NewAgent1PromptCache(catalog ports.CatalogPort) *Agent1PromptCache {
	return &Agent1PromptCache{catalog: catalog}
}

// GetOrBuild returns the cached system prompt for tenantSlug, building it
// on first miss. On any error during build, returns the base prompt.
func (c *Agent1PromptCache) GetOrBuild(ctx context.Context, tenantSlug string) string {
	if tenantSlug == "" {
		return prompts.Agent1SystemPrompt
	}
	if v, ok := c.store.Load(tenantSlug); ok {
		return v.(*agent1PromptCacheEntry).prompt
	}

	tenant, err := c.catalog.GetTenantBySlug(ctx, tenantSlug)
	if err != nil || tenant == nil {
		return prompts.Agent1SystemPrompt
	}
	digest, err := c.catalog.BuildCatalogDigest(ctx, tenant.ID)
	if err != nil || digest == nil {
		return prompts.Agent1SystemPrompt
	}
	digestText := digest.ToPromptText()
	if digestText == "" {
		return prompts.Agent1SystemPrompt
	}

	full := prompts.Agent1SystemPrompt + "\n\n<catalog>\n" + digestText + "</catalog>\n"
	c.store.Store(tenantSlug, &agent1PromptCacheEntry{prompt: full, builtAt: time.Now()})
	return full
}

// Invalidate drops the cached entry for tenantSlug. Use when the tenant's
// catalog has changed (admin push) — call from a future invalidation hook.
func (c *Agent1PromptCache) Invalidate(tenantSlug string) {
	c.store.Delete(tenantSlug)
}
