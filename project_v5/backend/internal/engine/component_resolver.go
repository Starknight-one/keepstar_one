package engine

import "fmt"

// MaxRefDepth caps the recursion in component expansion to prevent infinite
// loops on circular references.
const MaxRefDepth = 10

// ResolvedRef is the outcome of expanding a RefNode.
type ResolvedRef struct {
	// Root is the expanded clone (deep copy of the source with overrides
	// applied). On Ok=false, Root falls back to the original RefNode.
	Root Node
	// RefNode is the original RefNode that was resolved.
	RefNode Node
	// Ok indicates whether expansion succeeded.
	Ok bool
	// Error holds the failure reason when Ok is false.
	Error string
}

// ComponentResolver expands RefNodes into full node trees with overrides
// applied. Stateless.
type ComponentResolver struct{}

// NewComponentResolver returns the resolver.
func NewComponentResolver() *ComponentResolver { return &ComponentResolver{} }

// Expand resolves a single RefNode against the document. Faithful port of
// v9's expand: deep clone source → apply root overrides → set clone.id =
// refNode.id → apply descendant overrides → process slot injection →
// recurse into nested refs.
func (cr *ComponentResolver) Expand(refNode Node, doc *Document) ResolvedRef {
	return cr.expandRef(refNode, doc, 0)
}

// ExpandAll returns a map from each top-level/nested RefNode's ID to its
// resolved tree.
func (cr *ComponentResolver) ExpandAll(doc *Document) map[string]ResolvedRef {
	out := map[string]ResolvedRef{}
	var walk func(children []Node)
	walk = func(children []Node) {
		for _, n := range children {
			if NodeType(n) == NodeTypeRef {
				out[NodeID(n)] = cr.expandRef(n, doc, 0)
			}
			if HasChildren(n) {
				walk(Children(n))
			}
		}
	}
	walk(doc.Children)
	return out
}

var rootSkipKeys = map[string]bool{
	"type":        true,
	"ref":         true,
	"descendants": true,
	"id":          true,
}

func (cr *ComponentResolver) expandRef(refNode Node, doc *Document, depth int) ResolvedRef {
	if depth >= MaxRefDepth {
		return ResolvedRef{
			Root:    refNode,
			RefNode: refNode,
			Ok:      false,
			Error:   fmt.Sprintf("max ref depth (%d) exceeded — possible circular reference", MaxRefDepth),
		}
	}
	refTarget, _ := refNode["ref"].(string)
	if refTarget == "" {
		return ResolvedRef{Root: refNode, RefNode: refNode, Ok: false, Error: "ref: missing target"}
	}
	source := FindNodeByID(doc, refTarget)
	if source == nil {
		return ResolvedRef{
			Root:    refNode,
			RefNode: refNode,
			Ok:      false,
			Error:   fmt.Sprintf("source node %q not found", refTarget),
		}
	}

	clone := cloneNode(source)

	// Apply root-level overrides from the RefNode (skip ref-specific keys).
	for k, v := range refNode {
		if rootSkipKeys[k] {
			continue
		}
		if v != nil {
			clone[k] = cloneAny(v)
		}
	}
	// Set clone.id to RefNode's id so it occupies the correct layout slot.
	if id := NodeID(refNode); id != "" {
		clone["id"] = id
	}

	// Apply descendant overrides + slot injection.
	if descendants, ok := refNode["descendants"].(map[string]any); ok && descendants != nil {
		applyDescendantOverrides(clone, descendants)
		processSlots(clone, descendants)
	}

	// Recurse into nested refs in clone's children.
	if HasChildren(clone) {
		children := Children(clone)
		for i, child := range children {
			if NodeType(child) == NodeTypeRef {
				nested := cr.expandRef(child, doc, depth+1)
				if nested.Ok {
					children[i] = nested.Root
				}
			}
		}
		SetChildren(clone, children)
	}

	return ResolvedRef{Root: clone, RefNode: refNode, Ok: true}
}

// applyDescendantOverrides walks the cloned tree and merges the matching
// override map into each node (skipping the "children" key, which slot
// injection handles separately).
func applyDescendantOverrides(root Node, descendants map[string]any) {
	walkNodeAndSelf(root, func(n Node) {
		id := NodeID(n)
		if id == "" {
			return
		}
		ov, ok := descendants[id].(map[string]any)
		if !ok {
			return
		}
		for k, v := range ov {
			if k == "children" {
				continue
			}
			if v != nil {
				n[k] = cloneAny(v)
			}
		}
	})
}

// processSlots walks the cloned tree and replaces children of slot frames
// when an override carries a `children` key.
func processSlots(clone Node, descendants map[string]any) {
	walkNodeAndSelf(clone, func(n Node) {
		if NodeType(n) != NodeTypeFrame {
			return
		}
		// frame.slot must be a non-false value (string array)
		slot, ok := n["slot"]
		if !ok {
			return
		}
		if b, isBool := slot.(bool); isBool && !b {
			return
		}
		ov, ok := descendants[NodeID(n)].(map[string]any)
		if !ok {
			return
		}
		raw, has := ov["children"]
		if !has {
			return
		}
		switch v := raw.(type) {
		case []Node:
			out := make([]Node, len(v))
			for i, c := range v {
				out[i] = cloneNode(c)
			}
			SetChildren(n, out)
		case []any:
			out := make([]Node, 0, len(v))
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					out = append(out, cloneNode(m))
				}
			}
			SetChildren(n, out)
		}
	})
}

// walkNodeAndSelf invokes fn on n and (recursively) on every descendant
// inside frame/group children.
func walkNodeAndSelf(n Node, fn func(Node)) {
	fn(n)
	if HasChildren(n) {
		for _, c := range Children(n) {
			walkNodeAndSelf(c, fn)
		}
	}
}

// cloneNode returns a deep copy of n. Children are recursively cloned;
// other values are deep-cloned via cloneAny.
func cloneNode(n Node) Node {
	if n == nil {
		return nil
	}
	out := make(Node, len(n))
	for k, v := range n {
		if k == "children" {
			if cs, ok := childListAsNodes(v); ok {
				out["children"] = cloneChildren(cs)
				continue
			}
		}
		out[k] = cloneAny(v)
	}
	return out
}

func cloneChildren(children []Node) []Node {
	out := make([]Node, len(children))
	for i, c := range children {
		out[i] = cloneNode(c)
	}
	return out
}

// childListAsNodes coerces the value of "children" to []Node.
func childListAsNodes(v any) ([]Node, bool) {
	switch x := v.(type) {
	case []Node:
		return x, true
	case []any:
		out := make([]Node, 0, len(x))
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out, true
	}
	return nil, false
}

// cloneAny is a generic deep-clone for arbitrary JSON-like values used
// inside Node properties.
func cloneAny(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = cloneAny(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = cloneAny(val)
		}
		return out
	case []Node:
		out := make([]Node, len(x))
		for i, n := range x {
			out[i] = cloneNode(n)
		}
		return out
	case []string:
		out := make([]string, len(x))
		copy(out, x)
		return out
	case []float64:
		out := make([]float64, len(x))
		copy(out, x)
		return out
	}
	// Primitives (bool/int/float/string) are immutable in Go; return as-is.
	return v
}
