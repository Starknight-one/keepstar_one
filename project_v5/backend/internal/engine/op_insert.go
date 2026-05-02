package engine

import "fmt"

// InsertCommand inserts a node into the Document, optionally under a parent.
//
// Faithful port of v9's InsertCommand. The node's "id" key is auto-populated
// via GenerateID if missing. parentID == "" means insert at document root.
type InsertCommand struct {
	parentID   string // "" → document root
	nodeData   Node
	insertedID string
}

// NewInsertCommand prepares an insert. parentID is the parent node's ID, or
// "" for the document root. nodeData is the node payload; if it lacks an
// "id" key, one is generated and stored on nodeData (so it shows up in the
// inserted tree and can be referenced after Execute).
func NewInsertCommand(parentID string, nodeData Node) *InsertCommand {
	id, ok := nodeData["id"].(string)
	if !ok || id == "" {
		id = GenerateID()
		nodeData["id"] = id
	}
	return &InsertCommand{
		parentID:   parentID,
		nodeData:   nodeData,
		insertedID: id,
	}
}

// CreatedID returns the ID assigned to the inserted node.
func (c *InsertCommand) CreatedID() string { return c.insertedID }

func (c *InsertCommand) Execute(doc *Document) error {
	if c.parentID == "" {
		doc.Children = append(doc.Children, c.nodeData)
		return nil
	}
	parent := FindNodeByID(doc, c.parentID)
	if parent == nil {
		return fmt.Errorf("insert: parent %q not found", c.parentID)
	}
	if !HasChildren(parent) {
		return fmt.Errorf("insert: parent %q (type %q) cannot have children", c.parentID, NodeType(parent))
	}
	children := Children(parent)
	children = append(children, c.nodeData)
	SetChildren(parent, children)
	return nil
}

func (c *InsertCommand) Undo(doc *Document) error {
	if c.parentID == "" {
		idx := indexOfChild(doc.Children, c.insertedID)
		if idx >= 0 {
			doc.Children = append(doc.Children[:idx], doc.Children[idx+1:]...)
		}
		return nil
	}
	parent := FindNodeByID(doc, c.parentID)
	if parent == nil || !HasChildren(parent) {
		return nil
	}
	children := Children(parent)
	idx := indexOfChild(children, c.insertedID)
	if idx >= 0 {
		children = append(children[:idx], children[idx+1:]...)
		SetChildren(parent, children)
	}
	return nil
}

// indexOfChild returns the index of the first child whose id matches, or -1.
func indexOfChild(children []Node, id string) int {
	for i, c := range children {
		if NodeID(c) == id {
			return i
		}
	}
	return -1
}
