import { useState, useCallback, useEffect, useRef } from 'react'
import { ChatPanel } from './features/chat/ChatPanel'
import { FormationRenderer } from './entities/formation/FormationRenderer'
import { BackButton } from './features/navigation/BackButton'
import { ThemeProvider } from './shared/theme'
import { WidgetConfigProvider } from './shared/config/WidgetConfigContext'
import { ActionProvider } from './features/actions/ActionContext'
import { ActionToolbar } from './features/actions/ActionToolbar'
import { LikedView } from './features/actions/LikedView'
import { CartView } from './features/actions/CartView'
import { setTenantSlug, setApiBaseUrl } from './shared/api/apiClient'

export default function WidgetApp({ tenantSlug, apiBaseUrl }) {
  const [isChatOpen, setIsChatOpen] = useState(false)
  const [activeFormation, setActiveFormation] = useState(null)
  const [navState, setNavState] = useState({ canGoBack: false, onExpand: null, onBack: null })
  const [viewMode, setViewMode] = useState('normal') // 'normal' | 'liked' | 'cart'

  // Lifted entity refs for LikedView / CartView
  const adjacentTemplatesRef = useRef(null)
  const entitiesRef = useRef(null)
  const [entitiesVersion, setEntitiesVersion] = useState(0)

  useEffect(() => {
    if (tenantSlug) setTenantSlug(tenantSlug)
    if (apiBaseUrl) setApiBaseUrl(apiBaseUrl)
  }, [tenantSlug, apiBaseUrl])

  const handleNavigationStateChange = useCallback((state) => {
    setNavState(prev => ({ ...prev, ...state }))
  }, [])

  const handleEntitiesReceived = useCallback((adjacentTemplates, entities) => {
    adjacentTemplatesRef.current = adjacentTemplates
    entitiesRef.current = entities
    setEntitiesVersion(v => v + 1)
  }, [])

  const handleChatClose = () => {
    setIsChatOpen(false)
    setActiveFormation(null)
    setViewMode('normal')
  }

  const handleViewChange = useCallback((mode) => {
    setViewMode(mode)
  }, [])

  const handleBackToNormal = useCallback(() => {
    setViewMode('normal')
  }, [])

  return (
    <WidgetConfigProvider tenantSlug={tenantSlug} apiBaseUrl={apiBaseUrl}>
      <ThemeProvider defaultTheme="marketplace">
        <ActionProvider>
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
                  {viewMode === 'normal' && (
                    <>
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
                    </>
                  )}
                  {viewMode === 'liked' && (
                    <LikedView
                      adjacentTemplates={adjacentTemplatesRef.current}
                      entities={entitiesRef.current}
                      onWidgetClick={navState.onExpand}
                      onBack={handleBackToNormal}
                      key={entitiesVersion}
                    />
                  )}
                  {viewMode === 'cart' && (
                    <CartView
                      entities={entitiesRef.current}
                      onBack={handleBackToNormal}
                      key={entitiesVersion}
                    />
                  )}
                </div>
                <ActionToolbar viewMode={viewMode} onViewChange={handleViewChange} />
                <div className="chat-area">
                  <ChatPanel
                    onClose={handleChatClose}
                    onFormationReceived={setActiveFormation}
                    onNavigationStateChange={handleNavigationStateChange}
                    onEntitiesReceived={handleEntitiesReceived}
                    hideFormation={true}
                  />
                </div>
              </div>
            </>
          )}
        </ActionProvider>
      </ThemeProvider>
    </WidgetConfigProvider>
  )
}
