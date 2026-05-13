// Vitest setup — gives every test access to the @testing-library
// matchers (toBeInTheDocument, toHaveTextContent, etc.) and silences
// the ResizeObserver / matchMedia jsdom gaps that React 19 warns about.
import '@testing-library/jest-dom/vitest'

// jsdom doesn't ship matchMedia. WidgetApp doesn't use it but the
// React DOM client still calls it during hydration on some paths —
// stub it to a no-op for safety.
if (typeof window !== 'undefined' && !window.matchMedia) {
  window.matchMedia = () => ({
    matches: false,
    media: '',
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })
}
