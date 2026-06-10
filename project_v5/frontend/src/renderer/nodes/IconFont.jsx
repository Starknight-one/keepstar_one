import { ICONS } from '../icons'
import { firstSolidColor } from '../fills'

// IconFont — inline lucide icon leaf. Reads:
//   iconFontName  → lucide kebab-case id, looked up in the ICONS
//                   registry; unknown names warn + render nothing
//                   (mirrors the NodeRenderer default)
//   width/height  → numbers, default 16
//   fill          → first solid color → svg `color` (icons stroke
//                   with currentColor, so color drives the glyph)

export default function IconFont({ node }) {
  const name = node.iconFontName
  const glyph = ICONS[name]
  if (!glyph) {
    // eslint-disable-next-line no-console
    console.warn('[v5-renderer] unknown icon_font name', name, node?.id)
    return null
  }

  const color = firstSolidColor(node.fill)
  return (
    <svg
      className="kw-icon"
      data-id={node.id || ''}
      xmlns="http://www.w3.org/2000/svg"
      width={typeof node.width === 'number' ? node.width : 16}
      height={typeof node.height === 'number' ? node.height : 16}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      style={color ? { color } : undefined}
      aria-hidden="true"
    >
      {glyph}
    </svg>
  )
}
