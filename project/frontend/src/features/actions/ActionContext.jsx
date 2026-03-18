import { createContext, useContext } from 'react'
import { useActionState } from './useActionState'

const ActionContext = createContext(null)

export function ActionProvider({ children }) {
  const actions = useActionState()
  return (
    <ActionContext.Provider value={actions}>
      {children}
    </ActionContext.Provider>
  )
}

export function useActions() {
  const ctx = useContext(ActionContext)
  if (!ctx) {
    throw new Error('useActions must be used within ActionProvider')
  }
  return ctx
}
