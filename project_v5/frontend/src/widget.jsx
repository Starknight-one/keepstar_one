import { createRoot } from 'react-dom/client'
import WidgetApp from './WidgetApp'

// All CSS imported as ?inline strings — concatenated and injected into
// the shadow root. shadowDomCss plugin suppresses regular .css imports
// in components so nothing leaks into document.head.
import widgetCss from './widget.css?inline'

const ALL_CSS = [widgetCss].join('\n')

;(function () {
  const script =
    document.currentScript || document.querySelector('script[src*="widget.js"]')

  const devConfig = window.__KEEPSTAR_V5_WIDGET__ || {}

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
  if (!apiBaseUrl) apiBaseUrl = devConfig.api || 'http://localhost:8082/api/v1'

  function mount() {
    const host = document.createElement('div')
    host.id = 'keepstar-v5-widget'
    document.body.appendChild(host)

    const shadow = host.attachShadow({ mode: 'open' })

    const fontLink = document.createElement('link')
    fontLink.rel = 'stylesheet'
    fontLink.href =
      'https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap'
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

  mount()
})()
