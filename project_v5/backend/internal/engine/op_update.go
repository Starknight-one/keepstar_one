package engine

import "fmt"

// UpdateCommand sets properties on an existing node. Faithful port of v9's
// UpdateCommand: snapshots only the keys being updated; on Undo, restores
// (or deletes if the key was originally absent) those keys.
type UpdateCommand struct {
	nodeID   string
	newProps map[string]any
	oldProps map[string]any // populated on Execute
	hadProps map[string]bool
}

// NewUpdateCommand prepares an update of node nodeID with the given props.
func NewUpdateCommand(nodeID string, props map[string]any) *UpdateCommand {
	return &UpdateCommand{nodeID: nodeID, newProps: props}
}

func (c *UpdateCommand) Execute(doc *Document) error {
	node := FindNodeByID(doc, c.nodeID)
	if node == nil {
		return fmt.Errorf("update: node %q not found", c.nodeID)
	}
	c.oldProps = make(map[string]any, len(c.newProps))
	c.hadProps = make(map[string]bool, len(c.newProps))
	for k, v := range c.newProps {
		old, had := node[k]
		c.hadProps[k] = had
		c.oldProps[k] = old
		node[k] = v
	}
	return nil
}

func (c *UpdateCommand) Undo(doc *Document) error {
	if c.oldProps == nil {
		return nil
	}
	node := FindNodeByID(doc, c.nodeID)
	if node == nil {
		return nil
	}
	for k, oldVal := range c.oldProps {
		if c.hadProps[k] {
			node[k] = oldVal
		} else {
			delete(node, k)
		}
	}
	return nil
}
