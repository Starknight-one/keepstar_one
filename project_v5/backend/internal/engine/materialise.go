package engine

// Materialise produces a single Document where component bodies sit at the
// top level alongside the preset's children. Any RefNode in the preset
// that points to a component's root id will resolve via FindNodeByID once
// ResolveAndInline runs.
//
// Pipeline order, for context:
//
//	Materialise(preset, components)         → merged *Document
//	  ↓
//	(ApplyOps, future)
//	  ↓
//	ExpandReplicates(doc, count)            → fan-out
//	  ↓
//	ResolveAndInline(doc)                   → ref → resolved subtree
//	  ↓
//	BindData(doc, data)                     → atoms filled, reusables skipped
//
// Materialise deep-clones the preset before mutation so a cached / shared
// preset Document is never written through. Each component's first
// top-level child is appended to the merged Children with `reusable:true`
// stamped on it — the binding walker uses that flag to skip the
// definition while still walking instances inlined under refs.
//
// Components with empty Document.Children are silently skipped (caller
// supplied an unauthored component; not an engine error).
//
// Multi-root components: only Children[0] is appended in V5. Tracked in
// docs/v5-known-gaps.md; multi-root needs either component packs or a
// separate materialise mode to land cleanly.
func Materialise(preset *Document, components []*Document) *Document {
	merged := cloneDocument(preset)
	for _, comp := range components {
		if comp == nil || len(comp.Children) == 0 {
			continue
		}
		root := cloneNode(comp.Children[0])
		root[attrReusable] = true
		merged.Children = append(merged.Children, root)
	}
	return merged
}

// cloneDocument returns a deep copy of doc with deep-cloned Children.
// Variables and other top-level fields are copied by value (Variables map
// is not deep-cloned because it's typically theme tokens that shouldn't
// be mutated by binding/replicate; if a chunk later mutates Variables,
// upgrade this helper).
func cloneDocument(doc *Document) *Document {
	if doc == nil {
		return nil
	}
	out := &Document{
		Version:   doc.Version,
		Themes:    doc.Themes,
		Imports:   doc.Imports,
		Variables: doc.Variables,
		Children:  make([]Node, len(doc.Children)),
	}
	for i, c := range doc.Children {
		out.Children[i] = cloneNode(c)
	}
	return out
}
