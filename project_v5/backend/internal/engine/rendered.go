package engine

// RenderedDataIndices returns the distinct dataIndex values stamped on the
// document's top-level replicate clones, in encounter order. Used by Agent1
// to know which data rows are currently visible to the user so it can
// ground references like «у первого» / «у COSRX-кремА» without re-searching.
//
// The walker is intentionally narrow:
//   - Only top-level Children are inspected. Nested replicates (compose
//     mode) would surface their own clones at root after expansion, so the
//     caller still sees them.
//   - A clone is recognised by carrying both `__templateOrigin` (set by
//     fanOut) and a parseable `dataIndex`. Documents that pre-date replicate
//     (raw template, no expansion) yield an empty result — there's no
//     visible data subset to advertise.
//   - Reusable component definitions are skipped.
//
// Returns nil when the document has no replicate clones — caller can then
// omit the rendered block entirely instead of shipping `[]`.
func RenderedDataIndices(doc *Document) []int {
	if doc == nil || len(doc.Children) == 0 {
		return nil
	}
	seen := map[int]bool{}
	out := []int{}
	for _, child := range doc.Children {
		if isReusable(child) {
			continue
		}
		if origin, _ := child[attrTemplateOrigin].(string); origin == "" {
			continue
		}
		idx, ok := readDataIndex(child)
		if !ok || idx < 0 {
			continue
		}
		if seen[idx] {
			continue
		}
		seen[idx] = true
		out = append(out, idx)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
