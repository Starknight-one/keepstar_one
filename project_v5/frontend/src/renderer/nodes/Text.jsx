import { formatValue } from '../format'
import { wrapText } from '../wrapper'

// Text — leaf node carrying `content`, `format`, `wrapper`, `textStyle`.
// content is the raw bound value (string, number, or array post-binding);
// format turns it into a display string; wrapper applies a stylistic
// envelope (badge / button / etc.).

export default function Text({ node }) {
  const formatted = formatValue(node.content, node.format)
  const wrapped = wrapText(formatted, node.wrapper, node)

  const ts = node.textStyle || {}
  const lc = ts.lineClamp
  const styleProps = lc ? { '--clamp': String(lc) } : undefined

  return (
    <p
      className="kw-text"
      data-id={node.id || ''}
      data-fontsize={ts.fontSize || ''}
      data-fontweight={ts.fontWeight || ''}
      data-lineclamp={lc || ''}
      style={{
        color: ts.color || undefined,
        lineHeight: ts.lineHeight || undefined,
        ...styleProps,
      }}
    >
      {wrapped}
    </p>
  )
}
