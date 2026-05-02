package engine

// Binding fills atom values on a scene-graph from per-record data maps.
//
// Mechanism:
//   - A node opts in to binding by carrying a `fieldBinding` (string)
//     attribute. Value is a key into the data map (free-form, not a fixed
//     vocabulary — same as V4 fieldName).
//   - A node carrying a `dataIndex` (int) attribute selects which entity
//     from the data slice to bind against. Without `dataIndex`, the node
//     binds to data[0]. (Replication that sets dataIndex per clone is
//     chunk-4 work; chunk 3 just respects the marker if present.)
//   - Resolved values land in the right target attribute by node type
//     (TextNode → `content`; other types → `value` fallback).
//   - The node gets `__bound: true` only when a value actually landed.
//     This is the V5 fix for V4 Грабля #1 (V4 set __bound blindly).
//
// Nodes without `fieldBinding` are skipped entirely — that's how
// LLM-static text (TextNode with literal `content`) survives binding
// without modification.

const (
	attrFieldBinding = "fieldBinding"
	attrDataIndex    = "dataIndex"
	attrBound        = "__bound"
	attrContent      = "content"
	attrValue        = "value"
)

// BindResult is diagnostics from one BindData run. Use it in tests, or
// later in tracing to see which atoms ended up bound vs missing data.
type BindResult struct {
	Bound   []string // node ids whose binding resolved
	Missing []string // node ids that asked for a field not present in their data record
	Skipped []string // node ids without fieldBinding (literal/text-explainer atoms)
}

// BindData walks the entire document tree (including children of expanded
// RefNodes) and fills bindable nodes from data. Mutates the document in
// place. Empty data slice is a no-op.
func BindData(doc *Document, data []map[string]any) BindResult {
	res := BindResult{}
	if doc == nil {
		return res
	}
	for _, child := range doc.Children {
		bindNodeRecursive(child, data, &res)
	}
	return res
}

func bindNodeRecursive(n Node, data []map[string]any, res *BindResult) {
	if n == nil {
		return
	}
	bindOneNode(n, data, res)
	if HasChildren(n) {
		for _, child := range Children(n) {
			bindNodeRecursive(child, data, res)
		}
	}
}

// bindOneNode looks at one node, decides what to do with it. Three paths:
//   - no fieldBinding → skip (literal node)
//   - fieldBinding but no data record at index → mark missing
//   - fieldBinding + record + key present → write value, mark bound
//   - fieldBinding + record but key absent → mark missing (do NOT mark bound)
func bindOneNode(n Node, data []map[string]any, res *BindResult) {
	id := nodeID(n)
	binding, ok := stringAttr(n, attrFieldBinding)
	if !ok || binding == "" {
		res.Skipped = append(res.Skipped, id)
		return
	}

	idx := dataIndexOf(n)
	if idx < 0 || idx >= len(data) {
		res.Missing = append(res.Missing, id)
		return
	}
	record := data[idx]

	val, present := record[binding]
	if !present || val == nil {
		res.Missing = append(res.Missing, id)
		return
	}

	target := bindTargetForNode(n)
	n[target] = val
	n[attrBound] = true
	res.Bound = append(res.Bound, id)
}

// bindTargetForNode returns the attribute key that should receive the
// resolved value for this node type. TextNode uses `content` (matches v9
// and chunk-1 conventions). Other node types fall back to a generic
// `value` key for now — chunk 4 will introduce image-fill binding when the
// first preset needs it.
func bindTargetForNode(n Node) string {
	if t, ok := stringAttr(n, "type"); ok && t == NodeTypeText {
		return attrContent
	}
	return attrValue
}

// dataIndexOf reads the `dataIndex` attribute. Returns 0 (default) when
// absent so a non-replicated node binds to data[0]. Returns -1 only if the
// attribute is present but malformed.
func dataIndexOf(n Node) int {
	v, ok := n[attrDataIndex]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64: // JSON-unmarshal always lands here
		return int(x)
	default:
		return -1
	}
}

func stringAttr(n Node, key string) (string, bool) {
	v, ok := n[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func nodeID(n Node) string {
	if id, ok := stringAttr(n, "id"); ok {
		return id
	}
	return "<no-id>"
}
