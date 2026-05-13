package engine

// ResolveStats summarises a ResolveAndInline pass.
type ResolveStats struct {
	// Resolved counts how many RefNodes were successfully expanded and
	// replaced with their resolved subtrees.
	Resolved int
	// Failed lists ref-node ids whose ref target was missing or whose
	// expansion bounced off MaxRefDepth. The unresolved RefNode stays in
	// place so downstream debugging can see what was attempted.
	Failed []string
}

// ResolveAndInline walks the document and, for every RefNode encountered
// in the children of a frame/group (or the document root), replaces it
// in-place with the result of ComponentResolver.Expand. Top-level reusable
// component definitions are NOT removed — they sit alongside the inlined
// instances so subsequent passes (e.g. nested ref resolution if a
// component references another component) can still find their targets.
// BindData skips the reusable subtrees so they don't pollute results.
//
// Why this lives outside ComponentResolver: chunk 1's ExpandAll returns a
// map and never mutates the doc — that's the v9-compatible read surface.
// Chunk 5 needs in-place mutation as a deliberate pipeline step before
// BindData, and we'd rather add a wrapper than re-shape ExpandAll's
// behaviour mid-V5.
func ResolveAndInline(doc *Document) ResolveStats {
	stats := ResolveStats{}
	if doc == nil {
		return stats
	}
	cr := NewComponentResolver()
	doc.Children = inlineRefsInChildren(doc.Children, doc, cr, &stats)
	return stats
}

// inlineRefsInChildren returns the input slice with every RefNode replaced
// by its resolved subtree. Recurses into frame/group children.
//
// We do NOT recurse into a child *after* it gets resolved — that would
// expand refs inside the already-resolved subtree, which is the resolver's
// own job (expandRef recurses into nested refs at lines 108-120 of
// component_resolver.go). Recursing here would double-expand.
func inlineRefsInChildren(children []Node, doc *Document, cr *ComponentResolver, stats *ResolveStats) []Node {
	out := make([]Node, len(children))
	for i, child := range children {
		if NodeType(child) == NodeTypeRef {
			resolved := cr.Expand(child, doc)
			if resolved.Ok {
				// `reusable:true` is stripped inside expandRef itself, so
				// both top-level and nested ref expansions produce
				// instance-shaped clones BindData will walk.
				out[i] = resolved.Root
				stats.Resolved++
			} else {
				out[i] = child
				stats.Failed = append(stats.Failed, NodeID(child))
			}
			continue
		}
		// Non-ref: pass through, but recurse if the child has its own children.
		if HasChildren(child) {
			SetChildren(child, inlineRefsInChildren(Children(child), doc, cr, stats))
		}
		out[i] = child
	}
	return out
}
