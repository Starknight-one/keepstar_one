// Wrapper pass — wrap the formatted content into a styled element
// (badge / tag / pill / button / link / alert). Mirrors V5's
// Agent2 wrapper vocabulary.
//
// Buttons are interactive: chunk-10 wires onClick to a no-op that
// console.logs the action ID for diagnostic. Real action wiring
// lands with P0-C (interaction loop).
//
// Returns a React element. Caller (Text node renderer) decides whether
// to wrap the formatted string or pass through.

import { createElement } from 'react'

export function wrapText(content, wrapper, node) {
  if (!wrapper || wrapper === 'none') return content

  const className = `kw-${wrapper}`
  switch (wrapper) {
    case 'button':
      return createElement(
        'button',
        {
          className,
          type: 'button',
          onClick: () => {
            // P0-C will replace this with an actions endpoint POST.
            // eslint-disable-next-line no-console
            console.log('[v5-action]', {
              wrapper,
              nodeId: node?.id,
              fieldBinding: node?.fieldBinding,
              content,
              hint: 'TODO: not wired (waiting for P0-C actions endpoint)',
            })
          },
        },
        content,
      )
    case 'link':
      return createElement(
        'a',
        {
          className,
          href: '#',
          onClick: (e) => {
            e.preventDefault()
            // eslint-disable-next-line no-console
            console.log('[v5-action]', { wrapper, nodeId: node?.id, content })
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
