import { firstSolidColor } from '../fills'

// Line — divider leaf. Diverges from v9 (which draws the bounding-box
// diagonal): in chat output a line is always a straight horizontal
// rule, or vertical when node.height (number) > node.width.
// Thickness = stroke.thickness, color = first solid stroke fill.

export default function Line({ node }) {
  const stroke = node.stroke || {}
  const thickness = stroke.thickness || 1
  const color = firstSolidColor(stroke.fill) || 'var(--kw-line, #e5e7eb)'
  const w = typeof node.width === 'number' ? node.width : undefined
  const h = typeof node.height === 'number' ? node.height : undefined
  const vertical = h !== undefined && w !== undefined && h > w

  const style = vertical
    ? { width: `${thickness}px`, height: `${h}px`, background: color }
    : {
        height: `${thickness}px`,
        width: w !== undefined ? `${w}px` : '100%',
        background: color,
      }

  return (
    <div
      className="kw-line"
      role="separator"
      aria-orientation={vertical ? 'vertical' : undefined}
      data-id={node.id || ''}
      style={style}
    />
  )
}
