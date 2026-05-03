package engine

// Replicate fan-out: turns a subtree marked replicate:true into N clones,
// one per data record. Runs after ApplyOps and before ComponentResolver
// expansion + BindData. Replication is a render-time projection of preset
// + data shape; it has no caller that needs to undo it (the LLM never sees
// the expanded tree, the canvas edits the un-replicated source). That's
// why it lives as a plain function rather than a Command on history.
//
// Each clone:
//   - drops the replicate marker (so a re-run is idempotent),
//   - gets `dataIndex: i` stamped on its root only — descendants inherit
//     the index through the binding walker (binding.go),
//   - has fresh IDs minted on every node in its subtree, so id-keyed ops
//     (Override, FindNodeByID, ComponentResolver.Expand at line ~98 which
//     forces clone.id = refNode.id) don't collide across siblings.
//
// Nested replicate:true inside an already-replicated subtree is intentionally
// skipped — see docs/v5-known-gaps.md. We still strip those nested markers
// during the clone so they don't leave inert noise in the document.

const attrReplicate = "replicate"

// ExpandReplicates walks the document, finds every node carrying
// replicate:true, and replaces each marker with `count` deep-cloned siblings
// in its parent's children slice. Mutates doc in place. Idempotent: a
// second call is a no-op (markers were stripped by the first).
func ExpandReplicates(doc *Document, count int) {
	if doc == nil {
		return
	}
	expandIn(doc, count)
}

// expandIn processes one parent's children: replicate markers get fanned
// out; non-marker frame/group children are recursed into so markers nested
// in non-replicated subtrees still get expanded.
func expandIn(parent any, count int) {
	children := childrenOf(parent)
	if len(children) == 0 {
		return
	}

	out := make([]Node, 0, len(children))
	changed := false
	for _, child := range children {
		if isReplicateMarker(child) {
			out = append(out, fanOut(child, count)...)
			changed = true
			continue
		}
		// Recurse into ordinary containers; their children list is mutated
		// in place via SetParentChildren when needed.
		if HasChildren(child) {
			expandIn(child, count)
		}
		out = append(out, child)
	}
	if changed {
		SetParentChildren(parent, out)
	}
}

// fanOut produces `count` independent deep-clones of template. count == 0
// returns nil — the marker disappears with no replacement, matching the
// "data was empty" case.
//
// Each clone carries `__templateOrigin: <template id>` so post-expand
// consumers (tree_map builder, future constraints engine for cross-widget
// rules) can group clones back to their source. Lightweight cousin of the
// V4 GroupID — see docs/v5-known-gaps.md for the full GroupID port.
func fanOut(template Node, count int) []Node {
	if count <= 0 {
		return nil
	}
	originID, _ := template["id"].(string)
	out := make([]Node, count)
	for i := 0; i < count; i++ {
		clone := cloneNode(template)
		clearReplicateMarkers(clone)
		clone[attrDataIndex] = i
		reIDSubtree(clone)
		if originID != "" {
			clone[attrTemplateOrigin] = originID
		}
		out[i] = clone
	}
	return out
}

// attrTemplateOrigin is the cloned-root attribute that records the
// id of the template node fanOut copied. Used by tree_map and (later)
// the constraints engine.
const attrTemplateOrigin = "__templateOrigin"

// isReplicateMarker reports whether n carries replicate:true. Anything
// else (false, missing, wrong type) is not a marker.
func isReplicateMarker(n Node) bool {
	v, ok := n[attrReplicate]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// clearReplicateMarkers walks root + descendants and deletes any
// replicate:true attribute. Used during fan-out so nested markers inside a
// clone don't survive as inert noise.
func clearReplicateMarkers(n Node) {
	delete(n, attrReplicate)
	if HasChildren(n) {
		for _, c := range Children(n) {
			clearReplicateMarkers(c)
		}
	}
}

// reIDSubtree mints fresh IDs on n and every descendant. Without this, the
// N clones share the original template's IDs and any post-replicate id
// lookup (FindNodeByID, ComponentResolver.expandRef which copies refNode.id
// onto the clone root, override targeting) collapses onto the first clone.
func reIDSubtree(n Node) {
	n["id"] = GenerateID()
	if HasChildren(n) {
		for _, c := range Children(n) {
			reIDSubtree(c)
		}
	}
}
