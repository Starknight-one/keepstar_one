import { createRoot } from 'react-dom/client'
import WidgetApp from './WidgetApp'

// All CSS imported as strings via ?inline — injected into Shadow DOM.
// Regular CSS imports in components are suppressed by the shadowDomCss plugin.
import widgetCss from './widget.css?inline'
import overlayCss from './features/overlay/Overlay.css?inline'
import chatPanelCss from './features/chat/ChatPanel.css?inline'
import widgetCompCss from './entities/widget/Widget.css?inline'
import formationCss from './entities/formation/Formation.css?inline'
import atomCss from './entities/atom/Atom.css?inline'
import marketplaceCss from './shared/theme/themes/marketplace.css?inline'
import productCardCss from './entities/widget/templates/ProductCardTemplate.css?inline'
import productDetailCss from './entities/widget/templates/ProductDetailTemplate.css?inline'
import serviceCardCss from './entities/widget/templates/ServiceCardTemplate.css?inline'
import serviceDetailCss from './entities/widget/templates/ServiceDetailTemplate.css?inline'
import productGridCss from './features/catalog/ProductGrid.css?inline'
import backButtonCss from './features/navigation/BackButton.css?inline'

const ALL_CSS = [
  widgetCss,
  overlayCss,
  chatPanelCss,
  widgetCompCss,
  formationCss,
  atomCss,
  marketplaceCss,
  productCardCss,
  productDetailCss,
  serviceCardCss,
  serviceDetailCss,
  productGridCss,
  backButtonCss,
].join('\n')

;(function () {
  // Production: read from data attributes (IIFE, document.currentScript works)
  // Dev: module scripts don't set currentScript, fall back to window.__KEEPSTAR_WIDGET__
  const script = document.currentScript
  const devConfig = window.__KEEPSTAR_WIDGET__
  const tenantSlug = script?.getAttribute('data-tenant') || devConfig?.tenant || null
  const apiBaseUrl = script?.getAttribute('data-api') || devConfig?.api || null

  // Create host element
  const host = document.createElement('div')
  host.id = 'keepstar-widget'
  document.body.appendChild(host)

  // Attach Shadow DOM
  const shadow = host.attachShadow({ mode: 'open' })

  // Load Google Fonts inside shadow root
  const fontLink = document.createElement('link')
  fontLink.rel = 'stylesheet'
  fontLink.href = 'https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=Plus+Jakarta+Sans:wght@600;700;800&display=swap'
  shadow.appendChild(fontLink)

  // Inject all CSS into shadow root
  const style = document.createElement('style')
  style.textContent = ALL_CSS
  shadow.appendChild(style)

  // Mount point for React
  const mountPoint = document.createElement('div')
  mountPoint.id = 'keepstar-root'
  shadow.appendChild(mountPoint)

  // Render React app
  const root = createRoot(mountPoint)
  root.render(
    <WidgetApp tenantSlug={tenantSlug} apiBaseUrl={apiBaseUrl} />
  )
})()
