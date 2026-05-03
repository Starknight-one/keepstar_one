// fillTemplate — port of V4 fillFormation for the V5 scene-graph
// shape. Walks a prefetched template (one Document per entity type)
// and fills any node carrying a `fieldBinding` with the matching key
// on the supplied entity. Returns a fresh deep-cloned Document so the
// prefetch cache stays clean for re-use on the next click.
//
// Binding rules mirror the backend engine.BindData logic:
//   - `text` node     → write entity[fieldBinding] to `content`
//   - `image` node    → write resolved URL to `fills[0].url` (with
//                       `type: "image"` + `mode: "fill"` defaults)
//   - other types     → write to `value`
//
// Skip nodes carrying `__resolvedFrom` — those are component
// subtrees already inlined and bound at template-build time on the
// backend (their bindings refer to a different entity context). For
// V5 chunk 11 the backend's prefetch builder runs ResolveAndInline
// but does NOT call BindData, so component-bound atoms still have
// fieldBinding intact and will fill correctly here. The
// __resolvedFrom guard is in place for forward-compat.

export function fillTemplate(template, entity) {
  if (!template || typeof template !== 'object') return template
  if (!entity || typeof entity !== 'object') return template
  const cloned = deepClone(template)
  if (Array.isArray(cloned.children)) {
    for (const child of cloned.children) {
      bindNode(child, entity)
    }
  }
  return cloned
}

function bindNode(node, entity) {
  if (!node || typeof node !== 'object') return
  if (node.__resolvedFrom) return // skip already-resolved component subtrees
  const fb = node.fieldBinding
  if (typeof fb === 'string' && fb !== '') {
    const value = entity[fb]
    if (value !== undefined && value !== null) {
      writeBoundValue(node, value)
    }
  }
  if (Array.isArray(node.children)) {
    for (const c of node.children) bindNode(c, entity)
  }
}

function writeBoundValue(node, value) {
  const t = node.type
  if (t === 'image') {
    const url = Array.isArray(value) ? value[0] : value
    if (typeof url !== 'string' || url === '') return
    setImageFill(node, url)
    node.__bound = true
    return
  }
  if (t === 'text') {
    node.content = value
    node.__bound = true
    return
  }
  node.value = value
  node.__bound = true
}

function setImageFill(node, url) {
  const fill = { type: 'image', url, mode: 'fill' }
  if (!Array.isArray(node.fills) || node.fills.length === 0) {
    node.fills = [fill]
    return
  }
  const first = node.fills[0]
  if (first && typeof first === 'object') {
    first.type = 'image'
    first.url = url
    if (first.mode == null) first.mode = 'fill'
  } else {
    node.fills[0] = fill
  }
}

function deepClone(obj) {
  // structuredClone is available in jsdom 22+ + every browser we
  // target; cheap and exact.
  return structuredClone(obj)
}
