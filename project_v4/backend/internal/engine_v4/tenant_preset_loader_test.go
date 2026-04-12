package engine_v4

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"keepstar_v4/internal/ports"
)

// --- fake ports ---

type fakePresetPort struct {
	presets []ports.CanvasPresetView
	calls   atomic.Int32
}

func (f *fakePresetPort) GetPublishedPreset(_ context.Context, _, name string) (*ports.CanvasPresetView, bool, error) {
	f.calls.Add(1)
	for i := range f.presets {
		if f.presets[i].Name == name {
			return &f.presets[i], true, nil
		}
	}
	return nil, false, nil
}

func (f *fakePresetPort) ListPublishedPresets(_ context.Context, _ string) ([]ports.CanvasPresetView, error) {
	f.calls.Add(1)
	return f.presets, nil
}

type fakeComponentPort struct {
	components []ports.CanvasComponentView
	calls      atomic.Int32
}

func (f *fakeComponentPort) ListPublishedComponents(_ context.Context, _ string) ([]ports.CanvasComponentView, error) {
	f.calls.Add(1)
	return f.components, nil
}

type fakeDesignContextPort struct {
	version  atomic.Int64
	tokens   []ports.CanvasDesignTokenView
	getCalls atomic.Int32
}

func (f *fakeDesignContextPort) GetVersion(_ context.Context, _ string) (int64, error) {
	f.getCalls.Add(1)
	return f.version.Load(), nil
}

func (f *fakeDesignContextPort) ListDesignTokens(_ context.Context, _ string) ([]ports.CanvasDesignTokenView, error) {
	return f.tokens, nil
}

// --- tests ---

func TestLoadDesignContext_CachesByVersion(t *testing.T) {
	pp := &fakePresetPort{presets: []ports.CanvasPresetView{
		{Name: "banana_card", Category: "product", Description: "Test", OpsJSON: []byte(`[{"op":"insert","ref":"w","parent":"formation","props":{"type":"widget"}}]`)},
	}}
	cp := &fakeComponentPort{}
	dp := &fakeDesignContextPort{}
	dp.version.Store(1)

	loader := NewTenantPresetLoaderWithPorts(pp, cp, dp)
	ctx := context.Background()

	// First call — builds snapshot (hits all ports).
	snap1 := loader.LoadDesignContext(ctx, "tenant-a")
	if snap1 == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap1.Version != 1 {
		t.Errorf("expected version 1, got %d", snap1.Version)
	}
	if len(snap1.Presets) != 1 {
		t.Errorf("expected 1 preset, got %d", len(snap1.Presets))
	}
	listCalls1 := pp.calls.Load()

	// Second call — same version, should hit cache (0 new port calls for list).
	snap2 := loader.LoadDesignContext(ctx, "tenant-a")
	if snap2 != snap1 {
		t.Error("expected cached snapshot pointer reuse")
	}
	listCalls2 := pp.calls.Load()
	if listCalls2 != listCalls1 {
		t.Errorf("expected no new list calls, got %d → %d", listCalls1, listCalls2)
	}

	// Bump version — force bypass the 5s coalescing by clearing versionFetchedAt.
	dp.version.Store(2)
	tc := loader.tenantCacheFor("tenant-a")
	tc.mu.Lock()
	tc.versionFetchedAt = time.Time{} // force re-check
	tc.mu.Unlock()

	snap3 := loader.LoadDesignContext(ctx, "tenant-a")
	if snap3 == nil {
		t.Fatal("expected non-nil snapshot after version bump")
	}
	if snap3.Version != 2 {
		t.Errorf("expected version 2, got %d", snap3.Version)
	}
	if snap3 == snap1 {
		t.Error("expected new snapshot after version bump, got same pointer")
	}
}

func TestLoadDesignContext_VersionFetchedOncePerTTL(t *testing.T) {
	dp := &fakeDesignContextPort{}
	dp.version.Store(1)
	loader := NewTenantPresetLoaderWithPorts(nil, nil, dp)
	loader.versionTTL = 5 * time.Second // default, but explicit for test
	ctx := context.Background()

	// 3 rapid calls — should only hit GetVersion once (within coalescing window).
	loader.LoadDesignContext(ctx, "tenant-b")
	loader.LoadDesignContext(ctx, "tenant-b")
	loader.LoadDesignContext(ctx, "tenant-b")

	calls := dp.getCalls.Load()
	if calls != 1 {
		t.Errorf("expected 1 GetVersion call within TTL window, got %d", calls)
	}
}

func TestLoadDesignContext_NilTolerance(t *testing.T) {
	ctx := context.Background()

	// nil loader
	var nilLoader *TenantPresetLoader
	if snap := nilLoader.LoadDesignContext(ctx, "t"); snap != nil {
		t.Error("nil loader should return nil")
	}

	// empty tenant
	loader := NewTenantPresetLoaderWithPorts(nil, nil, nil)
	if snap := loader.LoadDesignContext(ctx, ""); snap != nil {
		t.Error("empty tenant should return nil")
	}

	// nil component port, nil design context port — should return snapshot
	// with just zero values, no panic.
	pp := &fakePresetPort{presets: []ports.CanvasPresetView{
		{Name: "test_preset", Category: "product", OpsJSON: []byte(`[]`)},
	}}
	partialLoader := NewTenantPresetLoaderWithPorts(pp, nil, nil)
	snap := partialLoader.LoadDesignContext(ctx, "tenant-c")
	if snap == nil {
		t.Fatal("expected non-nil snapshot with partial ports")
	}
	if snap.Version != 0 {
		t.Errorf("expected version 0 with nil design context port, got %d", snap.Version)
	}
	if len(snap.Presets) != 1 {
		t.Errorf("expected 1 preset, got %d", len(snap.Presets))
	}
	if snap.Components != nil {
		t.Error("expected nil components with nil component port")
	}
	if snap.Tokens != nil {
		t.Error("expected nil tokens with nil design context port")
	}
}

func TestLoadDesignContext_OverridesGlobal(t *testing.T) {
	// "product_card" exists in the global registry, so it should be flagged.
	pp := &fakePresetPort{presets: []ports.CanvasPresetView{
		{Name: "product_card", Category: "product", OpsJSON: []byte(`[{"op":"insert","ref":"w","parent":"formation","props":{"type":"widget"}}]`)},
		{Name: "banana_card", Category: "product", OpsJSON: []byte(`[{"op":"insert","ref":"w","parent":"formation","props":{"type":"widget"}}]`)},
	}}
	loader := NewTenantPresetLoaderWithPorts(pp, nil, nil)
	ctx := context.Background()

	snap := loader.LoadDesignContext(ctx, "tenant-d")
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if !snap.OverridesGlobal["product_card"] {
		t.Error("expected product_card to be flagged as global override")
	}
	if snap.OverridesGlobal["banana_card"] {
		t.Error("expected banana_card NOT to be flagged as global override")
	}
}

func TestNewTenantPresetLoader_Phase2Compat(t *testing.T) {
	// The Phase 2 constructor should still work and produce a functional loader.
	pp := &fakePresetPort{}
	loader := NewTenantPresetLoader(pp)
	if loader == nil {
		t.Fatal("expected non-nil loader from Phase 2 constructor")
	}
	if loader.componentPort != nil {
		t.Error("expected nil componentPort from Phase 2 constructor")
	}
	if loader.designContextPort != nil {
		t.Error("expected nil designContextPort from Phase 2 constructor")
	}
}
