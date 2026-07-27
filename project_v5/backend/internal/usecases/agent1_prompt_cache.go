package usecases

import (
	"context"
	"strings"
	"sync"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/prompts"
)

// Agent1PromptCache memoises the assembled Agent1 system prompt per
// (form, tenant). The prompt body is a per-form base prompt (storefront's is
// the static `prompts.Agent1SystemPrompt`; other forms register theirs via
// SetFormPrompt — the R17 prompt-selection seam) plus a per-tenant <catalog>
// digest block (CatalogPort.BuildCatalogDigest). The digest is the expensive
// part — multiple aggregate SQL queries — so caching here avoids re-running
// them on every Agent1 turn.
//
// Cache key: "<mode>|<tenant slug>" — a form is data (more than three will
// exist), so the mode string participates in the key rather than being an
// enum switch.
// Value: the fully-assembled prompt + built-at timestamp.
//
// TTL safety net (RUNTIME_SPEC.md §6.1): entries older than CACHE_TTL_SECONDS
// (default 600s) are treated as a miss, healing any missed best-effort
// invalidation within one TTL.
//
// Failure mode: any error fetching tenant or digest returns the base prompt
// unchanged (fail-open).
type Agent1PromptCache struct {
	catalog ports.CatalogPort
	store   sync.Map // "mode|tenantSlug" → *agent1PromptCacheEntry
	ttl     time.Duration

	// formPrompts maps a form's storage key (mode) to its base system
	// prompt. Registered at boot via SetFormPrompt, before serving —
	// the map is not guarded for concurrent mutation. Unregistered forms
	// fall back to the storefront base prompt.
	formPrompts map[domain.PipelineMode]string
}

type agent1PromptCacheEntry struct {
	prompt  string
	builtAt time.Time
}

// NewAgent1PromptCache constructs a cache backed by the given catalog port.
// TTL comes from CACHE_TTL_SECONDS (default 600s, §6.1).
func NewAgent1PromptCache(catalog ports.CatalogPort) *Agent1PromptCache {
	return &Agent1PromptCache{
		catalog:     catalog,
		ttl:         cacheTTLFromEnv(),
		formPrompts: make(map[domain.PipelineMode]string),
	}
}

// SetFormPrompt registers the base system prompt for a form (R17 prompt
// selection seam — the onboarding workstream registers its prompt here in
// M3). Boot-time only: call before the server starts serving turns.
func (c *Agent1PromptCache) SetFormPrompt(mode domain.PipelineMode, prompt string) {
	c.formPrompts[mode] = prompt
}

// basePrompt returns the registered base prompt for mode, falling back to
// the storefront prompt for unregistered forms.
func (c *Agent1PromptCache) basePrompt(mode domain.PipelineMode) string {
	if p, ok := c.formPrompts[mode]; ok && p != "" {
		return p
	}
	return prompts.Agent1SystemPrompt
}

// GetOrBuild returns the cached storefront-form prompt for tenantSlug —
// the legacy single-form entry point, kept so pre-mode call sites behave
// byte-identically. New callers pass the session's mode via GetOrBuildForm.
func (c *Agent1PromptCache) GetOrBuild(ctx context.Context, tenantSlug string) string {
	return c.GetOrBuildForm(ctx, tenantSlug, domain.ModeStorefront)
}

// GetOrBuildForm returns the cached system prompt for (mode, tenantSlug),
// building it on first miss or after TTL expiry. On any error during build,
// returns the form's base prompt (fail-open, not cached).
func (c *Agent1PromptCache) GetOrBuildForm(ctx context.Context, tenantSlug string, mode domain.PipelineMode) string {
	if mode == "" {
		mode = domain.ModeStorefront
	}
	if tenantSlug == "" {
		return c.basePrompt(mode)
	}
	key := string(mode) + "|" + tenantSlug
	if v, ok := c.store.Load(key); ok {
		e := v.(*agent1PromptCacheEntry)
		if c.ttl <= 0 || time.Since(e.builtAt) <= c.ttl {
			return e.prompt
		}
		// TTL safety net (§6.1): stale entry = miss.
		c.store.Delete(key)
	}
	return c.build(ctx, key, tenantSlug, mode).prompt
}

// build assembles + caches the prompt. On any error it returns (and does
// NOT cache) the fail-open default.
//
// The per-tenant catalog_search InputSchema no longer lives here: the
// operation registry materializes it via the catalog_search executor's
// SpecForTenant (operations.WrapCatalogSearch — the same digest-derived
// schema, cached per tenant in the registry's spec cache, R15 scope=agent1).
func (c *Agent1PromptCache) build(ctx context.Context, key, tenantSlug string, mode domain.PipelineMode) *agent1PromptCacheEntry {
	base := c.basePrompt(mode)
	failOpen := &agent1PromptCacheEntry{prompt: base}

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
		prompt:  base + "\n\n<catalog>\n" + digestText + "</catalog>\n",
		builtAt: time.Now(),
	}
	c.store.Store(key, entry)
	return entry
}

// Invalidate drops the cached entries for tenantSlug across ALL forms.
// Called by the scoped cache-invalidation endpoint (R15 scope=agent1,
// handler_cache.go) when the tenant's catalog has changed.
func (c *Agent1PromptCache) Invalidate(tenantSlug string) {
	suffix := "|" + tenantSlug
	c.store.Range(func(k, _ any) bool {
		if ks, ok := k.(string); ok && strings.HasSuffix(ks, suffix) {
			c.store.Delete(k)
		}
		return true
	})
}
