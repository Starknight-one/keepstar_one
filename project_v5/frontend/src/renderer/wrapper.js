// Wrapper pass — wrap the formatted content into a styled element
// (badge / tag / pill / button / link / alert). Mirrors V5's
// Agent2 wrapper vocabulary.
//
// Buttons + links carrying an `action` prop dispatch through
// actionDispatch; the click ctx flows in via React Context
// (RenderContext). Buttons without an action keep the no-op
// console.log path so freestyle LLM-emitted buttons are still visible
// during diagnostics.
//
// stopPropagation on the button click prevents the parent Frame's
// card-body click (drill_detail, see Frame.jsx) from firing too —
// inner buttons own their click.

import { createElement } from 'react'
import { useRenderContext } from './RenderContext'
import { dispatchAction, isLiked, isInCart } from './actionDispatch'

export function wrapText(content, wrapper, node) {
  if (!wrapper || wrapper === 'none') return content
  return createElement(WrappedContent, { content, wrapper, node })
}

// activeFor reports whether an action button is in its "on" state
// (entity already liked / already in cart) for active styling + a11y.
function activeFor(node, actionState) {
  const kind = node?.action?.kind
  const id = node?.action?.entity?.id
  if (!kind || !id) return false
  if (kind === 'like') return isLiked(actionState, id)
  if (kind === 'cart_add') return isInCart(actionState, id)
  return false
}

function WrappedContent({ content, wrapper, node }) {
  const ctx = useRenderContext()
  const className = `kw-${wrapper}`

  switch (wrapper) {
    case 'button': {
      const active = activeFor(node, ctx.actionState)
      return createElement(
        'button',
        {
          className: active ? `${className} kw-active` : className,
          type: 'button',
          'aria-pressed': node?.action?.kind === 'like' || node?.action?.kind === 'cart_add' ? active : undefined,
          // actionKind (stamped by InjectDefaultActions) drives per-kind
          // styling via [data-action-kind] — black round add, translucent
          // round like. Absent → the neutral default .kw-button.
          'data-action-kind': node?.actionKind || undefined,
          onClick: (e) => {
            e.stopPropagation()
            if (node?.action) {
              dispatchAction(node.action, ctx)
              return
            }
            // Diagnostic: button without explicit action and no
            // auto-injected default. Keeps freestyle LLM atoms
            // visible during dev.
            // eslint-disable-next-line no-console
            console.log('[v5-action] button without action prop', { nodeId: node?.id, content })
          },
        },
        content,
      )
    }
    case 'link':
      return createElement(
        'a',
        {
          className,
          href: node?.action?.params?.url || '#',
          onClick: (e) => {
            e.stopPropagation()
            if (node?.action) {
              e.preventDefault()
              dispatchAction(node.action, ctx)
              return
            }
            e.preventDefault()
            // eslint-disable-next-line no-console
            console.log('[v5-action] link without action prop', { nodeId: node?.id })
          },
        },
        content,
      )
    case 'badge':
    case 'tag':
    case 'pill':
    case 'alert':
      return createElement('span', { className }, content)
    default:
      return content
  }
}
