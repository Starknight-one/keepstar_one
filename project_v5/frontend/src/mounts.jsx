import { createRoot } from 'react-dom/client'
import WidgetApp from './WidgetApp'
import OnboardingShell from './shells/OnboardingShell'
import CRMShell from './shells/CRMShell'
import { ALL_CSS, FONT_HREF } from './widget-preview'

// Page mounts (RUNTIME_SPEC §5.1). The v5 fileserver hosts three pages
// on the SAME origin as the API:
//   GET /onboard        → mountOnboarding  (password gate → chat)
//   GET /s/{slug}       → mountStorefront  (today's widget as the page)
//   GET /crm/{slug}?k=  → mountCRM         (chat-first, R13)
// Each mount attaches a shadow root on the host (same isolation as the
// embedded widget), injects the shared stylesheet + font, and renders
// the shell. Options are optional — defaults derive from the page URL:
// tenant slug from the path, surface token from ?k=, API base from the
// page origin (§5.1: same origin → zero CORS).
//
// Exposed as window.KeepstarV5Widget.{mountStorefront,mountCRM,
// mountOnboarding} by the entry (widget.jsx) — exports live HERE, never
// in the entry (vite.config.js build guard).

export function parseStorefrontSlug(pathname) {
  const m = /^\/s\/([^/]+)\/?$/.exec(pathname || '')
  return m ? decodeURIComponent(m[1]) : null
}

export function parseCRMSlug(pathname) {
  const m = /^\/crm\/([^/]+)\/?$/.exec(pathname || '')
  return m ? decodeURIComponent(m[1]) : null
}

export function parseSurfaceToken(search) {
  try {
    return new URLSearchParams(search || '').get('k') || null
  } catch (_) {
    return null
  }
}

function defaultApiBase() {
  return window.location.origin + '/api/v1'
}

// mountShell — shared shadow-root plumbing (mirrors widget.jsx mount()
// and widget-preview renderDocument): reuse an existing shadow root
// (attachShadow twice throws), clear it, inject font + CSS, render.
function mountShell(hostElement, render) {
  const host = hostElement || createDefaultHost()
  const shadow = host.shadowRoot ?? host.attachShadow({ mode: 'open' })
  while (shadow.firstChild) shadow.removeChild(shadow.firstChild)

  const fontLink = document.createElement('link')
  fontLink.rel = 'stylesheet'
  fontLink.href = FONT_HREF
  shadow.appendChild(fontLink)

  const style = document.createElement('style')
  style.textContent = ALL_CSS
  shadow.appendChild(style)

  const mountPoint = document.createElement('div')
  mountPoint.id = 'keepstar-v5-root'
  shadow.appendChild(mountPoint)

  const root = createRoot(mountPoint)
  root.render(render(shadow))

  return {
    unmount() {
      root.unmount()
      while (shadow.firstChild) shadow.removeChild(shadow.firstChild)
    },
  }
}

function createDefaultHost() {
  const host = document.createElement('div')
  host.id = 'keepstar-v5-page'
  document.body.appendChild(host)
  return host
}

// mountStorefront — today's widget as the public per-tenant page
// (kw-page variant of the overlay; session/init {mode:"storefront"}).
export function mountStorefront(hostElement, opts = {}) {
  const tenantSlug = opts.tenant || parseStorefrontSlug(window.location.pathname)
  const apiBaseUrl = opts.api || defaultApiBase()
  return mountShell(hostElement, (shadow) => (
    <WidgetApp
      tenantSlug={tenantSlug}
      apiBaseUrl={apiBaseUrl}
      shadowRoot={shadow}
      mode="storefront"
      variant="page"
    />
  ))
}

// mountCRM — chat-first CRM page (R13); ?k= surface token → role staff
// at session init, invalid/absent token → access-denied page.
export function mountCRM(hostElement, opts = {}) {
  const tenantSlug = opts.tenant || parseCRMSlug(window.location.pathname)
  const surfaceToken = opts.k || parseSurfaceToken(window.location.search)
  const apiBaseUrl = opts.api || defaultApiBase()
  return mountShell(hostElement, (shadow) => (
    <CRMShell
      apiBaseUrl={apiBaseUrl}
      tenantSlug={tenantSlug}
      surfaceToken={surfaceToken}
      shadowRoot={shadow}
    />
  ))
}

// mountOnboarding — the /onboard page (R5): password gate → chat.
export function mountOnboarding(hostElement, opts = {}) {
  const apiBaseUrl = opts.api || defaultApiBase()
  return mountShell(hostElement, (shadow) => (
    <OnboardingShell apiBaseUrl={apiBaseUrl} shadowRoot={shadow} />
  ))
}
