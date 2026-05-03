import NodeRenderer from '../NodeRenderer'

// Frame — flexbox container. Reads node.layout for {direction, gap,
// align, justify}. Recurses into children. Empty children → empty box
// (still rendered so layout placeholders work).

export default function Frame({ node }) {
  const layout = node.layout || {}
  const children = Array.isArray(node.children) ? node.children : []

  return (
    <div
      className="kw-frame"
      data-direction={layout.direction || 'column'}
      data-gap={layout.gap || ''}
      data-align={layout.align || ''}
      data-justify={layout.justify || ''}
      data-id={node.id || ''}
    >
      {children.map((c, i) => (
        <NodeRenderer key={c?.id || i} node={c} />
      ))}
    </div>
  )
}
