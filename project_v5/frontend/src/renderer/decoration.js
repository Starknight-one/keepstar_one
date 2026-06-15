// Shared decoration helpers — turn the optional visual/position props a
// node may carry into inline-style fragments. Nodes are untyped property
// bags end-to-end (the engine passes new props through verbatim), so any
// node type can opt into these by spreading the returned style object.
//
// Every helper is additive and absence-safe: a node with none of these
// props produces an empty object, so untouched nodes render byte-identically
// to before. Numbers become "<n>px"; strings pass through (so seeds can emit
// "100%", "50vw", "full", etc.).

import { firstSolidColor } from './fills'

// pxOrPass — number → "<n>px", non-empty string → pass-through, else
// undefined. The single coercion used across sizing / position / radius.
function pxOrPass(v) {
  if (typeof v === 'number') return `${v}px`
  if (typeof v === 'string' && v !== '') return v
  return undefined
}

// radiusValue — cornerRadius shapes: number → "Npx"; [tl,tr,br,bl] → the
// four-corner shorthand. Mirrors Rectangle.jsx so every node rounds the
// same way. "full" → a large pill radius.
export function radiusValue(r) {
  if (typeof r === 'number') return `${r}px`
  if (r === 'full') return '9999px'
  if (Array.isArray(r) && r.length === 4) {
    return r.map((v) => (typeof v === 'number' ? `${v}px` : v)).join(' ')
  }
  return undefined
}

// paddingValue — layout.padding shapes (v9 / engine NormalizePadding):
//   number        → all four sides
//   [v, h]        → vertical, horizontal
//   [t, r, b, l]  → explicit per-side
// Numbers → px; strings pass through. Returns undefined when absent.
function paddingValue(p) {
  if (typeof p === 'number') return `${p}px`
  if (Array.isArray(p)) {
    if (p.length === 2 || p.length === 4) {
      return p.map((v) => (typeof v === 'number' ? `${v}px` : v)).join(' ')
    }
  }
  return undefined
}

// effectShadow — maps the first outer drop-shadow effect to a CSS
// box-shadow. Effect shape (v9 / engine value_effect): {type:"shadow",
// shadowType?:"outer", offset:{x,y}|[x,y], blur, spread?, color}. Inner
// shadows get the "inset" keyword. Returns undefined when no shadow.
function effectShadow(effects) {
  const list = Array.isArray(effects) ? effects : effects ? [effects] : []
  for (const e of list) {
    if (!e || typeof e !== 'object') continue
    if (e.type !== 'shadow' && e.type !== undefined) continue
    if (e.type === undefined && e.blur === undefined && e.offset === undefined) {
      continue
    }
    const { x, y } = readOffset(e.offset)
    const blur = num(e.blur, 0)
    const spread = num(e.spread, 0)
    const color = firstSolidColor(e.color) || 'rgba(0,0,0,0.12)'
    const inset = e.shadowType === 'inner' ? 'inset ' : ''
    return `${inset}${x}px ${y}px ${blur}px ${spread}px ${color}`
  }
  return undefined
}

function readOffset(offset) {
  if (Array.isArray(offset)) {
    return { x: num(offset[0], 0), y: num(offset[1], 0) }
  }
  if (offset && typeof offset === 'object') {
    return { x: num(offset.x, 0), y: num(offset.y, 0) }
  }
  return { x: 0, y: 0 }
}

function num(v, fallback) {
  return typeof v === 'number' && Number.isFinite(v) ? v : fallback
}

// decorationStyle — the visual envelope a container/leaf node may carry:
//   fill          → solid background (same firstSolidColor helper as Rectangle)
//   cornerRadius  → border-radius
//   stroke        → {thickness, fill} → border
//   effect        → outer/inner drop shadow → box-shadow
//   clip          → overflow hidden (clip:true)
//   layout.padding→ inner padding
// Returns a plain object (possibly empty) safe to spread into a style.
export function decorationStyle(node) {
  const out = {}
  const layout = node.layout || {}

  const bg = firstSolidColor(node.fill)
  if (bg) out.background = bg

  const radius = radiusValue(node.cornerRadius)
  if (radius !== undefined) out.borderRadius = radius

  if (node.stroke && typeof node.stroke === 'object') {
    const color = firstSolidColor(node.stroke.fill)
    if (color) out.border = `${node.stroke.thickness || 1}px solid ${color}`
  }

  const shadow = effectShadow(node.effect)
  if (shadow) out.boxShadow = shadow

  if (node.clip === true) out.overflow = 'hidden'

  const pad = paddingValue(layout.padding)
  if (pad !== undefined) out.padding = pad

  return out
}

// hasAbsoluteChild — does any direct child carry position:"absolute"?
// A parent with one must become position:relative so the overlay anchors
// to it. Direct children only (matches CSS containing-block semantics).
export function hasAbsoluteChild(node) {
  const kids = Array.isArray(node?.children) ? node.children : []
  return kids.some((c) => c && c.position === 'absolute')
}

// positionStyle — the inline position envelope for an overlay node
// (position:"absolute" + any of top/left/right/bottom/zIndex). Numbers →
// px, strings pass through. Returns {} when the node is not positioned, so
// flow nodes are untouched.
export function positionStyle(node) {
  if (!node || node.position !== 'absolute') return {}
  const out = { position: 'absolute' }
  const top = pxOrPass(node.top)
  if (top !== undefined) out.top = top
  const left = pxOrPass(node.left)
  if (left !== undefined) out.left = left
  const right = pxOrPass(node.right)
  if (right !== undefined) out.right = right
  const bottom = pxOrPass(node.bottom)
  if (bottom !== undefined) out.bottom = bottom
  if (typeof node.zIndex === 'number') out.zIndex = node.zIndex
  return out
}

// relativeStyle — {position:'relative'} when the node hosts any absolute
// child, else {}. Spread onto the parent so overlays anchor to it. Never
// overrides an explicit position the node already set for itself.
export function relativeStyle(node) {
  return hasAbsoluteChild(node) ? { position: 'relative' } : {}
}
