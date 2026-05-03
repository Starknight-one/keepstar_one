package engine

import (
	"sort"
)

// TreeMap is the compact summary of a resolved Document that the runtime
// hands to Agent2 in modify-mode. Without it, the LLM has no idea which
// node ids exist and cannot send meaningful ops against the current view.
//
// Shape mirrors what V5's Agent2 system prompt advertises (see
// internal/prompts/agent2_prompt.go "TREE_MAP" section). Keep keys
// stable — the prompt teaches the LLM these specific names.
type TreeMap struct {
	PresetInUse string                 `json:"preset_in_use,omitempty"`
	Instances   *TreeMapInstances      `json:"instances,omitempty"`
	Literals    []TreeMapLiteral       `json:"literals,omitempty"`
	Components  []TreeMapComponent     `json:"components,omitempty"`
	DataCount   int                    `json:"data_count"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// TreeMapInstances groups the replicate clones for a single template.
// Multi-template documents (multi-widget compose) carry one TreeMap per
// orchestrator turn but only one instances group is exposed at a time —
// see Literals for the rest.
type TreeMapInstances struct {
	Count     int            `json:"count"`
	IDs       []string       `json:"ids"`
	Shape     TreeMapShape   `json:"shape"`
	GroupedBy string         `json:"grouped_by,omitempty"`
}

// TreeMapShape lists the atoms inside ONE replicate clone (not all N).
// Bound atoms collapse to {id, field}; ref-slot nodes show {id, ref, slot};
// literal atoms (no fieldBinding) include their content as a 32-char
// preview so the LLM can recognise them.
type TreeMapShape struct {
	Atoms []TreeMapAtom `json:"atoms"`
}

// TreeMapLiteral is a top-level non-replicated frame (hero, cta, error,
// explainer). Carries its own atoms inline.
type TreeMapLiteral struct {
	ID    string        `json:"id"`
	Atoms []TreeMapAtom `json:"atoms"`
}

// TreeMapAtom is one row in a shape/literal listing.
type TreeMapAtom struct {
	ID           string `json:"id"`
	Type         string `json:"type,omitempty"`
	FieldBinding string `json:"field,omitempty"`
	ContentPrev  string `json:"content,omitempty"`
	Format       string `json:"format,omitempty"`
	Wrapper      string `json:"wrapper,omitempty"`
	Slot         string `json:"slot,omitempty"`
	Ref          string `json:"ref,omitempty"`
}

// TreeMapComponent describes a v9 component inlined into the tree (post
// ResolveAndInline). Lists the atoms inside ONE resolved instance so the
// LLM knows what fields the component exposes — but NEVER target these
// ids in ops; they're shared across replicate clones.
type TreeMapComponent struct {
	ID    string        `json:"id"`
	Atoms []TreeMapAtom `json:"atoms"`
}

// BuildTreeMap walks a (resolved + replicated + bound) Document and returns
// the compact summary the LLM needs in modify mode. Returns nil for an
// empty document.
//
// Algorithm:
//  1. Collect top-level children. Group those carrying __templateOrigin
//     (replicate clones) by that origin id; the largest group becomes the
//     primary `instances`. Other groups + non-clone literals go into
//     `Literals` (caller may rebalance for multi-widget compose later).
//  2. For the primary group, derive Shape by walking the FIRST clone:
//     emit one TreeMapAtom per leaf node and per ref node.
//  3. Walk the document for ref nodes that have been resolved
//     (resolved == true) and emit a TreeMapComponent per unique ref id.
//  4. DataCount equals the number of clones in the primary group; falls
//     back to literal count when there are no replicates.
func BuildTreeMap(doc *Document) *TreeMap {
	if doc == nil || len(doc.Children) == 0 {
		return nil
	}

	tm := &TreeMap{}

	// 1. Group top-level children by templateOrigin.
	groups := map[string][]Node{}
	literalsRoots := []Node{}
	groupOrder := []string{}
	for _, child := range doc.Children {
		// Skip reusable component definitions — they live at root for the
		// resolver to find but should never appear in a user-facing tree map.
		if isReusable(child) {
			continue
		}
		origin, _ := child[attrTemplateOrigin].(string)
		if origin == "" {
			literalsRoots = append(literalsRoots, child)
			continue
		}
		if _, seen := groups[origin]; !seen {
			groupOrder = append(groupOrder, origin)
		}
		groups[origin] = append(groups[origin], child)
	}

	// 2. Pick primary group (largest, ties broken by appearance order).
	primaryOrigin := ""
	primarySize := 0
	for _, origin := range groupOrder {
		if len(groups[origin]) > primarySize {
			primarySize = len(groups[origin])
			primaryOrigin = origin
		}
	}
	if primaryOrigin != "" {
		clones := groups[primaryOrigin]
		ids := make([]string, 0, len(clones))
		for _, c := range clones {
			if id := NodeID(c); id != "" {
				ids = append(ids, id)
			}
		}
		tm.Instances = &TreeMapInstances{
			Count:     len(clones),
			IDs:       ids,
			Shape:     TreeMapShape{Atoms: collectAtoms(clones[0])},
			GroupedBy: primaryOrigin,
		}
		tm.DataCount = len(clones)
	}

	// Other replicate groups land as literals (each clone gets one entry,
	// labelled by id); bonafide non-clone literals do too.
	for _, origin := range groupOrder {
		if origin == primaryOrigin {
			continue
		}
		for _, c := range groups[origin] {
			tm.Literals = append(tm.Literals, TreeMapLiteral{
				ID:    NodeID(c),
				Atoms: collectAtoms(c),
			})
		}
	}
	for _, c := range literalsRoots {
		tm.Literals = append(tm.Literals, TreeMapLiteral{
			ID:    NodeID(c),
			Atoms: collectAtoms(c),
		})
	}
	if tm.DataCount == 0 {
		tm.DataCount = len(tm.Literals)
	}

	// 3. Components — collect resolved ref-node bodies. Walk the entire
	// document looking for resolved refs, dedup by ref name (each
	// component appears once even if used N times across clones).
	tm.Components = collectResolvedComponents(doc)

	return tm
}

// collectAtoms walks a single subtree (clone root or literal root) and
// emits one TreeMapAtom per leaf node, one per still-unresolved ref node,
// and one per resolved-component root (carrying __resolvedFrom). Frames /
// groups that are NOT resolved-component roots are structural plumbing
// and don't get their own row.
//
// Resolved component subtrees are NOT recursed into — their atoms belong
// to the component shape (see Components in TreeMap), and presenting
// them as targetable ids would mislead the LLM (those ids are shared
// across replicate clones and across uses of the same component).
func collectAtoms(root Node) []TreeMapAtom {
	out := []TreeMapAtom{}
	var walk func(n Node, isRoot bool)
	walk = func(n Node, isRoot bool) {
		// Treat resolved-component subtrees as opaque — emit a ref-style
		// row pointing at the source component and don't recurse. The root
		// of `collectAtoms` itself never qualifies as a resolved-component
		// (it's the clone root); the marker only fires on inner subtrees.
		if !isRoot {
			if rf, _ := n[attrResolvedFrom].(string); rf != "" {
				atom := TreeMapAtom{
					ID:   NodeID(n),
					Type: NodeTypeRef,
					Ref:  rf,
				}
				if s, _ := n["slot"].(string); s != "" {
					atom.Slot = s
				}
				out = append(out, atom)
				return
			}
		}
		t := NodeType(n)
		switch t {
		case NodeTypeFrame, NodeTypeGroup:
			for _, c := range Children(n) {
				walk(c, false)
			}
		case NodeTypeRef:
			atom := TreeMapAtom{
				ID:   NodeID(n),
				Type: t,
			}
			if r, _ := n["ref"].(string); r != "" {
				atom.Ref = r
			}
			if s, _ := n["slot"].(string); s != "" {
				atom.Slot = s
			}
			out = append(out, atom)
		default:
			atom := TreeMapAtom{
				ID:   NodeID(n),
				Type: t,
			}
			if fb, _ := n["fieldBinding"].(string); fb != "" {
				atom.FieldBinding = fb
			}
			if c, ok := n["content"].(string); ok && c != "" && atom.FieldBinding == "" {
				if len(c) > 32 {
					c = c[:29] + "..."
				}
				atom.ContentPrev = c
			}
			if f, _ := n["format"].(string); f != "" {
				atom.Format = f
			}
			if w, _ := n["wrapper"].(string); w != "" {
				atom.Wrapper = w
			}
			if s, _ := n["slot"].(string); s != "" {
				atom.Slot = s
			}
			out = append(out, atom)
		}
	}
	walk(root, true)
	return out
}

// collectResolvedComponents walks the document looking for resolved-
// component subtrees (carrying __resolvedFrom). Returns one entry per
// unique source component name with a flat atom listing of its leaves.
//
// Pre-resolve docs (still carrying ref nodes with their own children
// being the original component definition) won't surface here — only
// the post-resolve marker counts. The reusable definition lives at
// document root and is skipped via isReusable.
func collectResolvedComponents(doc *Document) []TreeMapComponent {
	seen := map[string]TreeMapComponent{}
	var walk func(nodes []Node)
	walk = func(nodes []Node) {
		for _, n := range nodes {
			if isReusable(n) {
				continue
			}
			if rf, _ := n[attrResolvedFrom].(string); rf != "" {
				if _, ok := seen[rf]; !ok {
					atoms := []TreeMapAtom{}
					for _, k := range Children(n) {
						atoms = append(atoms, flattenAtomsForComponent(k)...)
					}
					seen[rf] = TreeMapComponent{ID: rf, Atoms: atoms}
				}
				// Don't recurse into a resolved component subtree — its
				// inner refs (if any) are the component's own concern.
				continue
			}
			if HasChildren(n) {
				walk(Children(n))
			}
		}
	}
	walk(doc.Children)

	if len(seen) == 0 {
		return nil
	}
	out := make([]TreeMapComponent, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// attrResolvedFrom is the node attribute stamped by ComponentResolver
// onto resolved-component subtree roots. Holds the source component id
// (the value the original RefNode pointed at). Used by tree_map and
// future tooling; engine passes ignore it.
const attrResolvedFrom = "__resolvedFrom"

// flattenAtomsForComponent emits TreeMapAtoms for one node + its
// descendants. Differs from collectAtoms by NOT emitting ids — component
// internals are not targetable, so the LLM only needs to know what fields
// they expose.
func flattenAtomsForComponent(n Node) []TreeMapAtom {
	out := []TreeMapAtom{}
	var walk func(n Node)
	walk = func(n Node) {
		t := NodeType(n)
		switch t {
		case NodeTypeFrame, NodeTypeGroup:
			for _, c := range Children(n) {
				walk(c)
			}
		case NodeTypeRef:
			// Nested ref inside a component — skip.
			return
		default:
			atom := TreeMapAtom{Type: t}
			if fb, _ := n["fieldBinding"].(string); fb != "" {
				atom.FieldBinding = fb
			}
			if f, _ := n["format"].(string); f != "" {
				atom.Format = f
			}
			if w, _ := n["wrapper"].(string); w != "" {
				atom.Wrapper = w
			}
			if s, _ := n["slot"].(string); s != "" {
				atom.Slot = s
			}
			out = append(out, atom)
		}
	}
	walk(n)
	return out
}

// isReusable reports whether n carries the reusable marker (component
// definition that lives at document root for the resolver to find).
func isReusable(n Node) bool {
	v, ok := n["reusable"].(bool)
	return ok && v
}
