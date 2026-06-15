package usecases

import (
	"context"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// samplingFieldDefPort returns canned price/stock samples (no published defs)
// so the assembled <fields> block carries them unless suppressed. This drives
// the LEAK-1 prompt-cache tests: the block must omit hidden fields, and the
// cache must key on the visibility flags so a toggle rebuilds.
type samplingFieldDefPort struct{}

func (samplingFieldDefPort) ListFieldDefinitions(context.Context, string, domain.EntityType) ([]ports.FieldDefinition, error) {
	return nil, nil
}
func (samplingFieldDefPort) GetFieldDefinition(context.Context, string, domain.EntityType, string) (*ports.FieldDefinition, error) {
	return nil, nil
}
func (samplingFieldDefPort) SampleFieldValues(context.Context, string, domain.EntityType, int) (map[string][]interface{}, error) {
	return map[string][]interface{}{
		"name":          {"COSRX Snail Serum"},
		"price":         {250000},
		"currency":      {"RUB"},
		"stockQuantity": {7},
	}, nil
}

// flagTenantCatalog returns one tenant whose Settings can be swapped between
// reads, so a test can simulate a visibility toggle on the same slug.
type flagTenantCatalog struct {
	settings map[string]any
}

func (c *flagTenantCatalog) GetTenantBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	return &domain.Tenant{ID: "tnt-1", Slug: slug, Settings: c.settings}, nil
}
func (c *flagTenantCatalog) ListProducts(context.Context, string, ports.ProductFilter) ([]domain.Product, int, error) {
	return nil, 0, nil
}
func (c *flagTenantCatalog) GetProduct(context.Context, string, string) (*domain.Product, error) {
	return nil, domain.ErrProductNotFound
}
func (c *flagTenantCatalog) BuildCatalogDigest(context.Context, string) (*domain.CatalogDigest, error) {
	return &domain.CatalogDigest{}, nil
}
func (c *flagTenantCatalog) VectorSearch(context.Context, string, []float32, int, *ports.VectorFilter) ([]domain.Product, error) {
	return nil, nil
}
func (c *flagTenantCatalog) SearchProjection(context.Context, string, []float32, ports.ProductFilter, int) ([]domain.Product, error) {
	return nil, nil
}

// LEAK-1: a tenant that hides price must get a system prompt whose <fields>
// block carries neither the price field nor its real sample value.
func TestPromptCache_HidesPriceWhenInvisible(t *testing.T) {
	cat := &flagTenantCatalog{settings: map[string]any{"priceVisible": false}}
	pc := NewPromptCache(samplingFieldDefPort{}, noopPresetPort{}, cat, "product")

	prompt, err := pc.GetOrBuild(context.Background(), "acme", 3)
	if err != nil {
		t.Fatalf("GetOrBuild: %v", err)
	}
	if strings.Contains(prompt, "250000") {
		t.Errorf("hidden price sample leaked into cached prompt:\n%s", prompt)
	}
	// stock is still visible (only price hidden) — it should remain.
	if !strings.Contains(prompt, "stockQuantity") {
		t.Errorf("stock should remain visible when only price is hidden:\n%s", prompt)
	}
}

// LEAK-1: the cache key must include the visibility flags so flipping a flag
// rebuilds the prompt instead of serving the stale (leaky) cached one.
func TestPromptCache_KeyedOnVisibilityFlags(t *testing.T) {
	cat := &flagTenantCatalog{settings: map[string]any{"priceVisible": true}}
	pc := NewPromptCache(samplingFieldDefPort{}, noopPresetPort{}, cat, "product")

	visible, err := pc.GetOrBuild(context.Background(), "acme", 3)
	if err != nil {
		t.Fatalf("GetOrBuild (visible): %v", err)
	}
	if !strings.Contains(visible, "250000") {
		t.Fatalf("price should be present when visible:\n%s", visible)
	}

	// Toggle the tenant to hide price. Same slug — if the cache keyed on slug
	// alone it would return the stale visible prompt.
	cat.settings = map[string]any{"priceVisible": false}
	hidden, err := pc.GetOrBuild(context.Background(), "acme", 3)
	if err != nil {
		t.Fatalf("GetOrBuild (hidden): %v", err)
	}
	if strings.Contains(hidden, "250000") {
		t.Errorf("toggle to hidden served a stale prompt — cache not keyed on flags:\n%s", hidden)
	}
}

// Default visible (nil catalog or no flags) keeps price — guards against
// regressing other tenants when the flags are absent.
func TestPromptCache_DefaultVisible(t *testing.T) {
	pc := NewPromptCache(samplingFieldDefPort{}, noopPresetPort{}, &flagTenantCatalog{settings: nil}, "product")
	prompt, err := pc.GetOrBuild(context.Background(), "acme", 3)
	if err != nil {
		t.Fatalf("GetOrBuild: %v", err)
	}
	if !strings.Contains(prompt, "250000") {
		t.Errorf("default-visible tenant lost its price sample:\n%s", prompt)
	}
}
