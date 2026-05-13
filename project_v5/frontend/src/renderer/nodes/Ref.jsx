import NodeRenderer from '../NodeRenderer'

// Ref — by the time the document reaches the frontend, the backend has
// already replaced ref nodes with their resolved component subtrees
// (see engine.ResolveAndInline). So in practice we rarely see "type":"ref"
// here; if we do, it means the ref didn't resolve (component missing).
// Render the children if any, otherwise nothing.

export default function Ref({ node }) {
  const children = Array.isArray(node.children) ? node.children : []
  if (children.length === 0) return null
  return (
    <div className="kw-frame" data-direction="column" data-id={node.id || ''}>
      {children.map((c, i) => (
        <NodeRenderer key={c?.id || i} node={c} />
      ))}
    </div>
  )
}
