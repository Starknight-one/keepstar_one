import { useEffect, useState } from 'react'
import ChatShell from './ChatShell'
import { initSession } from '../api/client'

// CRMShell — the /crm/{slug}?k=<surface_token> page (RUNTIME_SPEC §5.1 +
// R13): CRM v1 is chat-first — one wide chat column, no view engine, no
// login. The surface token k is checked ONCE at session init; a valid,
// unrevoked token stamps role=staff into the session row and every
// pipeline turn inherits it. Invalid or absent token → access-denied
// page (403), no session created.
export default function CRMShell({ apiBaseUrl, tenantSlug, surfaceToken, shadowRoot }) {
  // 'connecting' → 'ready' | 'denied' | 'error'
  const [phase, setPhase] = useState(surfaceToken ? 'connecting' : 'denied')
  const [session, setSession] = useState(null) // {sessionId, tenantName}

  useEffect(() => {
    if (!surfaceToken) return undefined
    let cancelled = false
    initSession({ baseUrl: apiBaseUrl, tenantSlug, mode: 'crm', k: surfaceToken })
      .then((r) => {
        if (cancelled) return
        setSession({ sessionId: r.sessionId, tenantName: r.tenant?.name || tenantSlug })
        setPhase('ready')
      })
      .catch((err) => {
        if (cancelled) return
        // eslint-disable-next-line no-console
        console.error('[v5-crm] session init failed', err)
        setPhase(err.status === 403 ? 'denied' : 'error')
      })
    return () => {
      cancelled = true
    }
  }, [apiBaseUrl, tenantSlug, surfaceToken])

  if (phase === 'ready' && session) {
    return (
      <ChatShell
        apiBaseUrl={apiBaseUrl}
        tenantSlug={tenantSlug}
        sessionId={session.sessionId}
        shadowRoot={shadowRoot}
        variant="crm"
        headerTitle={session.tenantName}
        headerBadge="CRM"
        initialMessages={[{ role: 'bot', text: 'Ask about your leads — e.g. "any new leads?"' }]}
        placeholder="Any new leads?"
      />
    )
  }

  if (phase === 'denied') {
    return (
      <div className="kw-gate">
        <div className="kw-gate-card">
          <div className="kw-gate-logo">Keepstar</div>
          <p className="kw-gate-note" role="alert">
            This CRM link is invalid or has been revoked. Ask the workspace owner for a fresh link.
          </p>
        </div>
      </div>
    )
  }

  if (phase === 'error') {
    return (
      <div className="kw-gate">
        <div className="kw-gate-card">
          <div className="kw-gate-logo">Keepstar</div>
          <p className="kw-gate-note">Could not reach the CRM service. Refresh to try again.</p>
        </div>
      </div>
    )
  }

  return (
    <div className="kw-gate">
      <div className="kw-gate-card">
        <div className="kw-gate-logo">Keepstar</div>
        <p className="kw-gate-note">Connecting…</p>
      </div>
    </div>
  )
}
