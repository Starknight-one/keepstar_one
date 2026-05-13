package engine

import "fmt"

// MoveCommand relocates a node to a new parent (or document root) at an
// optional new index. Mirrors v9's MoveCommand. Undo reverses including a
// parent swap.
type MoveCommand struct {
	nodeID      string
	newParentID string // "" → document root
	newIndex    int    // -1 = append
	useIndex    bool

	// undo state
	oldParentID string
	oldIndex    int
}

// NewMoveCommand prepares a move. newParentID="" puts the node at the
// document root. If useIndex is false, the node is appended; otherwise
// inserted at index (clamped to length).
func NewMoveCommand(nodeID, newParentID string, newIndex int, useIndex bool) *MoveCommand {
	return &MoveCommand{
		nodeID:      nodeID,
		newParentID: newParentID,
		newIndex:    newIndex,
		useIndex:    useIndex,
	}
}

func (c *MoveCommand) Execute(doc *Document) error {
	current := FindParent(doc, c.nodeID)
	if current == nil {
		return fmt.Errorf("move: node %q not found", c.nodeID)
	}
	c.oldParentID = ParentID(current.Parent)
	c.oldIndex = current.Index

	srcChildren := childrenOf(current.Parent)
	node := srcChildren[current.Index]
	srcChildren = append(srcChildren[:current.Index], srcChildren[current.Index+1:]...)
	SetParentChildren(current.Parent, srcChildren)

	dstChildren, dstParent, err := resolveTargetChildren(doc, c.newParentID)
	if err != nil {
		return err
	}
	idx := len(dstChildren)
	if c.useIndex && c.newIndex < idx {
		idx = c.newIndex
		if idx < 0 {
			idx = 0
		}
	}
	dstChildren = insertAt(dstChildren, idx, node)
	SetParentChildren(dstParent, dstChildren)
	return nil
}

func (c *MoveCommand) Undo(doc *Document) error {
	current := FindParent(doc, c.nodeID)
	if current == nil {
		return nil
	}
	srcChildren := childrenOf(current.Parent)
	node := srcChildren[current.Index]
	srcChildren = append(srcChildren[:current.Index], srcChildren[current.Index+1:]...)
	SetParentChildren(current.Parent, srcChildren)

	dstChildren, dstParent, err := resolveTargetChildren(doc, c.oldParentID)
	if err != nil {
		// Fall back to document root
		dstChildren = doc.Children
		dstParent = doc
	}
	dstChildren = insertAt(dstChildren, c.oldIndex, node)
	SetParentChildren(dstParent, dstChildren)
	return nil
}

// resolveTargetChildren returns (children slice, parent any) for the given
// parent ID. parentID="" means document root.
func resolveTargetChildren(doc *Document, parentID string) ([]Node, any, error) {
	if parentID == "" {
		return doc.Children, doc, nil
	}
	p := FindNodeByID(doc, parentID)
	if p == nil {
		return nil, nil, fmt.Errorf("target parent %q not found", parentID)
	}
	if !HasChildren(p) {
		return nil, nil, fmt.Errorf("target parent %q cannot have children", parentID)
	}
	return Children(p), p, nil
}

func insertAt(children []Node, idx int, node Node) []Node {
	if idx >= len(children) {
		return append(children, node)
	}
	if idx < 0 {
		idx = 0
	}
	out := make([]Node, 0, len(children)+1)
	out = append(out, children[:idx]...)
	out = append(out, node)
	out = append(out, children[idx:]...)
	return out
}
