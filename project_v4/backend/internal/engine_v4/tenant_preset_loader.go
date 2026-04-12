package engine_v4

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"keepstar_v4/internal/ports"
)

// TenantPresetLoader serves tenant-scoped published presets to the visual
// assembly tool, falling back to the global engine_v4 preset registry when
// the tenant has no custom version of the requested preset.
//
// Phase 2 introduced the core Resolve/ResolverFor/ListForTenant paths with a
// strict 60s TTL cache. Phase 3 extends the loader with:
//   - component + design-context ports for the <tenant_design_context> block
//   - DesignContextSnapshot — a read-only struct capturing presets, components,
//     tokens, and the version counter for Agent2's prompt builder
//   - LoadDesignContext — version-aware snapshot method; rebuilds only when the
//     admin.tenant_design_context_version counter changes
//   - 5s coalescing window on version counter reads to bound DB load
//
// Nil-tolerance at every seam is a contract:
//   - nil loader   → callers must use GetPreset directly
//   - nil port     → Resolve always misses the tenant tier
//   - nil componentPort / nil designContextPort → that section stays empty
//   - empty tenant → global registry only
type TenantPresetLoader struct {
	port              ports.CanvasPresetPort
	componentPort     ports.CanvasComponentPort     // Phase 3; may be nil
	designContextPort ports.CanvasDesignContextPort // Phase 3; may be nil
	ttl               time.Duration
	versionTTL        time.Duration // coalescing window for version counter reads

	// cache is keyed by tenantIDOrSlug. Each entry is an independent
	// tenantCache so invalidations can drop one tenant without touching others.
	cache sync.Map // map[string]*tenantCache
}

// tenantCache holds one tenant's resolved presets with TTL metadata. A single
// mutex guards both presets (Get result) and listExpiresAt (List result) so
// concurrent turns for the same tenant don't fight the map.
type tenantCache struct {
	mu             sync.Mutex
	presets        map[string]*presetCacheEntry
	list           []*Preset
	listExpiresAt  time.Time
	listPopulated  bool

	// Phase 3 additions: version-aware invalidation
	version          int64
	versionFetchedAt time.Time
	snapshot         *DesignContextSnapshot
	snapshotVersion  int64
}

// presetCacheEntry is one cached Preset with its expiry. A nil preset with a
// non-zero expiry means "we looked and the tenant does not have this preset"
// — a negative cache hit that avoids re-querying for every Agent2 turn.
type presetCacheEntry struct {
	preset    *Preset
	expiresAt time.Time
}

// DesignContextSnapshot is the loader's per-tenant payload consumed by
// Agent2's prompt builder. Captures everything the <tenant_design_context>
// block needs in a single read-only struct so the formatter stays pure and
// unit-testable.
type DesignContextSnapshot struct {
	TenantIDOrSlug  string
	Version         int64 // admin.tenant_design_context_version.version, 0 when missing
	Presets         []*Preset
	Components      []ports.CanvasComponentView
	Tokens          []ports.CanvasDesignTokenView
	// OverridesGlobal is the set of preset names that exist both in the
	// tenant library AND the global registry. Pre-computed so the formatter
	// can flag them without a second pass.
	OverridesGlobal map[string]bool
}

// defaultPresetCacheTTL is the 60s window specified in the KeepstarCanvas
// Phase 2 plan. Tuned short enough that a publish in admin becomes visible
// quickly, long enough that chat turns don't thrash Postgres.
const defaultPresetCacheTTL = 60 * time.Second

// defaultVersionCheckTTL is the 5s coalescing window. A burst of concurrent
// Agent2 turns on the same tenant only hits the DB for the version counter
// once per 5s, while a publish in admin becomes visible within ~5s.
const defaultVersionCheckTTL = 5 * time.Second

// NewTenantPresetLoaderWithPorts constructs a loader with all three port
// dependencies. Any port may be nil — the loader degrades gracefully:
//   - nil presetPort  → Resolve/ListForTenant miss the tenant tier
//   - nil componentPort → snap.Components == nil
//   - nil designContextPort → snap.Version stays 0, TTL-only invalidation
func NewTenantPresetLoaderWithPorts(
	presetPort ports.CanvasPresetPort,
	componentPort ports.CanvasComponentPort,
	designContextPort ports.CanvasDesignContextPort,
) *TenantPresetLoader {
	return &TenantPresetLoader{
		port:              presetPort,
		componentPort:     componentPort,
		designContextPort: designContextPort,
		ttl:               defaultPresetCacheTTL,
		versionTTL:        defaultVersionCheckTTL,
	}
}

// NewTenantPresetLoader is the Phase 2 compatibility wrapper. Leaves the new
// ports nil; loader degrades gracefully (no components, no tokens, version
// stays 0 forever, falls back to TTL-only invalidation).
func NewTenantPresetLoader(port ports.CanvasPresetPort) *TenantPresetLoader {
	return NewTenantPresetLoaderWithPorts(port, nil, nil)
}

// Resolve returns the preset the tool should run with, preferring a
// tenant-published version over the global registry. The second return value
// is false only when neither source has the preset — in that case the tool
// surfaces the same "unknown preset" error it always did.
func (l *TenantPresetLoader) Resolve(ctx context.Context, tenantIDOrSlug, name string) (*Preset, bool) {
	if l == nil || name == "" {
		return GetPreset(name)
	}

	if l.port != nil && tenantIDOrSlug != "" {
		if p, ok := l.lookupTenant(ctx, tenantIDOrSlug, name); ok {
			return p, true
		}
	}
	return GetPreset(name)
}

// ResolverFor returns a closure that captures the current request's tenant
// context for use inside the engine pipeline. ExpandInlinePresets needs a
// resolver that takes only a preset name (no ctx/tenant argument) because it
// runs deep inside ApplyOps and we don't want to plumb context through every
// hop of the op walker.
//
// The closure captures ctx and tenantIDOrSlug so a single request keeps using
// the same tenant across the whole op batch. When the loader is nil (no DB),
// this returns nil and the inline-expansion path falls through to GetPreset
// on its own.
func (l *TenantPresetLoader) ResolverFor(ctx context.Context, tenantIDOrSlug string) PresetResolver {
	if l == nil {
		return nil
	}
	return func(name string) (*Preset, bool) {
		return l.Resolve(ctx, tenantIDOrSlug, name)
	}
}

// ListForTenant returns every published preset the tenant owns, as fully
// parsed Preset structs ready for rendering. Used by Phase 3 to build the
// <tenant_design_context> block in Agent2's system prompt.
//
// Returns nil (not an empty slice) when the loader or port is nil, the
// tenant string is empty, or the DB query fails — callers should treat nil
// as "tenant has no custom library, fall back to global registry listing".
func (l *TenantPresetLoader) ListForTenant(ctx context.Context, tenantIDOrSlug string) []*Preset {
	if l == nil || l.port == nil || tenantIDOrSlug == "" {
		return nil
	}
	tc := l.tenantCacheFor(tenantIDOrSlug)
	tc.mu.Lock()
	defer tc.mu.Unlock()

	now := time.Now()
	return cloneSlice(l.listForTenantLocked(ctx, tc, tenantIDOrSlug, now))
}

// LoadDesignContext returns a snapshot of the tenant's published canvas
// library (presets + components + tokens) paired with the current design
// context version counter. Memoized per tenant; rebuilt when the version
// counter changes. Returns nil for a nil loader or empty tenant — callers
// skip the <tenant_design_context> block entirely in that case.
//
// Graceful degradation at every seam:
//   - nil componentPort → snap.Components == nil
//   - nil designContextPort → snap.Version stays 0, behaves like Phase 2 (TTL only)
//   - individual query error → that section is empty, rest of snapshot populates
func (l *TenantPresetLoader) LoadDesignContext(ctx context.Context, tenantIDOrSlug string) *DesignContextSnapshot {
	if l == nil || tenantIDOrSlug == "" {
		return nil
	}
	tc := l.tenantCacheFor(tenantIDOrSlug)
	tc.mu.Lock()
	defer tc.mu.Unlock()

	now := time.Now()

	// 1. Version check, coalesced within versionTTL (default 5s) so a burst
	//    of concurrent Agent2 turns on the same tenant only hits DB once.
	if l.designContextPort != nil &&
		(tc.versionFetchedAt.IsZero() || now.Sub(tc.versionFetchedAt) >= l.versionTTL) {
		if v, err := l.designContextPort.GetVersion(ctx, tenantIDOrSlug); err == nil {
			if v != tc.version {
				// Version changed — invalidate derived state so every downstream
				// (Resolve list, snapshot) rebuilds against the new version.
				tc.version = v
				tc.list = nil
				tc.listPopulated = false
				tc.snapshot = nil
				tc.snapshotVersion = 0
				// Also invalidate per-preset cache entries so Resolve picks up
				// newly published / deleted presets.
				tc.presets = make(map[string]*presetCacheEntry)
			}
			tc.versionFetchedAt = now
		}
	}

	// 2. Snapshot reuse.
	if tc.snapshot != nil && tc.snapshotVersion == tc.version {
		return tc.snapshot
	}

	// 3. Build fresh snapshot at current version.
	snap := &DesignContextSnapshot{
		TenantIDOrSlug:  tenantIDOrSlug,
		Version:         tc.version,
		OverridesGlobal: make(map[string]bool),
	}
	if l.port != nil {
		snap.Presets = l.listForTenantLocked(ctx, tc, tenantIDOrSlug, now)
		for _, p := range snap.Presets {
			if _, ok := GetPreset(p.Name); ok {
				snap.OverridesGlobal[p.Name] = true
			}
		}
	}
	if l.componentPort != nil {
		if comps, err := l.componentPort.ListPublishedComponents(ctx, tenantIDOrSlug); err == nil {
			snap.Components = comps
		}
	}
	if l.designContextPort != nil {
		if toks, err := l.designContextPort.ListDesignTokens(ctx, tenantIDOrSlug); err == nil {
			snap.Tokens = toks
		}
	}

	tc.snapshot = snap
	tc.snapshotVersion = tc.version
	return snap
}

// lookupTenant hits the cache first, then the port. Both positive and
// negative results are cached for the full TTL — we don't want every Agent2
// turn for a tenant with zero customizations to re-query Postgres.
func (l *TenantPresetLoader) lookupTenant(ctx context.Context, tenantIDOrSlug, name string) (*Preset, bool) {
	tc := l.tenantCacheFor(tenantIDOrSlug)
	tc.mu.Lock()
	defer tc.mu.Unlock()

	now := time.Now()
	if entry, ok := tc.presets[name]; ok && now.Before(entry.expiresAt) {
		if entry.preset == nil {
			return nil, false
		}
		return entry.preset, true
	}

	view, ok, err := l.port.GetPublishedPreset(ctx, tenantIDOrSlug, name)
	if err != nil || !ok || view == nil {
		l.storeEntry(tc, name, nil, now)
		return nil, false
	}
	preset := viewToPreset(view)
	if preset == nil {
		// Ops parse failure — cache the miss so we don't loop on bad data.
		l.storeEntry(tc, name, nil, now)
		return nil, false
	}
	l.storeEntry(tc, name, preset, now)
	return preset, true
}

// listForTenantLocked is the shared helper for ListForTenant and
// LoadDesignContext. Caller must hold tc.mu. Does NOT clone the result —
// ListForTenant clones on return, LoadDesignContext stores the slice in the
// snapshot (which is read-only once published).
func (l *TenantPresetLoader) listForTenantLocked(ctx context.Context, tc *tenantCache, tenantIDOrSlug string, now time.Time) []*Preset {
	if tc.listPopulated && now.Before(tc.listExpiresAt) {
		return tc.list
	}

	views, err := l.port.ListPublishedPresets(ctx, tenantIDOrSlug)
	if err != nil {
		return nil
	}
	out := make([]*Preset, 0, len(views))
	for i := range views {
		if p := viewToPreset(&views[i]); p != nil {
			out = append(out, p)
		}
	}
	tc.list = out
	tc.listPopulated = true
	tc.listExpiresAt = now.Add(l.ttl)
	return out
}

// tenantCacheFor returns (creating if needed) the per-tenant cache bucket.
func (l *TenantPresetLoader) tenantCacheFor(tenantIDOrSlug string) *tenantCache {
	if v, ok := l.cache.Load(tenantIDOrSlug); ok {
		return v.(*tenantCache)
	}
	fresh := &tenantCache{presets: make(map[string]*presetCacheEntry)}
	actual, _ := l.cache.LoadOrStore(tenantIDOrSlug, fresh)
	return actual.(*tenantCache)
}

// storeEntry writes one preset result into the tenant bucket. Caller must
// already hold tc.mu.
func (l *TenantPresetLoader) storeEntry(tc *tenantCache, name string, p *Preset, now time.Time) {
	if tc.presets == nil {
		tc.presets = make(map[string]*presetCacheEntry)
	}
	tc.presets[name] = &presetCacheEntry{preset: p, expiresAt: now.Add(l.ttl)}
}

// viewToPreset converts a raw ports.CanvasPresetView into an engine Preset.
// Parses the ops JSON once at cache-write time so every subsequent Resolve
// call is a map lookup with no JSON work in the hot path. Returns nil when
// the ops blob fails to parse — the caller records this as a miss and falls
// back to the global registry.
func viewToPreset(view *ports.CanvasPresetView) *Preset {
	if view == nil {
		return nil
	}
	ops, err := parseOpsJSON(view.OpsJSON)
	if err != nil {
		return nil
	}
	// Preset.Build is invoked once per Agent2 turn that uses the preset, and
	// ApplyOps deep-copies each op's props before mutating. Capture `ops` by
	// value in the closure so the slice we hand back is the same cached one
	// and the deep-copy in ExpandInlinePresets keeps callers isolated.
	return &Preset{
		Name:             view.Name,
		Description:      view.Description,
		Category:         view.Category,
		DefaultReplicate: view.DefaultReplicate,
		Build:            func() []Op { return ops },
	}
}

// parseOpsJSON unmarshals a stored ops JSON array into the engine's Op type.
// Accepts empty/null blobs as a zero-length slice so empty presets (just
// "hello world" placeholders) round-trip cleanly through the cache.
func parseOpsJSON(raw json.RawMessage) ([]Op, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var out []Op
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// cloneSlice returns a shallow copy of the slice so callers can reorder or
// append without mutating the cached list. Preset pointers inside are shared
// on purpose — Preset.Build returns fresh ops each call so aliasing the
// pointer is safe.
func cloneSlice(in []*Preset) []*Preset {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Preset, len(in))
	copy(out, in)
	return out
}
