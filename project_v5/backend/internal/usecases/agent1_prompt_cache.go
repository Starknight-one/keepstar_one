package usecases

import (
	"context"
	"sync"
	"time"

	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/prompts"
	"keepstar_v5/internal/tools"
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
	prompt string
	// searchSchema is the per-tenant catalog_search InputSchema derived
	// from the same digest (nil → keep the tool's static legacy schema).
	searchSchema map[string]interface{}
	builtAt      time.Time
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
	return c.build(ctx, tenantSlug).prompt
}

// SearchSchema returns the per-tenant catalog_search InputSchema (nil →
// caller keeps the static legacy schema). Built from the same cached digest
// as the system prompt.
func (c *Agent1PromptCache) SearchSchema(ctx context.Context, tenantSlug string) map[string]interface{} {
	if tenantSlug == "" {
		return nil
	}
	if v, ok := c.store.Load(tenantSlug); ok {
		return v.(*agent1PromptCacheEntry).searchSchema
	}
	return c.build(ctx, tenantSlug).searchSchema
}

// build assembles + caches the prompt and per-tenant search schema. On any
// error it returns (and does NOT cache) the fail-open defaults.
func (c *Agent1PromptCache) build(ctx context.Context, tenantSlug string) *agent1PromptCacheEntry {
	failOpen := &agent1PromptCacheEntry{prompt: prompts.Agent1SystemPrompt}

	tenant, err := c.catalog.GetTenantBySlug(ctx, tenantSlug)
	if err != nil || tenant == nil {
		return failOpen
	}
	digest, err := c.catalog.BuildCatalogDigest(ctx, tenant.ID)
	if err != nil || digest == nil {
		return failOpen
	}
	digestText := digest.ToPromptText()
	if digestText == "" {
		return failOpen
	}

	entry := &agent1PromptCacheEntry{
		prompt:       prompts.Agent1SystemPrompt + "\n\n<catalog>\n" + digestText + "</catalog>\n",
		searchSchema: tools.CatalogSearchSchemaForDigest(digest),
		builtAt:      time.Now(),
	}
	c.store.Store(tenantSlug, entry)
	return entry
}

// Invalidate drops the cached entry for tenantSlug. Use when the tenant's
// catalog has changed (admin push) — call from a future invalidation hook.
func (c *Agent1PromptCache) Invalidate(tenantSlug string) {
	c.store.Delete(tenantSlug)
}
