package domain

// UserActionKind enumerates the closed vocabulary of click-handlers the
// frontend can emit. The set is deliberately small: every kind has a
// matching backend handler (like / cart) or a matching frontend
// dispatcher (drill / back / open / external / search). LLM-emitted
// actions on rendered atoms must use one of these names; unknown kinds
// reject at the action endpoint.
//
// New kinds = explicit enum extension + paired handler/dispatcher.
// Tenants do NOT pick action kinds — that's a shop-layer decision, not
// a chat-layer decision.
type UserActionKind string

const (
	UserActionLike         UserActionKind = "like"
	UserActionUnlike       UserActionKind = "unlike"
	UserActionCartAdd      UserActionKind = "cart_add"
	UserActionCartRemove   UserActionKind = "cart_remove"
	UserActionDrillDetail  UserActionKind = "drill_detail"
	UserActionBack         UserActionKind = "back"
	UserActionOpenCategory UserActionKind = "open_category"
	UserActionExternalLink UserActionKind = "external_link"
	UserActionSearch       UserActionKind = "search"
)

// UserAction is the runtime payload attached to button atoms in a
// scene-graph Document. Lives directly on the atom node (not on a
// parent widget — V5 has no widget concept, the scene-graph itself
// is the structure).
//
// Wire shape on a button atom:
//
//	{type:"text", wrapper:"button", action:{kind:"like",
//	 entity:{type:"product", id:"abc"}, params:{...}}}
//
// Entity is optional for kinds that don't bind to one entity (search,
// external_link, back, open_category). Params is a free-form bag for
// kind-specific arguments (e.g. {url} for external_link, {query} for
// search, {categoryId} for open_category, {quantity} for cart_add).
type UserAction struct {
	Kind   UserActionKind         `json:"kind"`
	Entity *EntityRef             `json:"entity,omitempty"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// IsBackendHandled reports whether this action kind is processed by the
// POST /api/v1/actions endpoint. The other kinds are frontend-only
// (drill / back / search / external_link / open_category — they route
// through dispatcher logic, not the actions endpoint).
func (k UserActionKind) IsBackendHandled() bool {
	switch k {
	case UserActionLike, UserActionUnlike, UserActionCartAdd, UserActionCartRemove:
		return true
	}
	return false
}

// IsKnown reports whether s is one of the nine recognised kinds.
// Used by handlers to reject unknown kinds with 400 instead of
// silently no-op'ing.
func (k UserActionKind) IsKnown() bool {
	switch k {
	case UserActionLike, UserActionUnlike, UserActionCartAdd, UserActionCartRemove,
		UserActionDrillDetail, UserActionBack, UserActionOpenCategory,
		UserActionExternalLink, UserActionSearch:
		return true
	}
	return false
}
