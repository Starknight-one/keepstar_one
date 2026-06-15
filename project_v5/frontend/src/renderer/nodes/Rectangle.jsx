import { firstSolidColor } from '../fills'
import { positionStyle } from '../decoration'

// Rectangle — decorative leaf node. Reads:
//   fills        → first solid color → background (gradient/image/mesh
//                  ignored; image-filled rectangles arrive as `image`)
//   cornerRadius → number → "Npx"; array of 4 → "tl tr br bl" px
//   stroke       → {thickness, fill} → border
//   width / height / minWidth / maxWidth → numbers → px, strings pass
//   opacity      → inline style
//   position:"absolute" + top/left/right/bottom/zIndex → overlay
// No children rendered (leaf).

export default function Rectangle({ node }) {
  return <div className="kw-rect" data-id={node.id || ''} style={rectStyle(node)} />
}

function rectStyle(node) {
  const out = {}
  const bg = firstSolidColor(node.fills)
  if (bg) out.background = bg
  const radius = radiusValue(node.cornerRadius)
  if (radius !== undefined) out.borderRadius = radius
  if (node.stroke && typeof node.stroke === 'object') {
    const color = firstSolidColor(node.stroke.fill)
    if (color) out.border = `${node.stroke.thickness || 1}px solid ${color}`
  }
  const w = sizeValue(node.width)
  if (w !== undefined) out.width = w
  const h = sizeValue(node.height)
  if (h !== undefined) out.height = h
  const mw = sizeValue(node.maxWidth)
  if (mw !== undefined) out.maxWidth = mw
  const mn = sizeValue(node.minWidth)
  if (mn !== undefined) out.minWidth = mn
  if (typeof node.opacity === 'number') out.opacity = node.opacity
  Object.assign(out, positionStyle(node))
  return Object.keys(out).length > 0 ? out : undefined
}

function radiusValue(r) {
  if (typeof r === 'number') return `${r}px`
  if (Array.isArray(r) && r.length === 4) {
    return r.map((v) => `${v}px`).join(' ')
  }
  return undefined
}

function sizeValue(v) {
  if (v === undefined || v === null) return undefined
  if (typeof v === 'number') return `${v}px`
  if (typeof v === 'string' && v !== '') return v
  return undefined
}
