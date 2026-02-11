import { useState, useCallback, useEffect } from 'react'
import { ChatPanel } from './features/chat/ChatPanel'
import { FormationRenderer } from './entities/formation/FormationRenderer'
import { BackButton } from './features/navigation/BackButton'
import { ThemeProvider } from './shared/theme'
import { WidgetConfigProvider } from './shared/config/WidgetConfigContext'
import { setTenantSlug, setApiBaseUrl } from './shared/api/apiClient'

export default function WidgetApp({ tenantSlug, apiBaseUrl }) {
  const [isChatOpen, setIsChatOpen] = useState(false)
  const [activeFormation, setActiveFormation] = useState(null)
  const [navState, setNavState] = useState({ canGoBack: false, onExpand: null, onBack: null })

  useEffect(() => {
    if (tenantSlug) setTenantSlug(tenantSlug)
    if (apiBaseUrl) setApiBaseUrl(apiBaseUrl)
  }, [tenantSlug, apiBaseUrl])

  const handleNavigationStateChange = useCallback((state) => {
    setNavState(prev => ({ ...prev, ...state }))
  }, [])

  const handleChatClose = () => {
    setIsChatOpen(false)
    setActiveFormation(null)
  }

  return (
    <WidgetConfigProvider tenantSlug={tenantSlug} apiBaseUrl={apiBaseUrl}>
      <ThemeProvider defaultTheme="marketplace">
        <button
          className="chat-toggle-btn"
          onClick={() => setIsChatOpen(!isChatOpen)}
        >
          {isChatOpen ? '\u2715' : '\uD83D\uDCAC'}
        </button>

        {isChatOpen && (
          <>
            <div className="chat-backdrop" onClick={handleChatClose} />
            <div className="chat-overlay-layout">
              <div className="widget-display-area">
                <BackButton
                  visible={navState.canGoBack}
                  onClick={navState.onBack}
                />
                {activeFormation && (
                  <FormationRenderer
                    formation={activeFormation}
                    onWidgetClick={navState.onExpand}
                  />
                )}
              </div>
              <div className="chat-area">
                <ChatPanel
                  onClose={handleChatClose}
                  onFormationReceived={setActiveFormation}
                  onNavigationStateChange={handleNavigationStateChange}
                  hideFormation={true}
                />
              </div>
            </div>
          </>
        )}
      </ThemeProvider>
    </WidgetConfigProvider>
  )
}
