package usecases

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/prompts"
)

// cacheTTLFromEnv returns the §6.1 TTL safety net duration: CACHE_TTL_SECONDS
// env, default 600s. Both v5 prompt caches treat entries older than this as a
// miss, so a missed best-effort invalidation heals within one TTL.
func cacheTTLFromEnv() time.Duration {
	if s := os.Getenv("CACHE_TTL_SECONDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 600 * time.Second
}

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
// flag variants). Admin/curator call POST /api/v1/internal/cache/invalidate
// with scope=agent2 (or the legacy presets/cache-invalidate alias, R15) after
// a tenant publishes a preset so the next turn rebuilds the
// <tenant_design_context> block. Entries also expire after CACHE_TTL_SECONDS
// (§6.1 TTL safety net) so a missed best-effort invalidation self-heals.
type PromptCache struct {
	fdPort     ports.FieldDefinitionPort
	presetPort ports.PresetPort
	catalog    ports.CatalogPort
	store      sync.Map // cacheKey → *promptCacheEntry
	entityType domain.EntityType
	ttl        time.Duration // §6.1 TTL safety net; entries older = miss

	// formPrompts maps a form's storage key (mode) to its base Agent2
	// system prompt (R24: onboarding = base + compose_turn + choreography,
	// crm = base + compose_turn). Registered at boot via SetFormPrompt,
	// before serving — not guarded for concurrent mutation. Unregistered
	// forms fall back to the storefront base prompt, byte-identical to the
	// pre-form path (duty C3).
	formPrompts map[domain.PipelineMode]string
}

type promptCacheEntry struct {
	prompt  string
	builtAt time.Time
}

// NewPromptCache constructs a cache. entityType usually "product". catalog is
// used to read the tenant's price/stock visibility flags so the cached
// <fields> block omits hidden fields (and the cache key reflects them).
func NewPromptCache(fdPort ports.FieldDefinitionPort, presetPort ports.PresetPort, catalog ports.CatalogPort, entityType domain.EntityType) *PromptCache {
	return &PromptCache{fdPort: fdPort, presetPort: presetPort, catalog: catalog, entityType: entityType, ttl: cacheTTLFromEnv(), formPrompts: make(map[domain.PipelineMode]string)}
}

// SetFormPrompt registers the base Agent2 system prompt for a form (R17
// seam, R24 additions — mirrors Agent1PromptCache.SetFormPrompt). Boot-time
// only: call before the server starts serving turns.
func (c *PromptCache) SetFormPrompt(mode domain.PipelineMode, prompt string) {
	c.formPrompts[mode] = prompt
}

// basePrompt returns the registered base prompt for mode, falling back to
// the storefront prompt for unregistered forms.
func (c *PromptCache) basePrompt(mode domain.PipelineMode) string {
	if p, ok := c.formPrompts[mode]; ok && p != "" {
		return p
	}
	return prompts.Agent2SystemPrompt
}

// GetOrBuild returns the assembled storefront-form prompt for
// tenantSlugOrID — the legacy single-form entry point, kept so pre-mode
// call sites behave byte-identically. New callers pass the session's mode
// via GetOrBuildForm.
func (c *PromptCache) GetOrBuild(ctx context.Context, tenantSlugOrID string, sampleLimit int) (string, error) {
	return c.GetOrBuildForm(ctx, tenantSlugOrID, sampleLimit, domain.ModeStorefront)
}

// GetOrBuildForm returns the assembled prompt for (mode, tenantSlugOrID),
// building it (and caching) on first miss. Build errors propagate — callers
// that can't proceed without a prompt should treat them as fatal for the
// turn.
//
// `sampleLimit` is forwarded to FormatFieldsBlock; pass 3 to match V4.
func (c *PromptCache) GetOrBuildForm(ctx context.Context, tenantSlugOrID string, sampleLimit int, mode domain.PipelineMode) (string, error) {
	if mode == "" {
		mode = domain.ModeStorefront
	}
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

	// The mode participates in the key (a form is data — more than three
	// will exist). It is SUFFIXED so the tenant stays the key prefix and
	// Invalidate(tenant)'s prefix scan keeps dropping every form variant.
	key := cacheKey(tenantSlugOrID, priceVisible, stockVisible) + "|m=" + string(mode)
	if v, ok := c.store.Load(key); ok {
		e := v.(*promptCacheEntry)
		if c.ttl <= 0 || time.Since(e.builtAt) <= c.ttl {
			return e.prompt, nil
		}
		// TTL safety net (§6.1): stale entry = miss — rebuild below.
		c.store.Delete(key)
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
	assembled := prompts.AssembleSystemPrompt(c.basePrompt(mode), designContext, fields)
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
