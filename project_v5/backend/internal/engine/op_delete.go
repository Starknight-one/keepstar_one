package engine

import "fmt"

// DeleteCommand removes a node from its parent's children. Mirrors v9's
// DeleteCommand. On Undo, reinserts at the original index in the original
// parent.
type DeleteCommand struct {
	nodeID      string
	removed     Node   // populated on Execute
	parentID    string // "" → document root
	index       int
}

// NewDeleteCommand prepares a delete of node nodeID.
func NewDeleteCommand(nodeID string) *DeleteCommand {
	return &DeleteCommand{nodeID: nodeID}
}

func (c *DeleteCommand) Execute(doc *Document) error {
	ref := FindParent(doc, c.nodeID)
	if ref == nil {
		return fmt.Errorf("delete: node %q not found", c.nodeID)
	}
	c.parentID = ParentID(ref.Parent)
	c.index = ref.Index

	children := childrenOf(ref.Parent)
	c.removed = children[ref.Index]
	children = append(children[:ref.Index], children[ref.Index+1:]...)
	SetParentChildren(ref.Parent, children)
	return nil
}

func (c *DeleteCommand) Undo(doc *Document) error {
	if c.removed == nil {
		return nil
	}
	if c.parentID == "" {
		doc.Children = insertAt(doc.Children, c.index, c.removed)
		return nil
	}
	parent := FindNodeByID(doc, c.parentID)
	if parent == nil || !HasChildren(parent) {
		return nil
	}
	children := Children(parent)
	children = insertAt(children, c.index, c.removed)
	SetChildren(parent, children)
	return nil
}
