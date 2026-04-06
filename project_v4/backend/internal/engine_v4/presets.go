package engine_v4

import "sort"

// Preset is a named, prebuilt bundle of ops the Agent2 can invoke by name via
// the visual_assembly tool (B2). Agent2 passes `preset: "<name>"` and the
// engine concatenates the preset ops with any override ops the agent included.
// Because ApplyOps processes a single batch, override ops can reference any
// $ref exposed by the preset (e.g. $w, $root, $info, $meta).
type Preset struct {
	Name             string
	Description      string
	Category         string   // "product" | "system" | "nav"
	Refs             []string // refs exposed by the preset that override ops can target
	DefaultReplicate bool     // inherited when the tool call does not pass `replicate` explicitly
	Build            func() []Op
}

var registry = map[string]*Preset{}

// RegisterPreset adds a preset to the registry. Called from init() in the
// preset files (presets_product.go, presets_system.go, presets_nav.go).
func RegisterPreset(p *Preset) {
	registry[p.Name] = p
}

// GetPreset returns a preset by name.
func GetPreset(name string) (*Preset, bool) {
	p, ok := registry[name]
	return p, ok
}

// ListPresets returns all registered presets sorted by name — stable order for
// tool schema enum generation and the Agent2 prompt listing.
func ListPresets() []*Preset {
	out := make([]*Preset, 0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// PresetNames returns sorted preset names — convenience for tool schema enum.
func PresetNames() []string {
	presets := ListPresets()
	names := make([]string, len(presets))
	for i, p := range presets {
		names[i] = p.Name
	}
	return names
}
