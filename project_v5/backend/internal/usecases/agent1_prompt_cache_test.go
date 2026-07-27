package usecases

import (
	"context"
	"strings"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/prompts"
)

// countingCatalog wraps fakeCatalog (agent1_execute_test.go) counting the
// expensive digest builds — the observable proxy for cache miss vs hit.
type countingCatalog struct {
	*fakeCatalog
	digestCalls int
}

func (c *countingCatalog) BuildCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error) {
	c.digestCalls++
	return c.fakeCatalog.BuildCatalogDigest(ctx, tenantID)
}

func newCountingCatalog() *countingCatalog {
	return &countingCatalog{fakeCatalog: &fakeCatalog{
		tenant: &domain.Tenant{ID: "tnt-1", Slug: "acme"},
		digest: &domain.CatalogDigest{TotalProducts: 100},
	}}
}

// R17 prompt selection seam: the cache keys on (form, tenant); a form
// registered via SetFormPrompt gets its own base prompt, unregistered forms
// fall back to the storefront base. Each (form, tenant) pair caches
// independently — a hit must not rebuild the digest.
func TestAgent1PromptCache_FormKeying(t *testing.T) {
	cat := newCountingCatalog()
	c := NewAgent1PromptCache(cat)
	c.SetFormPrompt(domain.ModeOnboarding, "ONBOARDING BASE")

	sf := c.GetOrBuildForm(context.Background(), "acme", domain.ModeStorefront)
	ob := c.GetOrBuildForm(context.Background(), "acme", domain.ModeOnboarding)

	if !strings.HasPrefix(sf, prompts.Agent1SystemPrompt) {
		t.Errorf("storefront prompt does not start with the storefront base")
	}
	if !strings.HasPrefix(ob, "ONBOARDING BASE") {
		t.Errorf("onboarding prompt does not start with the registered form base")
	}
	if !strings.Contains(sf, "<catalog>") || !strings.Contains(ob, "<catalog>") {
		t.Errorf("catalog digest missing from assembled prompts")
	}
	if cat.digestCalls != 2 {
		t.Fatalf("digest builds = %d, want 2 (one per form)", cat.digestCalls)
	}

	// Second reads are hits: no further digest builds.
	_ = c.GetOrBuildForm(context.Background(), "acme", domain.ModeStorefront)
	_ = c.GetOrBuildForm(context.Background(), "acme", domain.ModeOnboarding)
	if cat.digestCalls != 2 {
		t.Errorf("digest builds after cached reads = %d, want 2", cat.digestCalls)
	}

	// A form with no registered prompt falls back to the storefront base —
	// forms are data, an unknown mode string must not panic or fail closed.
	crm := c.GetOrBuildForm(context.Background(), "acme", domain.ModeCRM)
	if !strings.HasPrefix(crm, prompts.Agent1SystemPrompt) {
		t.Errorf("unregistered form did not fall back to the storefront base")
	}
}

// The legacy single-form entry point must stay byte-identical to the
// storefront form (pre-mode call sites — agent1_execute.go — depend on it).
func TestAgent1PromptCache_LegacyGetOrBuildIsStorefront(t *testing.T) {
	cat := newCountingCatalog()
	c := NewAgent1PromptCache(cat)

	legacy := c.GetOrBuild(context.Background(), "acme")
	form := c.GetOrBuildForm(context.Background(), "acme", domain.ModeStorefront)
	if legacy != form {
		t.Errorf("GetOrBuild != GetOrBuildForm(storefront)")
	}
	if cat.digestCalls != 1 {
		t.Errorf("digest builds = %d, want 1 (same cache entry)", cat.digestCalls)
	}
}

// R15 scope=agent1: Invalidate(tenant) must drop EVERY form's entry for the
// tenant — a stale non-storefront prompt after a catalog change is exactly
// the bug the scoped endpoint exists to prevent.
func TestAgent1PromptCache_InvalidateDropsAllForms(t *testing.T) {
	cat := newCountingCatalog()
	c := NewAgent1PromptCache(cat)
	_ = c.GetOrBuildForm(context.Background(), "acme", domain.ModeStorefront)
	_ = c.GetOrBuildForm(context.Background(), "acme", domain.ModeCRM)

	c.Invalidate("acme")

	_ = c.GetOrBuildForm(context.Background(), "acme", domain.ModeStorefront)
	_ = c.GetOrBuildForm(context.Background(), "acme", domain.ModeCRM)
	if cat.digestCalls != 4 {
		t.Errorf("digest builds = %d, want 4 (2 before + 2 rebuilt after invalidate)", cat.digestCalls)
	}
}

// §6.1 TTL safety net: entries older than the TTL are a miss, so a missed
// best-effort invalidation heals without a restart.
func TestAgent1PromptCache_TTLExpiry(t *testing.T) {
	cat := newCountingCatalog()
	c := NewAgent1PromptCache(cat)
	c.ttl = time.Millisecond

	_ = c.GetOrBuild(context.Background(), "acme")
	time.Sleep(5 * time.Millisecond)
	_ = c.GetOrBuild(context.Background(), "acme")
	if cat.digestCalls != 2 {
		t.Errorf("digest builds = %d, want 2 (TTL-expired entry must rebuild)", cat.digestCalls)
	}
}
