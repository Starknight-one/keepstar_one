import React from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'

// Friendly mapping of backend / callback error reasons to UX copy.
// Backends emit free-form messages; we substring-match against the known
// fragments. Anything unmatched falls back to a generic "Something went wrong".
//
// Each entry carries: title, body, and two action buttons. `provider` is
// supplied by OAuthLoadingPage / TelegramCallbackPage so "Try Google again"
// vs "Try Telegram again" stays accurate without extra UI plumbing.
const REASONS = [
  // Scenario 27 — code/state replay (user hit back-button after consent).
  // Backend deletes state on first Consume, so the second callback gets
  // "invalid or expired state" too — same screen, single-use copy is the
  // friendlier framing for the common back-button case.
  {
    match: ['invalid or expired state', 'state_mismatch'],
    title: 'This link is single-use',
    body: 'Sign-in links can only be used once. Please start again from the sign-in page.',
    primary: { label: 'Back to sign in', href: '/auth/sign-in' },
  },
  // Scenarios 26, 35 — same family, fronted as "expired" when we know.
  {
    match: ['expired'],
    title: 'Time to sign in has expired',
    body: 'For your security, sign-in links expire after a short time. Please try again.',
    primary: { label: 'Back to sign in', href: '/auth/sign-in' },
  },
  // Scenario 29 — user declined Google's consent screen.
  {
    match: ['access_denied', 'google rejected', 'google_rejected'],
    title: "You didn't allow Google access",
    body: 'No problem — you can try Google again, or sign in another way.',
    primary: { label: 'Try Google again', href: '/auth/sign-in', action: 'google' },
    secondary: { label: 'Other methods', href: '/auth/sign-in' },
  },
  // Same shape for Telegram rejected.
  {
    match: ['telegram rejected', 'telegram_rejected'],
    title: "You didn't allow Telegram access",
    body: 'No problem — you can try Telegram again, or sign in another way.',
    primary: { label: 'Try Telegram again', href: '/auth/sign-in', action: 'telegram' },
    secondary: { label: 'Other methods', href: '/auth/sign-in' },
  },
  // State kind mismatch (cross-kind replay — scenario 28). Same UX as 27.
  {
    match: ['state kind mismatch', 'kind mismatch'],
    title: 'Sign-in mismatch',
    body: 'Something looked off with that sign-in attempt. Please start over.',
    primary: { label: 'Back to sign in', href: '/auth/sign-in' },
  },
  // Generic Google failure (network, malformed response).
  {
    match: ['google_failed', 'google could not', 'google token error'],
    title: 'Google sign-in failed',
    body: 'We couldn’t complete Google sign-in. Please try again.',
    primary: { label: 'Try again', href: '/auth/sign-in' },
  },
  {
    match: ['missing_params', 'missing code', 'missing or empty'],
    title: 'Sign-in link was incomplete',
    body: 'Some required pieces of the sign-in link are missing. Please start again.',
    primary: { label: 'Back to sign in', href: '/auth/sign-in' },
  },
]

function pick(reasonRaw) {
  if (!reasonRaw) return null
  const reason = String(reasonRaw).toLowerCase()
  for (const r of REASONS) {
    for (const m of r.match) {
      if (reason.includes(m.toLowerCase())) return r
    }
  }
  return null
}

export default function AuthErrorPage() {
  const [params] = useSearchParams()
  const reasonRaw = params.get('reason') || ''
  const matched = pick(reasonRaw)

  // Fallback for truly unknown errors.
  const view = matched || {
    title: 'Sign-in failed',
    body: 'Something went wrong while signing you in. Please try again.',
    primary: { label: 'Back to sign in', href: '/auth/sign-in' },
  }

  return (
    <AuthShell>
      <h1>{view.title}</h1>
      <p>{view.body}</p>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginTop: 16 }}>
        <Link to={view.primary.href} style={{ textDecoration: 'none' }}>
          <PillButton variant="primary" block>{view.primary.label}</PillButton>
        </Link>
        {view.secondary && (
          <Link to={view.secondary.href} style={{ textDecoration: 'none' }}>
            <PillButton variant="secondary" block>{view.secondary.label}</PillButton>
          </Link>
        )}
      </div>
    </AuthShell>
  )
}
