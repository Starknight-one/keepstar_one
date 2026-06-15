package usecases

import (
	"context"
	"fmt"
	"sync"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/prompts"
)

// PromptCache memoises the assembled Agent2 system prompt per tenant slug.
// The prompt body is mostly static (`prompts.Agent2SystemPrompt` is a Go
// constant), but the appended <fields> block requires a DB read against
// FieldDefinitionPort. Caching avoids that read on every Agent2 turn for
// tenants we've already served.
//
// Cache key: tenant slug (or UUID — adapter handles both) PLUS the tenant's
// price/stock visibility flags. The flags are part of the key because the
// assembled <fields> block omits hidden price/stock fields — without the flags
// in the key a visibility toggle would keep serving the stale (leaky) prompt.
// Value: the fully-assembled prompt + a built-at timestamp for observability.
//
// Invalidation: Invalidate(tenant) drops every cached entry for a tenant (all
// flag variants). The curator cockpit calls POST
// /api/v1/internal/presets/cache-invalidate after a tenant publishes a preset
// so the next turn rebuilds the <tenant_design_context> block.
type PromptCache struct {
	fdPort     ports.FieldDefinitionPort
	presetPort ports.PresetPort
	catalog    ports.CatalogPort
	store      sync.Map // cacheKey → *promptCacheEntry
	entityType domain.EntityType
}

type promptCacheEntry struct {
	prompt  string
	builtAt time.Time
}

// NewPromptCache constructs a cache. entityType usually "product". catalog is
// used to read the tenant's price/stock visibility flags so the cached
// <fields> block omits hidden fields (and the cache key reflects them).
func NewPromptCache(fdPort ports.FieldDefinitionPort, presetPort ports.PresetPort, catalog ports.CatalogPort, entityType domain.EntityType) *PromptCache {
	return &PromptCache{fdPort: fdPort, presetPort: presetPort, catalog: catalog, entityType: entityType}
}

// GetOrBuild returns the assembled prompt for tenantSlugOrID, building it
// (and caching) on first miss. Build errors propagate — callers that
// can't proceed without a prompt should treat them as fatal for the turn.
//
// `sampleLimit` is forwarded to FormatFieldsBlock; pass 3 to match V4.
func (c *PromptCache) GetOrBuild(ctx context.Context, tenantSlugOrID string, sampleLimit int) (string, error) {
	// Read the tenant's hide-only price/stock visibility flags. These both
	// gate which fields the <fields> block advertises AND form part of the
	// cache key, so a visibility toggle rebuilds the prompt instead of serving
	// a stale (leaky) one. Default visible: a tenant lookup miss/error leaves
	// both true (fail-open to visible — matches the catalog tool's default).
	priceVisible, stockVisible := true, true
	if c.catalog != nil {
		if tenant, err := c.catalog.GetTenantBySlug(ctx, tenantSlugOrID); err == nil && tenant != nil {
			priceVisible = domain.SettingVisible(tenant.Settings, "priceVisible")
			stockVisible = domain.SettingVisible(tenant.Settings, "stockVisible")
		}
	}

	key := cacheKey(tenantSlugOrID, priceVisible, stockVisible)
	if v, ok := c.store.Load(key); ok {
		return v.(*promptCacheEntry).prompt, nil
	}
	fields, err := prompts.FormatFieldsBlock(ctx, c.fdPort, tenantSlugOrID, c.entityType, sampleLimit, priceVisible, stockVisible)
	if err != nil {
		return "", err
	}
	// Published-preset catalog. Best-effort enrichment — FormatTenantDesignContext
	// fails open to an empty block, so a preset-read error never blocks the turn.
	designContext, err := prompts.FormatTenantDesignContext(ctx, c.presetPort, tenantSlugOrID)
	if err != nil {
		return "", err
	}
	assembled := prompts.AssembleSystemPrompt(prompts.Agent2SystemPrompt, designContext, fields)
	c.store.Store(key, &promptCacheEntry{prompt: assembled, builtAt: time.Now()})
	return assembled, nil
}

// cacheKey combines the tenant identifier with its visibility flags so each
// flag combination caches independently and a toggle never serves a stale
// prompt.
func cacheKey(tenantSlugOrID string, priceVisible, stockVisible bool) string {
	return fmt.Sprintf("%s|p=%t|s=%t", tenantSlugOrID, priceVisible, stockVisible)
}

// Invalidate drops every cached entry for tenantSlugOrID (all flag variants).
// Called when tenant_design_context version bumps (preset publish) — and the
// per-flag cache key means a visibility toggle self-invalidates on next build.
func (c *PromptCache) Invalidate(tenantSlugOrID string) {
	prefix := tenantSlugOrID + "|"
	c.store.Range(func(k, _ any) bool {
		if ks, ok := k.(string); ok && (ks == tenantSlugOrID || len(ks) >= len(prefix) && ks[:len(prefix)] == prefix) {
			c.store.Delete(k)
		}
		return true
	})
}
