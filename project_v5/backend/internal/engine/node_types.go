// Package engine ports v9's scene-graph engine to Go. Faithful to v9's
// semantics: Document is a typed envelope, but each Node is a property bag
// (map[string]any) — the v9 TS source treats nodes as duck-typed objects, so
// we mirror that. Operations (insert/update/override/move/delete) mutate
// Document state in place.
package engine

// Node is a polymorphic scene-graph node represented as a property bag.
// Type discrimination happens via the "type" key.
type Node = map[string]any

// Node type discriminator values.
const (
	NodeTypeFrame     = "frame"
	NodeTypeGroup     = "group"
	NodeTypeRectangle = "rectangle"
	NodeTypeEllipse   = "ellipse"
	NodeTypeLine      = "line"
	NodeTypePolygon   = "polygon"
	NodeTypePath      = "path"
	NodeTypeText      = "text"
	NodeTypeNote      = "note"
	NodeTypePrompt    = "prompt"
	NodeTypeContext   = "context"
	NodeTypeIconFont  = "icon_font"
	NodeTypeRef       = "ref"
)

// NodeType returns n's "type" string, or "" if missing/wrong shape.
func NodeType(n Node) string {
	if t, ok := n["type"].(string); ok {
		return t
	}
	return ""
}

// NodeID returns n's "id" string, or "" if missing.
func NodeID(n Node) string {
	if id, ok := n["id"].(string); ok {
		return id
	}
	return ""
}

// HasChildren reports whether n can hold children (frame or group).
func HasChildren(n Node) bool {
	t := NodeType(n)
	return t == NodeTypeFrame || t == NodeTypeGroup
}

// HasGraphics reports whether n carries a fill or stroke property.
func HasGraphics(n Node) bool {
	_, hasFill := n["fill"]
	_, hasStroke := n["stroke"]
	return hasFill || hasStroke
}

// IsRef reports whether n is a RefNode (component instance).
func IsRef(n Node) bool {
	return NodeType(n) == NodeTypeRef
}

// Children returns n's children as a []Node, normalizing across the two
// shapes that arise: []Node (in-process construction) and []any (JSON
// unmarshal). Returns nil if absent or wrong shape.
func Children(n Node) []Node {
	raw, ok := n["children"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []Node:
		return v
	case []any:
		out := make([]Node, 0, len(v))
		for _, c := range v {
			if cm, ok := c.(map[string]any); ok {
				out = append(out, cm)
			}
		}
		return out
	}
	return nil
}

// SetChildren replaces n's children. Passing nil deletes the key.
func SetChildren(n Node, children []Node) {
	if children == nil {
		delete(n, "children")
		return
	}
	n["children"] = children
}
