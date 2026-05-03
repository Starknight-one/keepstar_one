import NodeRenderer from '../NodeRenderer'
import { useRenderContext } from '../RenderContext'
import { dispatchAction } from '../actionDispatch'

// Frame — flexbox container. Reads node.layout for {direction, gap,
// align, justify}. Recurses into children. Empty children → empty box
// (still rendered so layout placeholders work).
//
// Replicate clones (frames carrying __templateOrigin from the engine
// fan-out) get an entire-card click handler that fires drill_detail
// when prefetch has a drill template for the bound entity. Inner
// buttons stopPropagation so their clicks don't bubble to the card.

export default function Frame({ node }) {
  const ctx = useRenderContext()
  const layout = node.layout || {}
  const children = Array.isArray(node.children) ? node.children : []

  const drillProps = computeDrillProps(node, ctx)

  return (
    <div
      className={drillProps ? 'kw-frame kw-frame--clickable' : 'kw-frame'}
      data-direction={layout.direction || 'column'}
      data-gap={layout.gap || ''}
      data-align={layout.align || ''}
      data-justify={layout.justify || ''}
      data-id={node.id || ''}
      role={drillProps ? 'button' : undefined}
      tabIndex={drillProps ? 0 : undefined}
      onClick={drillProps ? drillProps.onClick : undefined}
      onKeyDown={drillProps ? drillProps.onKeyDown : undefined}
    >
      {children.map((c, i) => (
        <NodeRenderer key={c?.id || i} node={c} />
      ))}
    </div>
  )
}

// computeDrillProps returns onClick + onKeyDown handlers if this
// frame is a replicate clone with a drillable entity. Returns null
// when no drill is possible (not a clone, no prefetch, no matching
// entity). Avoids attaching empty handlers in the common case.
function computeDrillProps(node, ctx) {
  const origin = node.__templateOrigin
  if (!origin) return null
  const entityType = inferEntityType(ctx)
  const tpl = ctx?.prefetch?.adjacentTemplate?.[entityType]
  if (!tpl) return null
  const entityId = resolveEntityIdFromClone(node, ctx, entityType)
  if (!entityId) return null

  const fire = () => {
    dispatchAction(
      {
        kind: 'drill_detail',
        entity: { type: entityType, id: entityId },
      },
      ctx,
    )
  }
  return {
    onClick: (e) => {
      e.stopPropagation()
      fire()
    },
    onKeyDown: (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        fire()
      }
    },
  }
}

// inferEntityType picks the entity type for the current prefetch.
// Today only `product` exists in SystemAdjacency; future categories /
// services will need a richer signal (likely from
// state.view.focused.type or a per-clone `data-entity-type` stamp).
function inferEntityType(ctx) {
  const keys = Object.keys(ctx?.prefetch?.entities || {})
  if (keys.length === 0) return 'product'
  return keys[0]
}

// resolveEntityIdFromClone uses the clone's dataIndex (stamped by
// engine.fanOut) to look up the matching entity in the prefetch
// entities list. Falls back to walking children for the first
// `__bound` text node id when dataIndex is missing — defensive only.
function resolveEntityIdFromClone(node, ctx, entityType) {
  const entities = ctx?.prefetch?.entities?.[entityType] || []
  if (entities.length === 0) return null
  const idx = node.dataIndex
  if (typeof idx === 'number' && entities[idx]?.id) {
    return entities[idx].id
  }
  return null
}
