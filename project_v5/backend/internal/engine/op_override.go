package engine

import "fmt"

// SetOverrideCommand writes a per-descendant property override onto a
// RefNode. Mirrors v9's SetOverrideCommand: merges into refNode.descendants
// [targetNodeID]; on undo, restores the prior map (or deletes the entry if
// it didn't exist before).
type SetOverrideCommand struct {
	refNodeID    string
	targetNodeID string
	overrideProps map[string]any

	// undo state
	prevExisted   bool
	prevOverrides map[string]any // nil iff !prevExisted
}

// NewSetOverrideCommand prepares an override write.
func NewSetOverrideCommand(refNodeID, targetNodeID string, props map[string]any) *SetOverrideCommand {
	return &SetOverrideCommand{
		refNodeID:     refNodeID,
		targetNodeID:  targetNodeID,
		overrideProps: props,
	}
}

func (c *SetOverrideCommand) Execute(doc *Document) error {
	refNode := FindNodeByID(doc, c.refNodeID)
	if refNode == nil || NodeType(refNode) != NodeTypeRef {
		return fmt.Errorf("setOverride: RefNode %q not found", c.refNodeID)
	}
	descendants, _ := refNode["descendants"].(map[string]any)
	if descendants == nil {
		descendants = map[string]any{}
		refNode["descendants"] = descendants
	}

	if existing, ok := descendants[c.targetNodeID].(map[string]any); ok {
		c.prevExisted = true
		c.prevOverrides = make(map[string]any, len(existing))
		for k, v := range existing {
			c.prevOverrides[k] = v
		}
	} else {
		c.prevExisted = false
		c.prevOverrides = nil
	}

	target, _ := descendants[c.targetNodeID].(map[string]any)
	if target == nil {
		target = map[string]any{}
		descendants[c.targetNodeID] = target
	}
	for k, v := range c.overrideProps {
		target[k] = v
	}
	return nil
}

func (c *SetOverrideCommand) Undo(doc *Document) error {
	refNode := FindNodeByID(doc, c.refNodeID)
	if refNode == nil || NodeType(refNode) != NodeTypeRef {
		return nil
	}
	descendants, _ := refNode["descendants"].(map[string]any)
	if descendants == nil {
		return nil
	}
	if !c.prevExisted {
		delete(descendants, c.targetNodeID)
	} else {
		descendants[c.targetNodeID] = c.prevOverrides
	}
	if len(descendants) == 0 {
		delete(refNode, "descendants")
	}
	return nil
}

// SetRootOverrideCommand writes property overrides directly on the RefNode
// itself (root-level overrides, separate from descendant overrides).
type SetRootOverrideCommand struct {
	refNodeID     string
	overrideProps map[string]any
	oldProps      map[string]any
	hadProps      map[string]bool
}

// NewSetRootOverrideCommand prepares a root-level override.
func NewSetRootOverrideCommand(refNodeID string, props map[string]any) *SetRootOverrideCommand {
	return &SetRootOverrideCommand{refNodeID: refNodeID, overrideProps: props}
}

func (c *SetRootOverrideCommand) Execute(doc *Document) error {
	refNode := FindNodeByID(doc, c.refNodeID)
	if refNode == nil || NodeType(refNode) != NodeTypeRef {
		return fmt.Errorf("setRootOverride: RefNode %q not found", c.refNodeID)
	}
	c.oldProps = make(map[string]any, len(c.overrideProps))
	c.hadProps = make(map[string]bool, len(c.overrideProps))
	for k, v := range c.overrideProps {
		old, had := refNode[k]
		c.oldProps[k] = old
		c.hadProps[k] = had
		refNode[k] = v
	}
	return nil
}

func (c *SetRootOverrideCommand) Undo(doc *Document) error {
	if c.oldProps == nil {
		return nil
	}
	refNode := FindNodeByID(doc, c.refNodeID)
	if refNode == nil {
		return nil
	}
	for k, oldVal := range c.oldProps {
		if c.hadProps[k] {
			refNode[k] = oldVal
		} else {
			delete(refNode, k)
		}
	}
	return nil
}
