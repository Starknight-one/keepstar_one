import { createRoot } from 'react-dom/client'
import WidgetApp from './WidgetApp'
// ALL_CSS / FONT_HREF live in widget-preview.jsx as the single source —
// both entries inject the identical stylesheet into their shadow roots.
import { renderDocument, ALL_CSS, FONT_HREF } from './widget-preview'

;(function () {
  const script =
    document.currentScript || document.querySelector('script[src*="widget.js"]')

  const devConfig = window.__KEEPSTAR_V5_WIDGET__ || {}

  // Public API — always assigned, even when auto-mount is skipped.
  // Admin canvas preview loads this same bundle and calls renderDocument
  // to render an engine document without any session / network activity.
  window.KeepstarV5Widget = { renderDocument }

  const tenantSlug = script?.getAttribute('data-tenant') || devConfig.tenant || null

  // Base URL priority:
  //   1. data-api on the embed <script>
  //   2. derive from script src origin (production deploy serves api on
  //      same host)
  //   3. dev config (window.__KEEPSTAR_V5_WIDGET__.api)
  //   4. localhost fallback
  let apiBaseUrl = script?.getAttribute('data-api') || null
  if (!apiBaseUrl && script?.src) {
    try {
      const scriptUrl = new URL(script.src)
      if (scriptUrl.port !== '5173' && !script.src.includes('/src/')) {
        apiBaseUrl = scriptUrl.origin + '/api/v1'
      }
    } catch (_) {
      /* invalid URL; fall through */
    }
  }
  // 8084 is the dev fallback to avoid clashing with V4 (which holds 8082
  // in this monorepo). Production deploys override via data-api or
  // script.src origin.
  if (!apiBaseUrl) apiBaseUrl = devConfig.api || 'http://localhost:8084/api/v1'

  function mount() {
    const host = document.createElement('div')
    host.id = 'keepstar-v5-widget'
    document.body.appendChild(host)

    const shadow = host.attachShadow({ mode: 'open' })

    const fontLink = document.createElement('link')
    fontLink.rel = 'stylesheet'
    fontLink.href = FONT_HREF
    shadow.appendChild(fontLink)

    const style = document.createElement('style')
    style.textContent = ALL_CSS
    shadow.appendChild(style)

    const mountPoint = document.createElement('div')
    mountPoint.id = 'keepstar-v5-root'
    shadow.appendChild(mountPoint)

    const root = createRoot(mountPoint)
    root.render(<WidgetApp tenantSlug={tenantSlug} apiBaseUrl={apiBaseUrl} />)
  }

  // Auto-mount guard: embedders that only want renderDocument (admin
  // canvas preview) set window.__KEEPSTAR_V5_WIDGET__ = { noAutoMount:
  // true } BEFORE the script loads. Flag absent/false → mount as today.
  if (devConfig.noAutoMount !== true) {
    mount()
  }
})()
