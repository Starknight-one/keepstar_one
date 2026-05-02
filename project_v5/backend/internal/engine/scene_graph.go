package engine

// childrenOf returns the children slice of either a *Document or a Node
// (frame/group). Returns nil if the input is not a container.
func childrenOf(root any) []Node {
	switch r := root.(type) {
	case *Document:
		return r.Children
	case Node:
		if HasChildren(r) {
			return Children(r)
		}
	}
	return nil
}

// FindNodeByID returns the first node within root with the given id, or nil
// if none. root may be *Document or Node. Recurses into frame/group children
// only — RefNode children are opaque (matches v9 semantics).
func FindNodeByID(root any, id string) Node {
	children := childrenOf(root)
	for _, child := range children {
		if NodeID(child) == id {
			return child
		}
		if HasChildren(child) {
			if found := FindNodeByID(child, id); found != nil {
				return found
			}
		}
	}
	return nil
}

// ParentRef captures the parent + index of a node within its parent's
// children slice. Parent is either a *Document or a Node (frame/group).
type ParentRef struct {
	Parent any
	Index  int
}

// FindParent returns the parent of the node identified by nodeId, plus the
// index in parent.children. Returns nil if the node is not found.
func FindParent(doc *Document, nodeId string) *ParentRef {
	return findParentIn(doc, nodeId)
}

func findParentIn(parent any, nodeId string) *ParentRef {
	children := childrenOf(parent)
	for i, child := range children {
		if NodeID(child) == nodeId {
			return &ParentRef{Parent: parent, Index: i}
		}
		if HasChildren(child) {
			if found := findParentIn(child, nodeId); found != nil {
				return found
			}
		}
	}
	return nil
}

// SetParentChildren writes children back to a *Document or Node parent.
// Used by operations that splice the children slice.
func SetParentChildren(parent any, children []Node) {
	switch p := parent.(type) {
	case *Document:
		if children == nil {
			p.Children = []Node{}
			return
		}
		p.Children = children
	case Node:
		SetChildren(p, children)
	}
}

// ParentID returns the parent's ID if it's a Node, or "" if parent is the
// Document root (no ID).
func ParentID(parent any) string {
	if n, ok := parent.(Node); ok {
		return NodeID(n)
	}
	return ""
}

// WalkFunc is invoked for each node visited by WalkNodes.
type WalkFunc func(node Node, depth int)

// WalkNodes traverses root depth-first, calling fn on each child node.
// Recurses into frame/group; RefNodes are visited but not descended into.
func WalkNodes(root any, fn WalkFunc) {
	walkAt(root, fn, 0)
}

func walkAt(root any, fn WalkFunc, depth int) {
	children := childrenOf(root)
	for _, child := range children {
		fn(child, depth)
		if HasChildren(child) {
			walkAt(child, fn, depth+1)
		}
	}
}

// FindReusableNodes returns every node with reusable=true.
func FindReusableNodes(doc *Document) []Node {
	var out []Node
	WalkNodes(doc, func(n Node, _ int) {
		if v, ok := n["reusable"].(bool); ok && v {
			out = append(out, n)
		}
	})
	return out
}

// FindNodesByType returns every node whose type discriminator matches.
func FindNodesByType(doc *Document, nodeType string) []Node {
	var out []Node
	WalkNodes(doc, func(n Node, _ int) {
		if NodeType(n) == nodeType {
			out = append(out, n)
		}
	})
	return out
}

// FindRefsToNode returns all RefNodes whose `ref` property equals targetID.
func FindRefsToNode(doc *Document, targetID string) []Node {
	var out []Node
	WalkNodes(doc, func(n Node, _ int) {
		if NodeType(n) != NodeTypeRef {
			return
		}
		if r, ok := n["ref"].(string); ok && r == targetID {
			out = append(out, n)
		}
	})
	return out
}

// SlotInfo pairs a frame node with its declared slot names.
type SlotInfo struct {
	Node      Node
	SlotNames []string
}

// FindSlots returns every frame node within root that declares slot names
// (frame.slot is a non-empty string array).
func FindSlots(root Node) []SlotInfo {
	var out []SlotInfo
	var walk func(n Node)
	walk = func(n Node) {
		if NodeType(n) == NodeTypeFrame {
			if names, ok := slotNamesFrom(n["slot"]); ok {
				out = append(out, SlotInfo{Node: n, SlotNames: names})
			}
		}
		if HasChildren(n) {
			for _, c := range Children(n) {
				walk(c)
			}
		}
	}
	walk(root)
	return out
}

// slotNamesFrom extracts a string slice from frame.slot. Returns (nil, false)
// when the slot is missing, false, or in another shape.
func slotNamesFrom(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		if len(s) == 0 {
			return nil, false
		}
		return s, true
	case []any:
		out := make([]string, 0, len(s))
		for _, x := range s {
			if str, ok := x.(string); ok {
				out = append(out, str)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	}
	return nil, false
}
