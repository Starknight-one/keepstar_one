package engine

// Auto-injection of default user-actions onto entity-bound subtrees.
//
// V5 doesn't have a V4-style widget concept that owns Actions[]. Instead,
// actions live directly on button atoms inside an "actions" frame — a
// hook the system seeds declare with id "actions" and an empty children
// slice, and which freestyle LLM-built cards may also include.
//
// Two routing modes, picked per entity scope:
//
//   - SLOT routing (preferred when present). A preset can declare typed
//     drop-zones: a frame carrying `acceptsAction:"like"` or
//     `acceptsAction:"cart_add"`. When a scope contains any such slot, each
//     default button is injected into the slot that accepts its kind — the
//     heart over the image, the add over the body, different parents. The
//     legacy single-"actions" frame is left untouched in this mode.
//   - LEGACY routing (fallback, unchanged). When a scope has no typed
//     slots, both buttons land together in the empty "actions" frame, the
//     behaviour every other preset and the canvas bridge rely on.
//
// What the pass does:
//
//  1. Find every replicate clone (frame carrying __templateOrigin) — those
//     are the entity-bound rows we want to decorate.
//  2. For each clone (and for replicate=0 / no-clone single-entity detail
//     views) resolve the entity via the clone's dataIndex against the data
//     slice, then route the default buttons into typed slots if any exist,
//     else into the empty "actions" frame.
//
// Idempotent: a slot / "actions" frame with non-empty children is left
// alone, so calling this pass twice in a row is safe (and so is a
// freestyle LLM that explicitly populated the action list).
//
// Action wire shape:
//
//	{type:"text", content:"♥", wrapper:"button", actionKind:"like",
//	 action:{kind:"like", entity:{type:"product", id:"<id>"}}}
//
// Why text+wrapper instead of a dedicated "button" node type: V5 reuses
// V4's atom-with-wrapper convention (see domain.AtomWrapper) — buttons
// are text atoms with wrapper="button". The frontend wrapper.js maps
// wrapper="button" → onClick → dispatchAction, and the actionKind hint →
// data-action-kind for per-kind styling.

import (
	"keepstar_v5/internal/domain"
)

const (
	actionsFrameIDHint = "actions"
	attrAction         = "action"
	attrWrapper        = "wrapper"
	attrActionKind     = "actionKind"
	attrAcceptsAction  = "acceptsAction"
)

// InjectDefaultActions walks doc and appends default like + cart_add
// buttons to each entity-bound subtree — into typed `acceptsAction` slots
// when the subtree declares them, else into an empty "actions" frame.
// Mutates doc in place.
//
// entityType is the EntityRef.Type that gets stamped on each action's
// entity. For product catalogs that's domain.EntityTypeProduct.
//
// data is the same []map[string]any BindData consumed; entries are
// indexed by the clone's dataIndex (or 0 for single-entity flows). The
// "id" key inside each record is read as the entity ID.
//
// Returns the number of frames (slots + actions) that were populated.
func InjectDefaultActions(doc *Document, entityType domain.EntityType, data []map[string]any) int {
	if doc == nil || len(data) == 0 {
		return 0
	}
	count := 0
	for _, child := range doc.Children {
		count += injectInSubtree(child, entityType, data, -1, true)
	}
	return count
}

// injectInSubtree recursively walks a node. It routes default buttons
// once per entity scope, where a scope is either:
//
//   - a replicate clone (carries a dataIndex stamped by ExpandReplicates), or
//   - a top-level document child with no clone ancestor (the single-entity
//     detail fallback) — recognised via isTopLevel + inheritedIndex < 0.
//
// Routing happens AT the scope node; the recursion then keeps descending so
// nested clones resolve against their own dataIndex, but inner frames are
// never themselves treated as fresh scopes (which would double-inject into
// a clone's own slot / "actions" frames).
//
// inheritedIndex is the active dataIndex (-1 = no replicate ancestor;
// the single-entity fallback uses 0). Mirrors how BindData threads its
// inheritedIndex.
func injectInSubtree(n Node, entityType domain.EntityType, data []map[string]any, inheritedIndex int, isTopLevel bool) int {
	if n == nil {
		return 0
	}
	currentIndex := inheritedIndex
	_, isClone := readDataIndex(n)
	if isClone {
		currentIndex = mustDataIndex(n)
	}

	count := 0
	if isClone || (isTopLevel && inheritedIndex < 0) {
		idx := currentIndex
		if idx < 0 {
			idx = 0
		}
		if id, ok := resolveEntityID(data, idx); ok {
			count += routeActions(n, entityType, id)
		}
	}

	if HasChildren(n) {
		for _, c := range Children(n) {
			count += injectInSubtree(c, entityType, data, currentIndex, false)
		}
	}
	return count
}

// mustDataIndex reads n's dataIndex, defaulting to 0 if absent (caller has
// already confirmed presence via readDataIndex).
func mustDataIndex(n Node) int {
	if idx, ok := readDataIndex(n); ok {
		return idx
	}
	return 0
}

// routeActions injects the like + cart_add buttons for entityID into n's
// scope. Slot routing wins when the scope declares any `acceptsAction`
// slot; otherwise the legacy empty-"actions"-frame fallback runs.
// Returns the number of frames populated.
func routeActions(n Node, entityType domain.EntityType, entityID string) int {
	slots := collectSlots(n)
	if len(slots) > 0 {
		count := 0
		for kind, frame := range slots {
			if !actionsFrameEmpty(frame) {
				continue // LLM / prior pass already populated it
			}
			SetChildren(frame, []Node{actionAtom(kind, entityType, entityID)})
			count++
		}
		return count
	}

	count := 0
	walkScope(n, func(c Node) {
		if isActionsFrame(c) && actionsFrameEmpty(c) {
			SetChildren(c, defaultActionAtoms(entityType, entityID))
			count++
		}
	})
	return count
}

// collectSlots returns the typed action slots in n's scope, keyed by the
// action kind they accept. First slot wins per kind (deterministic by
// document order). Only the closed like / cart_add vocabulary is honored.
func collectSlots(n Node) map[domain.UserActionKind]Node {
	slots := map[domain.UserActionKind]Node{}
	walkScope(n, func(c Node) {
		kind := acceptsActionKind(c)
		if kind == "" {
			return
		}
		if _, taken := slots[kind]; !taken {
			slots[kind] = c
		}
	})
	return slots
}

// walkScope visits n and every descendant in n's entity scope, stopping
// the descent at a nested replicate clone (those own their own scope and
// are handled by injectInSubtree's recursion). n itself is always visited.
func walkScope(n Node, fn func(Node)) {
	fn(n)
	if !HasChildren(n) {
		return
	}
	for _, c := range Children(n) {
		if _, nestedClone := readDataIndex(c); nestedClone {
			continue // nested scope — not ours
		}
		walkScope(c, fn)
	}
}

// acceptsActionKind returns the like / cart_add kind a frame slot accepts,
// or "" when n is not a typed slot (or names an unknown kind). Strict to
// the closed default-action vocabulary so a typo can't silently swallow a
// button.
func acceptsActionKind(n Node) domain.UserActionKind {
	if NodeType(n) != NodeTypeFrame {
		return ""
	}
	raw, _ := n[attrAcceptsAction].(string)
	switch domain.UserActionKind(raw) {
	case domain.UserActionLike:
		return domain.UserActionLike
	case domain.UserActionCartAdd:
		return domain.UserActionCartAdd
	}
	return ""
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
		actionAtom(domain.UserActionLike, entityType, entityID),
		actionAtom(domain.UserActionCartAdd, entityType, entityID),
	}
}

// actionAtom builds the canonical default button atom for one kind, wired
// to entityID. Centralises the label + actionKind stamp so slot routing
// and the legacy fallback emit identical atoms.
func actionAtom(kind domain.UserActionKind, entityType domain.EntityType, entityID string) Node {
	return buttonAtom(actionLabel(kind), kind, domain.UserAction{
		Kind: kind,
		Entity: &domain.EntityRef{
			Type: entityType,
			ID:   entityID,
		},
	})
}

// actionLabel maps a default action kind to its glyph label. "♥" for like,
// "+" for cart_add — the same glyphs the pass has always emitted.
func actionLabel(kind domain.UserActionKind) string {
	switch kind {
	case domain.UserActionLike:
		return "♥"
	case domain.UserActionCartAdd:
		return "+"
	}
	return ""
}

// buttonAtom builds a text atom with wrapper="button" + the action
// payload. Matches the wire shape the frontend wrapper.js dispatches.
// actionKind is stamped alongside so the frontend can style per kind
// (data-action-kind) without re-deriving it from action.kind.
func buttonAtom(label string, kind domain.UserActionKind, act domain.UserAction) Node {
	return Node{
		"id":           GenerateID(),
		"type":         NodeTypeText,
		"content":      label,
		attrWrapper:    "button",
		attrActionKind: string(kind),
		attrAction:     userActionToMap(act),
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
