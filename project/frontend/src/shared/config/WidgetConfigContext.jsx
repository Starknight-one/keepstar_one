import { createContext, useContext } from 'react'

const WidgetConfigContext = createContext({
  tenantSlug: null,
  apiBaseUrl: null,
})

export function WidgetConfigProvider({ tenantSlug, apiBaseUrl, children }) {
  return (
    <WidgetConfigContext.Provider value={{ tenantSlug, apiBaseUrl }}>
      {children}
    </WidgetConfigContext.Provider>
  )
}

export function useWidgetConfig() {
  return useContext(WidgetConfigContext)
}
