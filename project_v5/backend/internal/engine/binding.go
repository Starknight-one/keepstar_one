package engine

// Binding fills atom values on a scene-graph from per-record data maps.
//
// Mechanism:
//   - A node opts in to binding by carrying a `fieldBinding` (string)
//     attribute. Value is a key into the data map (free-form, not a fixed
//     vocabulary — same as V4 fieldName).
//   - A node carrying a `dataIndex` (int) attribute selects which entity
//     from the data slice to bind against. The walker also INHERITS
//     `dataIndex` from the nearest ancestor that carries it — so a
//     replicated subtree only needs the marker on its root, and every
//     descendant atom binds against the same record. Without an explicit
//     or inherited `dataIndex`, a node binds to data[0].
//   - Resolved values land in the right target attribute by node type:
//     TextNode → `content`; ImageNode → `fills[0].image`; other types
//     fall back to a generic `value` key.
//   - The node gets `__bound: true` only when a value actually landed.
//     This is the V5 fix for V4 Грабля #1 (V4 set __bound blindly).
//
// Nodes without `fieldBinding` are skipped entirely — that's how
// LLM-static text (TextNode with literal `content`) survives binding
// without modification.

const (
	attrFieldBinding   = "fieldBinding"
	attrDataIndex      = "dataIndex"
	attrBound          = "__bound"
	attrContent        = "content"
	attrValue          = "value"
	attrFills          = "fills"
	attrFillType       = "type"
	attrFillImageURL   = "image" // payload key on a {type:"image", image:"<url>"} fill
	attrFillImageMode  = "mode"
	defaultImageMode   = "fill"
	attrReusable       = "reusable" // component definitions in a materialised doc
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
// place. Empty data slice is a no-op for value resolution but still
// records skipped/missing diagnostics.
func BindData(doc *Document, data []map[string]any) BindResult {
	res := BindResult{}
	if doc == nil {
		return res
	}
	const noInheritedIndex = -1
	for _, child := range doc.Children {
		bindNodeRecursive(child, data, noInheritedIndex, &res)
	}
	return res
}

// bindNodeRecursive walks the subtree rooted at n, threading the nearest
// ancestor's dataIndex down. inheritedIndex == -1 means "no ancestor has
// stamped a dataIndex"; in that case bindOneNode falls back to 0.
//
// Subtrees rooted at a node carrying `reusable: true` are skipped
// entirely: they are component definitions sitting at the top of a
// materialised document, not instances. Refs that consume them have
// already been inlined by ResolveAndInline before BindData runs, and the
// definitions themselves should not bind anything.
func bindNodeRecursive(n Node, data []map[string]any, inheritedIndex int, res *BindResult) {
	if n == nil {
		return
	}
	if reusable, _ := n[attrReusable].(bool); reusable {
		return
	}
	currentIndex := inheritedIndex
	if explicit, ok := readDataIndex(n); ok {
		currentIndex = explicit
	}
	bindOneNode(n, data, currentIndex, res)
	if HasChildren(n) {
		for _, child := range Children(n) {
			bindNodeRecursive(child, data, currentIndex, res)
		}
	}
}

// bindOneNode looks at one node, decides what to do with it. Four paths:
//   - no fieldBinding → skip (literal node)
//   - fieldBinding + index out of range → mark missing
//   - fieldBinding + record + key present → write value, mark bound
//   - fieldBinding + record but key absent → mark missing (do NOT mark bound)
func bindOneNode(n Node, data []map[string]any, inheritedIndex int, res *BindResult) {
	id := nodeID(n)
	binding, ok := stringAttr(n, attrFieldBinding)
	if !ok || binding == "" {
		res.Skipped = append(res.Skipped, id)
		return
	}

	idx := inheritedIndex
	if idx < 0 {
		idx = 0
	}
	if idx >= len(data) {
		res.Missing = append(res.Missing, id)
		return
	}
	record := data[idx]

	val, present := record[binding]
	if !present || val == nil {
		res.Missing = append(res.Missing, id)
		return
	}

	if NodeType(n) == NodeTypeImage {
		urlStr, ok := val.(string)
		if !ok {
			// Non-string image binding doesn't make sense; treat as miss.
			res.Missing = append(res.Missing, id)
			return
		}
		setImageFill(n, urlStr)
		n[attrBound] = true
		res.Bound = append(res.Bound, id)
		return
	}

	target := bindTargetForNode(n)
	n[target] = val
	n[attrBound] = true
	res.Bound = append(res.Bound, id)
}

// bindTargetForNode returns the attribute key that should receive the
// resolved value for non-image, non-text nodes. TextNode uses `content`;
// everything else falls back to `value`. Image nodes are routed to
// fills[0].image directly in bindOneNode.
func bindTargetForNode(n Node) string {
	if NodeType(n) == NodeTypeText {
		return attrContent
	}
	return attrValue
}

// setImageFill writes url into the first image fill on n. If n already has
// a non-empty fills array, the existing fill[0]'s image+type are updated
// (preserving any other fill metadata like opacity / mode). Otherwise a
// fresh fills array with one image fill is installed.
//
// Image-fill body shape: {type:"image", image:"<url>", mode:"fill"}.
// See docs/v5-known-gaps.md — this shape is V5's reasoned guess; payload
// key is centralised in attrFillImageURL so a single rename suffices if
// the v9 wire format uses a different key (e.g. "url" or "src").
func setImageFill(n Node, url string) {
	existing, _ := n[attrFills].([]any)
	if len(existing) == 0 {
		n[attrFills] = []any{
			map[string]any{
				attrFillType:      FillTypeImage,
				attrFillImageURL:  url,
				attrFillImageMode: defaultImageMode,
			},
		}
		return
	}
	first, ok := existing[0].(map[string]any)
	if !ok {
		// Existing slot isn't a map — replace it with a fresh image fill.
		existing[0] = map[string]any{
			attrFillType:      FillTypeImage,
			attrFillImageURL:  url,
			attrFillImageMode: defaultImageMode,
		}
		n[attrFills] = existing
		return
	}
	first[attrFillType] = FillTypeImage
	first[attrFillImageURL] = url
	if _, hasMode := first[attrFillImageMode]; !hasMode {
		first[attrFillImageMode] = defaultImageMode
	}
}

// readDataIndex returns the explicit `dataIndex` value on n. The bool is
// true only when n carries the attribute and it parses to a valid int.
// Malformed values surface as (-1, true) so the walker treats them as
// "marker present but unusable" rather than silently inheriting.
func readDataIndex(n Node) (int, bool) {
	v, ok := n[attrDataIndex]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64: // JSON-unmarshal always lands here
		return int(x), true
	default:
		return -1, true
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
