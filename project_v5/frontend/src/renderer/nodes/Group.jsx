import NodeRenderer from '../NodeRenderer'

// Group — plain wrapper. v9 distinguishes group from frame for
// transformation/canvas semantics; in V5 chat output we treat them
// the same — a vertical stack of children.

export default function Group({ node }) {
  const children = Array.isArray(node.children) ? node.children : []
  return (
    <div className="kw-frame" data-direction="column" data-id={node.id || ''}>
      {children.map((c, i) => (
        <NodeRenderer key={c?.id || i} node={c} />
      ))}
    </div>
  )
}
