import NodeRenderer from '../NodeRenderer'
import { useRenderContext } from '../RenderContext'
import { dispatchAction } from '../actionDispatch'

// Frame — flexbox container. Reads:
//   layout.{direction, gap, align, justify, wrap}  → flexbox knobs
//   width / minWidth / maxWidth                    → inline-style sizing
//                                                    (numbers → px,
//                                                    strings pass through)
// Recurses into children. Empty children → empty box (placeholders OK).
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
  const styleProps = sizingStyle(node)

  return (
    <div
      className={drillProps ? 'kw-frame kw-frame--clickable' : 'kw-frame'}
      data-direction={layout.direction || 'column'}
      data-gap={layout.gap || ''}
      data-align={layout.align || ''}
      data-justify={layout.justify || ''}
      data-wrap={layout.wrap ? 'true' : ''}
      data-id={node.id || ''}
      style={styleProps}
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

// sizingStyle turns node.width / minWidth / maxWidth into an inline
// style object. Number → "<n>px"; string → pass-through (lets seeds
// emit "100%", "50vw", etc. when they need to). Returns undefined when
// no sizing is set so React doesn't add an empty style attribute.
function sizingStyle(node) {
  const out = {}
  const w = sizeValue(node.width)
  if (w !== undefined) out.width = w
  const mw = sizeValue(node.maxWidth)
  if (mw !== undefined) out.maxWidth = mw
  const mn = sizeValue(node.minWidth)
  if (mn !== undefined) out.minWidth = mn
  return Object.keys(out).length > 0 ? out : undefined
}

function sizeValue(v) {
  if (v === undefined || v === null) return undefined
  if (typeof v === 'number') return `${v}px`
  if (typeof v === 'string' && v !== '') return v
  return undefined
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
