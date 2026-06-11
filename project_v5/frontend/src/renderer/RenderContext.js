import { createContext, useContext } from 'react'

// RenderContext is the per-pipeline-turn context flowing through
// every node in the scene-graph renderer. Built once in WidgetApp on
// mount + on every pipeline response, threaded down via React's
// Context API so leaf nodes (Frame click, Text-button onClick) can
// dispatch actions without prop drilling through 6+ levels of frames.
//
// Shape:
//   apiBaseUrl       — for /actions, /navigation/{expand,back}
//   tenantSlug       — for X-Tenant-Slug header
//   sessionId        — current session
//   prefetch         — { adjacentTemplate: {entityType: doc}, entities: {...} }
//                      Updated on each pipeline response. Empty objects
//                      mean "no prefetch this turn — full round-trip".
//   onUpdateDocument — setDocument from WidgetApp; lets actionDispatch
//                      replace the rendered scene-graph in place
//                      (e.g. drill / back).
//   onSearch         — fill chat input with text (search action).
//
// Default value gives every callsite a safe no-op so missing-provider
// tests don't throw. Exported so the standalone preview renderer
// (widget-preview.jsx) can provide the same safe defaults explicitly.
export const defaultRenderContext = {
  apiBaseUrl: '',
  tenantSlug: null,
  sessionId: null,
  prefetch: { adjacentTemplate: {}, entities: {} },
  onUpdateDocument: () => {},
  onSearch: () => {},
}

const RenderContext = createContext(defaultRenderContext)

export default RenderContext

export function useRenderContext() {
  return useContext(RenderContext)
}
