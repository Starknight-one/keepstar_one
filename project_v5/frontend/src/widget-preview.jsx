import { createRoot } from 'react-dom/client'
import SceneGraphRenderer from './renderer/SceneGraphRenderer'
import RendererErrorBoundary from './renderer/RendererErrorBoundary'
import RenderContext, { defaultRenderContext } from './renderer/RenderContext'

// Same ?inline mechanism as widget.jsx — Vite dedupes the module, so the
// CSS string ships once in the bundle regardless of both entries
// importing it.
import widgetCss from './widget.css?inline'

const ALL_CSS = [widgetCss].join('\n')

const FONT_HREF =
  'https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap'

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
//     defaults inside the existing renderer error boundary — ZERO
//     network calls, no session init, actions no-op
//   - returns { unmount() } which unmounts the React root and clears
//     the shadow root
// eslint-disable-next-line no-unused-vars
export function renderDocument(hostElement, doc, opts = {}) {
  const shadow =
    hostElement.shadowRoot ?? hostElement.attachShadow({ mode: 'open' })

  // Clear prior content — repeat calls on the same host replace it.
  while (shadow.firstChild) shadow.removeChild(shadow.firstChild)

  const fontLink = document.createElement('link')
  fontLink.rel = 'stylesheet'
  fontLink.href = FONT_HREF
  shadow.appendChild(fontLink)

  const style = document.createElement('style')
  style.textContent = ALL_CSS
  shadow.appendChild(style)

  const mountPoint = document.createElement('div')
  mountPoint.id = 'keepstar-v5-preview-root'
  shadow.appendChild(mountPoint)

  const root = createRoot(mountPoint)
  root.render(
    <RenderContext.Provider value={defaultRenderContext}>
      <RendererErrorBoundary resetKey={doc}>
        <SceneGraphRenderer document={doc} />
      </RendererErrorBoundary>
    </RenderContext.Provider>,
  )

  return {
    unmount() {
      root.unmount()
      while (shadow.firstChild) shadow.removeChild(shadow.firstChild)
    },
  }
}
