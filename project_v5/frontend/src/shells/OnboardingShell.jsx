import { useCallback, useEffect, useState } from 'react'
import ChatShell from './ChatShell'
import { submitGate, createOnboardSession, resumeOnboardSession } from '../api/onboard'
import { transcriptToMessages } from '../chat/turnBlocks'

// OnboardingShell — the /onboard page (RUNTIME_SPEC §5.1 + R5): password
// form → gate endpoint → chat-only ~720px column. Refresh-safe: the
// session id persists in localStorage; mount resumes it via
// GET /onboard/session?sessionId= (all truth in DB — manifest,
// transcript). A 403 anywhere lands on the password gate; the gate posts
// {password} to POST /api/v1/onboard/gate which sets the HttpOnly
// ks_onboard cookie; with the cookie in place POST /onboard/session
// creates the mode='onboarding' session under the keepstar-onboarding
// system tenant. A stale (404) or killed (409) stored session is
// discarded and a fresh one is created.
//
// Pipeline calls ride credentials:'include' so the cookie reaches the
// pipeline handler's onboarding check (§6.3) in cross-origin dev setups
// too (prod is same-origin, §5.1).

// Canonical system-tenant slug (§3.4, mirrored in handler_onboard.go) —
// the fallback when a resume payload carries no tenant object.
const ONBOARDING_TENANT_SLUG = 'keepstar-onboarding'

// localStorage key for the onboarding session id (refresh-safe resume).
const SESSION_STORE_KEY = 'keepstar-onboard-session'

function readStoredSessionId() {
  try {
    return window.localStorage.getItem(SESSION_STORE_KEY) || null
  } catch (_) {
    return null // storage blocked (private mode) — every visit starts fresh
  }
}

function writeStoredSessionId(id) {
  try {
    if (id) window.localStorage.setItem(SESSION_STORE_KEY, id)
    else window.localStorage.removeItem(SESSION_STORE_KEY)
  } catch (_) {
    /* storage blocked — resume simply won't survive a refresh */
  }
}

export default function OnboardingShell({ apiBaseUrl, shadowRoot }) {
  // 'checking' → 'gate' → 'starting' → 'chat' | 'disabled' | 'error'
  const [phase, setPhase] = useState('checking')
  const [session, setSession] = useState(null) // {sessionId, tenantSlug, messages}
  const [gateError, setGateError] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const startSession = useCallback(async () => {
    setPhase('starting')
    try {
      const r = await createOnboardSession({ baseUrl: apiBaseUrl })
      writeStoredSessionId(r.sessionId)
      setSession({
        sessionId: r.sessionId,
        tenantSlug: r.tenant?.slug || ONBOARDING_TENANT_SLUG,
        messages: [],
      })
      setPhase('chat')
    } catch (err) {
      if (err.status === 403) {
        setPhase('gate')
      } else if (err.status === 503) {
        setPhase('disabled')
      } else {
        // eslint-disable-next-line no-console
        console.error('[v5-onboard] session create failed', err)
        setPhase('error')
      }
    }
  }, [apiBaseUrl])

  // boot — resume the stored session when one exists, else create a
  // fresh one. 403 → gate (cookie missing/expired — the stored id is
  // KEPT so entering the password resumes the same session); 404/409 →
  // stale/killed stored id, discard and create; 503 → disabled.
  const boot = useCallback(async () => {
    const stored = readStoredSessionId()
    if (stored) {
      try {
        const r = await resumeOnboardSession({ baseUrl: apiBaseUrl, sessionId: stored })
        if (r && r.sessionId) {
          setSession({
            sessionId: r.sessionId,
            tenantSlug: r.tenant?.slug || ONBOARDING_TENANT_SLUG,
            messages: transcriptToMessages(r.transcript),
          })
          setPhase('chat')
          return
        }
        writeStoredSessionId(null)
      } catch (err) {
        if (err.status === 403) {
          setPhase('gate')
          return
        }
        if (err.status === 503) {
          setPhase('disabled')
          return
        }
        if (err.status === 404 || err.status === 409 || err.status === 400) {
          writeStoredSessionId(null) // stale or killed — start over
        } else {
          // eslint-disable-next-line no-console
          console.error('[v5-onboard] resume failed', err)
          setPhase('error')
          return
        }
      }
    }
    await startSession()
  }, [apiBaseUrl, startSession])

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      if (!cancelled) await boot()
    })()
    return () => {
      cancelled = true
    }
  }, [boot])

  const onGateSubmit = async (e) => {
    e?.preventDefault()
    const value = password.trim()
    if (!value || submitting) return
    setSubmitting(true)
    setGateError('')
    try {
      await submitGate({ baseUrl: apiBaseUrl, password: value })
      setPassword('')
      await boot() // a stored session id resumes; otherwise a fresh session is created
    } catch (err) {
      if (err.status === 403) setGateError('Wrong password. Try again.')
      else if (err.status === 429) setGateError('Too many attempts — wait a minute and try again.')
      else if (err.status === 503) setPhase('disabled')
      else setGateError('Something went wrong. Please try again.')
    } finally {
      setSubmitting(false)
    }
  }

  if (phase === 'chat' && session) {
    return (
      <ChatShell
        apiBaseUrl={apiBaseUrl}
        tenantSlug={session.tenantSlug}
        sessionId={session.sessionId}
        shadowRoot={shadowRoot}
        variant="onboarding"
        headerTitle="Keepstar"
        headerBadge="Onboarding"
        initialMessages={
          session.messages.length > 0
            ? session.messages
            : [
                {
                  role: 'bot',
                  text: 'Tell me about your business and what you need — a storefront, a CRM, or both.',
                },
              ]
        }
        placeholder="Describe your business…"
        credentials="include"
      />
    )
  }

  if (phase === 'disabled') {
    return (
      <GatePage>
        <p className="kw-gate-note">Onboarding is currently disabled. Please come back later.</p>
      </GatePage>
    )
  }

  if (phase === 'error') {
    return (
      <GatePage>
        <p className="kw-gate-note">Could not reach the onboarding service. Refresh to try again.</p>
      </GatePage>
    )
  }

  if (phase === 'gate') {
    return (
      <GatePage>
        <form className="kw-gate-form" onSubmit={onGateSubmit}>
          <label className="kw-field-label" htmlFor="kw-gate-password">
            Access password
          </label>
          <input
            id="kw-gate-password"
            className="kw-input"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            disabled={submitting}
            data-invalid={gateError ? 'true' : undefined}
          />
          {gateError ? (
            <p className="kw-gate-error" role="alert">
              {gateError}
            </p>
          ) : null}
          <button className="kw-gate-button" type="submit" disabled={submitting || !password.trim()}>
            {submitting ? 'Checking…' : 'Enter'}
          </button>
        </form>
      </GatePage>
    )
  }

  // 'checking' / 'starting'
  return (
    <GatePage>
      <p className="kw-gate-note">Loading…</p>
    </GatePage>
  )
}

// GatePage — brand-styled centered card (light blue #5BA4D9 + orange
// #F0924A gradient wordmark; never purple).
function GatePage({ children }) {
  return (
    <div className="kw-gate">
      <div className="kw-gate-card">
        <div className="kw-gate-logo">Keepstar</div>
        <div className="kw-gate-subtitle">Set up your storefront and CRM in one conversation.</div>
        {children}
      </div>
    </div>
  )
}
