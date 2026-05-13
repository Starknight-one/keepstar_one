import NodeRenderer from './NodeRenderer'
import RendererErrorBoundary from './RendererErrorBoundary'

// SceneGraphRenderer — top-level walker for a V5 engine.Document.
// Receives the parsed Document (`{version, children: [Node]}`) and
// dispatches each top-level child to NodeRenderer.
//
// Filters out reusable component definitions — they live at the
// document root for the engine resolver to find but should never reach
// the user's screen.
//
// Logs a one-line debug summary per render so devtools shows what
// shape just landed. Pairs with the [v5-renderer] prefix throughout.

export default function SceneGraphRenderer({ document }) {
  if (!document || !Array.isArray(document.children)) {
    return null
  }

  const renderable = document.children.filter((c) => !c?.reusable)

  // eslint-disable-next-line no-console
  console.debug('[v5-renderer] document', {
    nodeCount: countNodes(renderable),
    topLevelCount: renderable.length,
    topLevelTypes: renderable.map((c) => c?.type),
  })

  return (
    <RendererErrorBoundary resetKey={document}>
      <div className="kw-display-inner">
        {renderable.map((node, i) => (
          <NodeRenderer key={node.id || i} node={node} />
        ))}
      </div>
    </RendererErrorBoundary>
  )
}

function countNodes(nodes) {
  let n = 0
  const walk = (xs) => {
    if (!Array.isArray(xs)) return
    for (const x of xs) {
      n += 1
      if (Array.isArray(x?.children)) walk(x.children)
    }
  }
  walk(nodes)
  return n
}
