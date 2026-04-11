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
// It sits in front of a ports.CanvasPresetPort with a process-local TTL cache
// so that a single chat session does not hit Postgres on every Agent2 turn.
// Cache invalidation is time-based (60s in Phase 2) — a publish in admin
// becomes visible everywhere within the TTL window. Phase 3 will switch to
// explicit bumping via the `admin.tenant_design_context_version` counter.
//
// The loader is nil-tolerant at every seam:
//   - nil loader   → callers must use GetPreset directly (main.go guards this
//                    by passing nil when the DB is unavailable and the tool
//                    falls through to the global registry).
//   - nil port     → Resolve always misses the tenant tier and returns the
//                    global registry result.
//   - empty tenant → same as nil port; global registry only.
//
// This matches the boot-without-DB pattern already used by the rest of V4.
type TenantPresetLoader struct {
	port ports.CanvasPresetPort
	ttl  time.Duration

	// cache is keyed by tenantIDOrSlug. Each entry is an independent
	// tenantCache so invalidations (future phase) can drop one tenant without
	// touching others.
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
}

// presetCacheEntry is one cached Preset with its expiry. A nil preset with a
// non-zero expiry means "we looked and the tenant does not have this preset"
// — a negative cache hit that avoids re-querying for every Agent2 turn.
type presetCacheEntry struct {
	preset    *Preset
	expiresAt time.Time
}

// defaultPresetCacheTTL is the 60s window specified in the KeepstarCanvas
// Phase 2 plan. Tuned short enough that a publish in admin becomes visible
// quickly, long enough that chat turns don't thrash Postgres.
const defaultPresetCacheTTL = 60 * time.Second

// NewTenantPresetLoader constructs a loader. The port may be nil — the loader
// then degrades to a plain wrapper around the global registry, which keeps
// boot-without-DB working.
func NewTenantPresetLoader(port ports.CanvasPresetPort) *TenantPresetLoader {
	return &TenantPresetLoader{
		port: port,
		ttl:  defaultPresetCacheTTL,
	}
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
	if tc.listPopulated && now.Before(tc.listExpiresAt) {
		return cloneSlice(tc.list)
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
	return cloneSlice(out)
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
