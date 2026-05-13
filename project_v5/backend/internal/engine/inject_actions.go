package engine

// Auto-injection of default user-actions onto entity-bound subtrees.
//
// V5 doesn't have a V4-style widget concept that owns Actions[]. Instead,
// actions live directly on button atoms inside an "actions" frame — a
// hook the system seeds declare with id "actions" and an empty children
// slice, and which freestyle LLM-built cards may also include.
//
// What this pass does:
//
//  1. Find every replicate clone (frame carrying __templateOrigin) — those
//     are the entity-bound rows we want to decorate.
//  2. Inside each clone, find a descendant frame whose id contains
//     "actions" and whose children slice is empty. (Empty children means
//     the LLM did not author its own action list — we respect explicit
//     LLM choices when they exist.)
//  3. Append two button atoms — like + cart_add — bound to the entity
//     resolved via the clone's dataIndex against the data slice.
//  4. For replicate=0 / no-clone documents (single-entity detail views),
//     also walk the document for an "actions" frame and inject for
//     data[0]. Lets product_detail get default actions too.
//
// Idempotent: an "actions" frame with non-empty children is left
// alone, so calling this pass twice in a row is safe (and so is a
// freestyle LLM that explicitly populated the action list).
//
// Action wire shape:
//
//	{type:"text", content:"♥", wrapper:"button",
//	 action:{kind:"like", entity:{type:"product", id:"<id>"}}}
//
// Why text+wrapper instead of a dedicated "button" node type: V5 reuses
// V4's atom-with-wrapper convention (see domain.AtomWrapper) — buttons
// are text atoms with wrapper="button". The frontend wrapper.js maps
// wrapper="button" → onClick → dispatchAction.

import (
	"keepstar_v5/internal/domain"
)

const (
	actionsFrameIDHint = "actions"
	attrAction         = "action"
	attrWrapper        = "wrapper"
)

// InjectDefaultActions walks doc and appends default like + cart_add
// buttons to any empty "actions" frame inside an entity-bound subtree.
// Mutates doc in place.
//
// entityType is the EntityRef.Type that gets stamped on each action's
// entity. For product catalogs that's domain.EntityTypeProduct.
//
// data is the same []map[string]any BindData consumed; entries are
// indexed by the clone's dataIndex (or 0 for single-entity flows). The
// "id" key inside each record is read as the entity ID.
//
// Returns the number of actions frames that were populated.
func InjectDefaultActions(doc *Document, entityType domain.EntityType, data []map[string]any) int {
	if doc == nil || len(data) == 0 {
		return 0
	}
	count := 0
	for _, child := range doc.Children {
		count += injectInSubtree(child, entityType, data, -1)
	}
	return count
}

// injectInSubtree recursively walks a node looking for either:
//   - replicate clones (carry __templateOrigin) → record their dataIndex
//     and recurse with that index threaded down,
//   - actions frames (id contains "actions", empty children) → inject
//     using the resolved entity for the current dataIndex.
//
// inheritedIndex is the active dataIndex (-1 = no replicate ancestor;
// the single-entity fallback uses 0). Mirrors how BindData threads its
// inheritedIndex.
func injectInSubtree(n Node, entityType domain.EntityType, data []map[string]any, inheritedIndex int) int {
	if n == nil {
		return 0
	}
	currentIndex := inheritedIndex
	if explicit, ok := readDataIndex(n); ok {
		currentIndex = explicit
	}

	count := 0
	if isActionsFrame(n) && actionsFrameEmpty(n) {
		idx := currentIndex
		if idx < 0 {
			idx = 0
		}
		if id, ok := resolveEntityID(data, idx); ok {
			SetChildren(n, defaultActionAtoms(entityType, id))
			count++
		}
	}

	if HasChildren(n) {
		for _, c := range Children(n) {
			count += injectInSubtree(c, entityType, data, currentIndex)
		}
	}
	return count
}

// isActionsFrame reports whether n is a frame whose id is the literal
// "actions" hook. Strict equality, not substring — avoids false hits
// on ids like "transactions". Renamed variants are out of scope; the
// system seeds use "actions" by convention and freestyle LLM cards
// must follow that convention to opt into auto-injection.
func isActionsFrame(n Node) bool {
	if NodeType(n) != NodeTypeFrame {
		return false
	}
	return NodeID(n) == actionsFrameIDHint
}

// actionsFrameEmpty reports whether n's children slice is missing or
// empty. Anything else means the LLM populated it — leave alone.
func actionsFrameEmpty(n Node) bool {
	children := Children(n)
	return len(children) == 0
}

// resolveEntityID reads data[idx]["id"] if present.
func resolveEntityID(data []map[string]any, idx int) (string, bool) {
	if idx < 0 || idx >= len(data) {
		return "", false
	}
	rec := data[idx]
	if rec == nil {
		return "", false
	}
	id, ok := rec["id"].(string)
	if !ok || id == "" {
		return "", false
	}
	return id, true
}

// defaultActionAtoms returns the canonical pair of button atoms — like
// (heart) + cart_add (plus) — wired to the given entity. Future kinds
// (e.g. share) get added here, not at call sites.
func defaultActionAtoms(entityType domain.EntityType, entityID string) []Node {
	return []Node{
		buttonAtom("♥", domain.UserAction{
			Kind: domain.UserActionLike,
			Entity: &domain.EntityRef{
				Type: entityType,
				ID:   entityID,
			},
		}),
		buttonAtom("+", domain.UserAction{
			Kind: domain.UserActionCartAdd,
			Entity: &domain.EntityRef{
				Type: entityType,
				ID:   entityID,
			},
		}),
	}
}

// buttonAtom builds a text atom with wrapper="button" + the action
// payload. Matches the wire shape the frontend wrapper.js dispatches.
func buttonAtom(label string, act domain.UserAction) Node {
	return Node{
		"id":         GenerateID(),
		"type":       NodeTypeText,
		"content":    label,
		attrWrapper:  "button",
		attrAction:   userActionToMap(act),
	}
}

// userActionToMap flattens a domain.UserAction into the map[string]any
// shape that round-trips through engine.Document JSON. Keeps the
// engine package free of an encoding/json dependency for this helper.
func userActionToMap(a domain.UserAction) map[string]any {
	out := map[string]any{
		"kind": string(a.Kind),
	}
	if a.Entity != nil {
		out["entity"] = map[string]any{
			"type": string(a.Entity.Type),
			"id":   a.Entity.ID,
		}
	}
	if len(a.Params) > 0 {
		out["params"] = a.Params
	}
	return out
}

