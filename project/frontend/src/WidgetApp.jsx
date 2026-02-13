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
        {!isChatOpen && (
          <div className="chat-toggle-btn" onClick={() => setIsChatOpen(true)}>
            <span className="chat-toggle-bubble">Спроси меня!</span>
            <button className="chat-toggle-circle" aria-label="Open chat">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
              </svg>
            </button>
          </div>
        )}

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
