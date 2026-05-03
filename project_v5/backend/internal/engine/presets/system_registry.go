package presets

import (
	"encoding/json"
	"sort"
	"sync"
)

// SystemPresetRegistry is the in-process fallback the postgres preset
// adapter consults when the tenant has not authored a named preset.
// Holds the embedded SystemPresetSeeds; parses each on first lookup
// and caches the parsed JSON bytes for reuse.
//
// Why a registry rather than DB seeding: per-tenant seeding would force
// re-seeding on every new tenant and create implicit ownership
// confusion. System presets are by definition not tenant-specific.
// Tenant-authored versions in v5_presets override the registry — the
// adapter checks DB first and falls back to the registry only when the
// DB returns ErrPresetNotFound.
type SystemPresetRegistry struct {
	cache sync.Map // name → []byte (cached doc body)
}

// NewSystemPresetRegistry returns a ready-to-use registry. Cheap
// constructor — does not parse anything until Get is called.
func NewSystemPresetRegistry() *SystemPresetRegistry {
	return &SystemPresetRegistry{}
}

// Get returns the embedded JSON bytes for the named system preset and a
// presence flag. Mirrors map-lookup ergonomics. The returned bytes are
// safe to pass directly to json.Unmarshal into engine.Document.
//
// Validates that the embedded payload is well-formed JSON on first hit;
// caches the validated bytes for later calls. Validation failures
// surface as "not found" — the registry never returns malformed JSON.
func (r *SystemPresetRegistry) Get(name string) ([]byte, bool) {
	if cached, ok := r.cache.Load(name); ok {
		return cached.([]byte), true
	}
	raw, ok := SystemPresetSeeds[name]
	if !ok {
		return nil, false
	}
	// Validate on first use.
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		// Embedded JSON is broken — treat as not present rather than
		// crashing or returning bad bytes. Tests catch this; production
		// would manifest as a "preset not found" log line.
		return nil, false
	}
	r.cache.Store(name, raw)
	return raw, true
}

// DefaultReplicate returns the seed-time default replicate flag for the
// named system preset. Used when the adapter wraps registry hits into
// domain.Preset rows.
func (r *SystemPresetRegistry) DefaultReplicate(name string) bool {
	return SystemPresetDefaultReplicate[name]
}

// List returns every system preset name in sorted order. Used by the
// postgres adapter's ListPublishedPresets to union DB + registry.
func (r *SystemPresetRegistry) List() []string {
	out := make([]string, 0, len(SystemPresetSeeds))
	for name := range SystemPresetSeeds {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// SystemComponentRegistry is the in-process fallback for reusable
// components. Mirrors SystemPresetRegistry: validate-on-first-use,
// cache validated bytes, treat malformed JSON as "not found".
//
// Used by ComponentAdapter when a tenant has not authored the named
// component in v5_components — without this, refs in system presets
// (price-rating-root, brand-badge-root) stay unresolved and rendered
// cards collapse to title+image only.
type SystemComponentRegistry struct {
	cache sync.Map // name → []byte
}

// NewSystemComponentRegistry returns a ready-to-use registry. Cheap
// constructor — does not parse anything until Get is called.
func NewSystemComponentRegistry() *SystemComponentRegistry {
	return &SystemComponentRegistry{}
}

// Get returns the embedded JSON bytes for the named system component
// and a presence flag. Same validate-on-first-hit semantics as
// SystemPresetRegistry.Get.
func (r *SystemComponentRegistry) Get(name string) ([]byte, bool) {
	if cached, ok := r.cache.Load(name); ok {
		return cached.([]byte), true
	}
	raw, ok := SystemComponentSeeds[name]
	if !ok {
		return nil, false
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, false
	}
	r.cache.Store(name, raw)
	return raw, true
}

// List returns every system component name in sorted order. Used by
// the postgres adapter's ListPublishedComponents to union DB + registry
// (so Materialise(preset, components) sees every reusable referenced
// by the system presets).
func (r *SystemComponentRegistry) List() []string {
	out := make([]string, 0, len(SystemComponentSeeds))
	for name := range SystemComponentSeeds {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
