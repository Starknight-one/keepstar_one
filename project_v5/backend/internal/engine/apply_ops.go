package engine

import (
	"fmt"
	"strings"
)

// ApplyOps translates a list of JSON-shaped ops (parsed from
// visual_assembly.input.ops) into engine.Command instances and executes
// them sequentially. Returns the first error encountered; subsequent ops
// are NOT applied (V4 fail-fast).
//
// Each op is a map[string]any with the shape:
//
//	{
//	  "op":     "insert" | "update" | "delete" | "move" | "override",
//	  "target":   "<node id>",         // for update / delete / move /
//	                                   // override (= ref node id)
//	  "parent":   "<node id> | $ref",  // for insert / move
//	  "ref":      "<local binding>",   // for insert (records the new
//	                                   // node's id under this name so
//	                                   // later ops can reference it as
//	                                   // "$<ref>")
//	  "after":    "<node id>",         // (reserved; not used yet)
//	  "props":    {...},               // for insert / update / override
//	}
//
// `$ref` resolution: when an op carries `"ref": "cta"`, the applier
// records `cta → <created node id>` after a successful Insert. Later
// ops' `parent: "$cta"` (or `target: "$cta"`) are rewritten to the actual
// id. Refs are scoped to one ApplyOps call and discarded afterwards —
// V5's run-binding cache (deferred) will lift this to per-turn.
//
// override shape:
//
//	{
//	  "op": "override",
//	  "target": "<refNode id>",
//	  "props": { "<descendantId>": { "key": "value" } }   // descendant override
//	  // OR
//	  "props": { "key": "value" }                         // root override
//	}
//
// Distinguished by whether the property values are themselves maps. If
// every top-level prop value is a map[string]any, treated as descendant
// overrides; otherwise treated as a root override (SetRootOverrideCommand).
func ApplyOps(doc *Document, ops []map[string]any) error {
	if len(ops) == 0 {
		return nil
	}
	history := NewCommandHistory()
	refMap := map[string]string{}

	for i, op := range ops {
		opName, _ := op["op"].(string)
		if opName == "" {
			return fmt.Errorf("ops[%d]: missing `op`", i)
		}

		cmd, refKey, err := buildCommand(op, refMap)
		if err != nil {
			return fmt.Errorf("ops[%d] (%s): %w", i, opName, err)
		}
		if err := history.Execute(cmd, doc); err != nil {
			return fmt.Errorf("ops[%d] (%s) execute: %w", i, opName, err)
		}
		// Record the new node's id under the local ref binding so
		// later ops can reference it via "$<ref>".
		if refKey != "" {
			if ic, ok := cmd.(*InsertCommand); ok {
				refMap[refKey] = ic.CreatedID()
			}
		}
	}
	return nil
}

// buildCommand returns a Command instance for one op. Also returns the
// `ref` value (so the applier can record it after Execute) and any error
// arising from missing/invalid fields.
func buildCommand(op map[string]any, refMap map[string]string) (Command, string, error) {
	opName, _ := op["op"].(string)
	switch opName {
	case "insert":
		parent := resolveRef(stringField(op, "parent"), refMap)
		props, _ := op["props"].(map[string]any)
		if props == nil {
			props = map[string]any{}
		}
		// InsertCommand takes a Node (= map[string]any). Treat props as
		// the node body — `type` and any other declarative keys live in
		// props per the schema.
		node := Node(props)
		ref, _ := op["ref"].(string)
		return NewInsertCommand(parent, node), ref, nil

	case "update":
		target := resolveRef(stringField(op, "target"), refMap)
		if target == "" {
			return nil, "", fmt.Errorf("update: target required")
		}
		props, _ := op["props"].(map[string]any)
		if props == nil {
			return nil, "", fmt.Errorf("update: props required")
		}
		return NewUpdateCommand(target, props), "", nil

	case "delete":
		target := resolveRef(stringField(op, "target"), refMap)
		if target == "" {
			return nil, "", fmt.Errorf("delete: target required")
		}
		return NewDeleteCommand(target), "", nil

	case "move":
		target := resolveRef(stringField(op, "target"), refMap)
		if target == "" {
			return nil, "", fmt.Errorf("move: target required")
		}
		parent := resolveRef(stringField(op, "parent"), refMap)
		// useIndex / newIndex unsupported via op JSON for now — append.
		return NewMoveCommand(target, parent, 0, false), "", nil

	case "override":
		target := resolveRef(stringField(op, "target"), refMap)
		if target == "" {
			return nil, "", fmt.Errorf("override: target (refNode id) required")
		}
		props, _ := op["props"].(map[string]any)
		if props == nil {
			return nil, "", fmt.Errorf("override: props required")
		}
		// Heuristic: if every top-level value is itself a map, this is a
		// descendant-override (one inner map per target descendant). Else
		// treat as root override.
		if isDescendantOverrideShape(props) {
			// One Command per descendant entry; for ApplyOps simplicity,
			// build a composite that runs them in order. Common case is a
			// single descendant; collapse cleanly in that case.
			return newOverrideMulti(target, props), "", nil
		}
		return NewSetRootOverrideCommand(target, props), "", nil
	}
	return nil, "", fmt.Errorf("unknown op %q", opName)
}

// stringField returns op[key] as a string, or "" when missing / wrong type.
func stringField(op map[string]any, key string) string {
	v, _ := op[key].(string)
	return v
}

// resolveRef rewrites a `$name` reference into its concrete node id from
// refMap. Pass-through for non-`$` strings.
func resolveRef(s string, refMap map[string]string) string {
	if !strings.HasPrefix(s, "$") {
		return s
	}
	if id, ok := refMap[strings.TrimPrefix(s, "$")]; ok {
		return id
	}
	// Unresolved $ref — leave as-is so the engine surfaces a clear
	// "node not found" error rather than silently swallowing.
	return s
}

// isDescendantOverrideShape returns true when every top-level value in
// props is a map (suggests {descendantId: {key:value, ...}, ...}).
func isDescendantOverrideShape(props map[string]any) bool {
	if len(props) == 0 {
		return false
	}
	for _, v := range props {
		if _, ok := v.(map[string]any); !ok {
			return false
		}
	}
	return true
}

// overrideMulti runs N SetOverrideCommands as one Command unit. Used when
// a single override op carries multiple descendant entries.
type overrideMulti struct {
	cmds []*SetOverrideCommand
}

func newOverrideMulti(refNodeID string, props map[string]any) *overrideMulti {
	out := &overrideMulti{}
	for descID, raw := range props {
		inner, _ := raw.(map[string]any)
		out.cmds = append(out.cmds, NewSetOverrideCommand(refNodeID, descID, inner))
	}
	return out
}

func (m *overrideMulti) Execute(doc *Document) error {
	for i, c := range m.cmds {
		if err := c.Execute(doc); err != nil {
			// Roll back the ones that succeeded so partial state isn't left.
			for j := i - 1; j >= 0; j-- {
				_ = m.cmds[j].Undo(doc)
			}
			return err
		}
	}
	return nil
}

func (m *overrideMulti) Undo(doc *Document) error {
	for i := len(m.cmds) - 1; i >= 0; i-- {
		if err := m.cmds[i].Undo(doc); err != nil {
			return err
		}
	}
	return nil
}
