import { createRoot } from 'react-dom/client'
import SceneGraphRenderer from './renderer/SceneGraphRenderer'
import RenderContext, { defaultRenderContext } from './renderer/RenderContext'
import { applyTheme, tokensFromDoc } from './theme-style'

// Single source of the shadow-DOM stylesheet + font for BOTH entries:
// widget.jsx (chat auto-mount) imports these, so the preview and the
// merchant widget can never drift apart on CSS.
import widgetCss from './widget.css?inline'

export const ALL_CSS = [widgetCss].join('\n')

export const FONT_HREF =
  'https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap'

// Live React roots per host — a repeat renderDocument on the same host
// must unmount the previous root (not just rip its DOM out) so any
// future node effects (listeners, timers) can't leak.
const liveRoots = new WeakMap()

// renderDocument — standalone, network-free renderer for a V5
// engine.Document. Powers window.KeepstarV5Widget.renderDocument
// (admin canvas preview modal embeds dist/widget.js cross-origin and
// calls this instead of the auto-mounted chat widget).
//
// Contract (Wave C):
//   - attaches (or REUSES — attachShadow twice throws) a shadow root on
//     hostElement, clears prior content
//   - injects the font link + the same ALL_CSS stylesheet as the main
//     widget
//   - renders <SceneGraphRenderer> with the safe no-op RenderContext
//     defaults (the renderer brings its own error boundary) — ZERO
//     network calls, no session init, actions no-op
//   - returns { unmount() } which unmounts the React root and clears
//     the shadow root; stale handles (superseded by a later
//     renderDocument on the same host) no-op
//   - injects the doc's design-system theme (doc.theme.tokens) as a
//     :host{ --kw-… } override after widget.css; falls back to
//     opts.theme.tokens when the standalone doc carries none
export function renderDocument(hostElement, doc, opts = {}) {
  const shadow =
    hostElement.shadowRoot ?? hostElement.attachShadow({ mode: 'open' })

  // Tear down the previous render properly before replacing it.
  const prev = liveRoots.get(hostElement)
  if (prev) {
    liveRoots.delete(hostElement)
    prev.unmount()
  }
  while (shadow.firstChild) shadow.removeChild(shadow.firstChild)

  const fontLink = document.createElement('link')
  fontLink.rel = 'stylesheet'
  fontLink.href = FONT_HREF
  shadow.appendChild(fontLink)

  const style = document.createElement('style')
  style.textContent = ALL_CSS
  shadow.appendChild(style)

  // Design-system theme: inject AFTER the static widget.css so the
  // tenant's --kw-* tokens override the hardcoded defaults. Precedence
  // doc.theme || opts.theme || none (admin preview passes opts.theme
  // when the standalone doc carries no theme). No tokens => nothing is
  // injected and widget.css defaults stand (byte-identical).
  applyTheme(shadow, tokensFromDoc(doc, opts))

  const mountPoint = document.createElement('div')
  mountPoint.id = 'keepstar-v5-preview-root'
  // The widget is designed for light merchant pages; previews often sit
  // inside dark admin chrome, and color/background inherit through the
  // shadow boundary — pin a light storefront-like canvas so cards look
  // the way they do on a real page.
  mountPoint.style.backgroundColor = '#ffffff'
  mountPoint.style.color = '#111827'
  mountPoint.style.colorScheme = 'light'
  mountPoint.style.padding = '16px'
  mountPoint.style.borderRadius = '8px'
  shadow.appendChild(mountPoint)

  const root = createRoot(mountPoint)
  liveRoots.set(hostElement, root)
  root.render(
    <RenderContext.Provider value={defaultRenderContext}>
      <SceneGraphRenderer document={doc} />
    </RenderContext.Provider>,
  )

  return {
    unmount() {
      // A later renderDocument on this host already tore this root down.
      if (liveRoots.get(hostElement) !== root) return
      liveRoots.delete(hostElement)
      root.unmount()
      while (shadow.firstChild) shadow.removeChild(shadow.firstChild)
    },
  }
}
