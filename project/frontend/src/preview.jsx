import { createRoot } from 'react-dom/client'
import { FormationRenderer } from './entities/formation/FormationRenderer'

// Same CSS imports as widget.jsx — exact same visual output
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
import genericCardCss from './entities/widget/templates/GenericCardTemplate.css?inline'
import comparisonCss from './entities/widget/templates/ComparisonTemplate.css?inline'
import productGridCss from './features/catalog/ProductGrid.css?inline'
import backButtonCss from './features/navigation/BackButton.css?inline'
import stepperCss from './features/navigation/Stepper.css?inline'

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
  genericCardCss,
  comparisonCss,
  productGridCss,
  backButtonCss,
  stepperCss,
].join('\n')

const FONT_URL = 'https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=Plus+Jakarta+Sans:wght@600;700;800&display=swap'

/**
 * PreviewLayout — mimics the real widget overlay:
 * backdrop + widget-display-area (formation) + chat-area (placeholder)
 */
function PreviewLayout({ formation, onClose }) {
  return (
    <>
      <div className="chat-backdrop" onClick={onClose} />
      <div className="chat-overlay-layout">
        <div className="widget-display-area">
          <FormationRenderer formation={formation} />
        </div>
        <div className="chat-area">
          <div className="preview-chat-placeholder">
            <div className="preview-chat-placeholder-text">Chat panel</div>
          </div>
        </div>
      </div>
    </>
  )
}

const PREVIEW_CSS = `
.preview-chat-placeholder {
  height: 100%;
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(12px);
  display: flex;
  align-items: center;
  justify-content: center;
  border-left: 1px solid #e2e8f0;
}
.preview-chat-placeholder-text {
  font-size: 14px;
  color: #94a3b8;
  font-weight: 500;
}
`

window.__keepstar_preview = {
  /**
   * Render a formation with the full widget overlay layout.
   * Safe to call multiple times — re-renders in place.
   */
  render(container, formation, options = {}) {
    let shadow = container.shadowRoot
    if (!shadow) {
      shadow = container.attachShadow({ mode: 'open' })

      // Inject CSS (once)
      const style = document.createElement('style')
      style.textContent = ALL_CSS + PREVIEW_CSS
      shadow.appendChild(style)

      // Google Fonts (once)
      const fontLink = document.createElement('link')
      fontLink.rel = 'stylesheet'
      fontLink.href = FONT_URL
      shadow.appendChild(fontLink)

      // Mount point
      const mount = document.createElement('div')
      mount.className = 'keepstar-preview-mount'
      shadow.appendChild(mount)

      container._keepstarRoot = createRoot(mount)
    }

    const onClose = options.onClose || (() => {})
    container._keepstarRoot.render(
      <PreviewLayout formation={formation} onClose={onClose} />
    )
  },

  /**
   * Unmount and clean up.
   */
  destroy(container) {
    if (container._keepstarRoot) {
      container._keepstarRoot.unmount()
      container._keepstarRoot = null
    }
  },
}
