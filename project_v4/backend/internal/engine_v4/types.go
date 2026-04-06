package engine_v4

import "keepstar_v4/internal/domain"

// OpType defines supported tree operations.
type OpType string

const (
	OpInsert  OpType = "insert"
	OpUpdate  OpType = "update"
	OpDelete  OpType = "delete"
	OpMove    OpType = "move"
	OpReplace OpType = "replace"
)

// Op is a single operation on the formation tree.
type Op struct {
	Type   OpType                 `json:"op"`
	Target string                 `json:"target,omitempty"` // atom/node ID or field name (wildcard)
	Ref    string                 `json:"ref,omitempty"`    // binding name for chaining ($ref)
	Parent string                 `json:"parent,omitempty"` // parent ID or $ref for insert/move
	After  string                 `json:"after,omitempty"`  // position after sibling
	Props  map[string]interface{} `json:"props,omitempty"`
}

// ExecuteInput is the single entry point for all engine operations.
// Ops-only pipeline: build from scratch or modify existing.
type ExecuteInput struct {
	// Operations — build widgets from scratch or modify existing formation
	Ops []Op

	// Entity data for data-binding (atoms with FieldName get values from here)
	Data       []map[string]interface{}
	EntityType string // "product" or "service"

	// Existing formation to modify (nil = start fresh)
	Formation *domain.FormationWithData

	// Formation-level settings
	Layout  string // grid, list, single, carousel
	Columns int
	Size    string // tiny, small, medium, large

	// Replication control (B3) — explicit, no magic.
	Replicate bool // if true, replicate a single widget template across data items
	Limit     int  // max data items to use (0 = all)
}

// ExecuteOutput contains the result of engine execution.
type ExecuteOutput struct {
	Formation *domain.FormationWithData
	TreeMap   map[string]interface{} // compact context for Agent2 next turn
	Warnings  []string
}
