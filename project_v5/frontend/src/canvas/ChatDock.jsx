import { useEffect, useRef } from 'react'

// ChatDock — the chat as a floating dock over the canvas (V2_SPEC §2
// step 3, mock-demo birthDesktop): a rounded bar docked low, with the
// history collapsing upward above it, translucent over the canvas.
//
// The history is height-bounded (CSS: recent turns visible, the rest
// behind a scroll) so the dock can never permanently obscure the stage.
// `children` is the shell's input form (chips + composer) — the dock
// styles it, it does not own it.
export default function ChatDock({ items, children }) {
  const historyRef = useRef(null)

  // Follow the newest line — the history grows upward from the bar. The dep
  // is the LENGTH, not the array: `items` is rebuilt on every render, so
  // depending on it would yank a user who scrolled up back to the bottom on
  // every keystroke in the composer.
  useEffect(() => {
    const el = historyRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [items.length])

  return (
    <div className="kw-dock">
      <div className="kw-dock-history" ref={historyRef}>
        {items.map((it) => (
          <div key={it.key} className={'kw-msg kw-dock-msg ' + dockRoleClass(it.role)}>
            {it.text}
          </div>
        ))}
      </div>
      {children}
    </div>
  )
}

function dockRoleClass(role) {
  if (role === 'user') return 'kw-msg-user'
  if (role === 'error') return 'kw-msg-error'
  if (role === 'status') return 'kw-msg-status'
  return 'kw-msg-bot'
}
