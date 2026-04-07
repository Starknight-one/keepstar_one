package engine_v4

import (
	"fmt"
	"sort"
)

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

// ExpandInlinePresets walks an op batch and replaces each
// `insert widget` with `props.preset = "<name>"` by the preset's full op
// sequence. All refs in the expanded preset are namespaced with a per-widget
// prefix (`p0_`, `p1_`...) so multiple inline presets in the same batch
// don't collide on `$w / $root / $info / $meta`.
//
// User-provided fields on the original insert are preserved:
//   - `op.Ref`     becomes the substitute name for the preset's first ref
//                  (typically `w`), so override ops can keep using `$<userRef>`
//                  to target the inserted widget root.
//   - `op.Parent`  overrides the preset's first op parent (usually "formation").
//   - `op.After`   overrides the preset's first op after.
//   - `op.Props`   merges into the preset's first op props with user winning
//                  on conflicts. `preset` itself is stripped; `replicate`,
//                  `replicateLimit`, and `dataIndex` are forwarded to
//                  `insertWidget` which materialises them as `ReplicateConfig`.
//
// Ops without `props.preset` pass through unchanged. Unknown preset names
// produce a warning and the original op is left as-is (with `preset` stripped),
// so the standard insertWidget path runs and the user gets an empty widget
// instead of a silent crash.
func ExpandInlinePresets(ops []Op) ([]Op, []string) {
	out := make([]Op, 0, len(ops))
	var warnings []string
	presetCounter := 0

	for i, op := range ops {
		if op.Type != OpInsert || op.Props == nil {
			out = append(out, op)
			continue
		}
		presetName, _ := op.Props["preset"].(string)
		if presetName == "" {
			out = append(out, op)
			continue
		}

		preset, ok := GetPreset(presetName)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("op[%d]: unknown preset %q", i, presetName))
			out = append(out, stripPresetProp(op))
			continue
		}

		presetOps := preset.Build()
		if len(presetOps) == 0 {
			warnings = append(warnings, fmt.Sprintf("op[%d]: preset %q is empty", i, presetName))
			continue
		}

		first := presetOps[0]
		firstType, _ := first.Props["type"].(string)
		if first.Type != OpInsert || firstType != "widget" {
			warnings = append(warnings, fmt.Sprintf("op[%d]: preset %q first op is not a widget insert", i, presetName))
			out = append(out, stripPresetProp(op))
			continue
		}

		prefix := fmt.Sprintf("p%d", presetCounter)
		presetCounter++

		// Build the rename map. The first op's ref (typically "w") gets
		// substituted with the user's ref if provided, so override ops can
		// continue to use $<userRef> for the widget root. All other refs are
		// prefixed to avoid collisions across inline presets.
		widgetRef := op.Ref
		if widgetRef == "" {
			widgetRef = prefix + "_w"
		}
		renames := make(map[string]string)
		if first.Ref != "" {
			renames[first.Ref] = widgetRef
		}
		for _, pop := range presetOps[1:] {
			if pop.Ref != "" {
				if _, exists := renames[pop.Ref]; !exists {
					renames[pop.Ref] = prefix + "_" + pop.Ref
				}
			}
		}

		for j, pop := range presetOps {
			rewritten := Op{
				Type:   pop.Type,
				Target: pop.Target,
				Ref:    pop.Ref,
				Parent: pop.Parent,
				After:  pop.After,
				Props:  deepCopyOpProps(pop.Props),
			}
			if rewritten.Ref != "" {
				if newName, ok := renames[rewritten.Ref]; ok {
					rewritten.Ref = newName
				}
			}
			rewritten.Parent = renamePresetRef(rewritten.Parent, renames)
			rewritten.After = renamePresetRef(rewritten.After, renames)

			if j == 0 {
				if op.Parent != "" {
					rewritten.Parent = op.Parent
				}
				if op.After != "" {
					rewritten.After = op.After
				}
				// Merge user props (user wins) but strip `preset`.
				for k, v := range op.Props {
					if k == "preset" {
						continue
					}
					rewritten.Props[k] = v
				}
			}

			out = append(out, rewritten)
		}
	}

	return out, warnings
}

// renamePresetRef substitutes "$oldRef" with "$newRef" using the rename map.
// Plain (non-$) values pass through.
func renamePresetRef(s string, renames map[string]string) string {
	if len(s) > 1 && s[0] == '$' {
		if newName, ok := renames[s[1:]]; ok {
			return "$" + newName
		}
	}
	return s
}

// deepCopyOpProps clones an op props map. Necessary because preset Build()
// returns the same static slice each call; mutating its props would corrupt
// subsequent expansions.
func deepCopyOpProps(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch vv := v.(type) {
		case map[string]interface{}:
			out[k] = deepCopyOpProps(vv)
		default:
			out[k] = v
		}
	}
	return out
}

// stripPresetProp returns a copy of op with `preset` removed from props.
// Used for fall-through paths where preset expansion failed.
func stripPresetProp(op Op) Op {
	if op.Props == nil {
		return op
	}
	newProps := make(map[string]interface{}, len(op.Props))
	for k, v := range op.Props {
		if k != "preset" {
			newProps[k] = v
		}
	}
	op.Props = newProps
	return op
}
