import Frame from './nodes/Frame'
import Group from './nodes/Group'
import Text from './nodes/Text'
import Image from './nodes/Image'
import Ref from './nodes/Ref'

// NodeRenderer — type dispatcher for V5 scene-graph nodes. Backend has
// already resolved refs (ResolveAndInline) and bound data (BindData)
// before the document reaches the frontend, so by the time we render:
//   frame / group → containers with children
//   text          → leaf with content + format + wrapper + textStyle
//   image         → leaf with fills[0].url or content URL
//   ref           → opaque resolved subtree (treated transparently)
//
// Anything else logs a warn and renders nothing — diagnostic for new
// node types the renderer doesn't handle yet (rectangle, ellipse, etc.).

export default function NodeRenderer({ node }) {
  if (!node || typeof node !== 'object') return null
  const t = node.type
  switch (t) {
    case 'frame':
      return <Frame node={node} />
    case 'group':
      return <Group node={node} />
    case 'text':
      return <Text node={node} />
    case 'image':
      return <Image node={node} />
    case 'ref':
      return <Ref node={node} />
    default:
      // eslint-disable-next-line no-console
      console.warn('[v5-renderer] unknown node type', t, node?.id)
      return null
  }
}
