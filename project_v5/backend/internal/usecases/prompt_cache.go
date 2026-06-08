package usecases

import (
	"context"
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
// Cache key: tenant slug (or UUID — adapter handles both).
// Value: the fully-assembled prompt + a built-at timestamp for observability.
//
// Invalidation: Invalidate(tenant) drops a tenant's cached entry. The
// curator cockpit calls POST /api/v1/internal/presets/cache-invalidate
// after a tenant publishes a preset so the next turn rebuilds the
// <tenant_design_context> block.
type PromptCache struct {
	fdPort     ports.FieldDefinitionPort
	presetPort ports.PresetPort
	store      sync.Map // tenantKey → *promptCacheEntry
	entityType domain.EntityType
}

type promptCacheEntry struct {
	prompt  string
	builtAt time.Time
}

// NewPromptCache constructs a cache. entityType usually "product".
func NewPromptCache(fdPort ports.FieldDefinitionPort, presetPort ports.PresetPort, entityType domain.EntityType) *PromptCache {
	return &PromptCache{fdPort: fdPort, presetPort: presetPort, entityType: entityType}
}

// GetOrBuild returns the assembled prompt for tenantSlugOrID, building it
// (and caching) on first miss. Build errors propagate — callers that
// can't proceed without a prompt should treat them as fatal for the turn.
//
// `sampleLimit` is forwarded to FormatFieldsBlock; pass 3 to match V4.
func (c *PromptCache) GetOrBuild(ctx context.Context, tenantSlugOrID string, sampleLimit int) (string, error) {
	if v, ok := c.store.Load(tenantSlugOrID); ok {
		return v.(*promptCacheEntry).prompt, nil
	}
	fields, err := prompts.FormatFieldsBlock(ctx, c.fdPort, tenantSlugOrID, c.entityType, sampleLimit)
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
	c.store.Store(tenantSlugOrID, &promptCacheEntry{prompt: assembled, builtAt: time.Now()})
	return assembled, nil
}

// Invalidate drops the cached entry for tenantSlugOrID. No-op when absent.
// Will be called from chunk 6c+ when tenant_design_context version bumps.
func (c *PromptCache) Invalidate(tenantSlugOrID string) {
	c.store.Delete(tenantSlugOrID)
}
