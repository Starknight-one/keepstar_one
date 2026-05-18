import React, { useState } from 'react'
import { Link } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { api } from '../../../shared/api/apiClient.js'

// Scenarios 40, 41: friendly destination when a magic-link returns 401
// because the code was already used or expired. We don't distinguish
// "used" vs "expired" on the backend today — both surface as
// ErrInvalidCredentials → "link expired or already used". The UX leads
// with the more common case (already used) and offers a one-click way
// to request a fresh link.
//
// "Request a new one" hits the password-reset / forgot endpoint rather
// than introducing a separate magic-link request endpoint — for a user
// who can't open the existing link, the reset flow is the simplest path
// back into the account. (Anti-enumeration handled server-side: always
// returns 200 regardless of whether the email exists.)
export default function MagicLinkExpiredPage() {
  const [email, setEmail] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      await api.post('/auth/forgot', { email })
      setSent(true)
    } catch (err) {
      setError(err.message || 'Could not send a new link. Please try again.')
    } finally {
      setSubmitting(false)
    }
  }

  if (sent) {
    return (
      <AuthShell>
        <h1>Check your inbox</h1>
        <p>If an account exists for <strong>{email}</strong>, we've sent a fresh link there.</p>
        <Link to="/auth/sign-in" style={{ textDecoration: 'none' }}>
          <PillButton variant="primary" block>Back to sign in</PillButton>
        </Link>
      </AuthShell>
    )
  }

  return (
    <AuthShell>
      <h1>This link no longer works</h1>
      <p>Sign-in links are single-use and expire after 24 hours. Enter your email and we'll send you a fresh one.</p>

      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 16, marginTop: 16 }}>
        {error && <div className="auth-alert auth-alert--error">{error}</div>}

        <div className="auth-field">
          <label className="auth-field__label" htmlFor="email">Email</label>
          <input
            id="email"
            type="email"
            className="auth-field__input"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@company.com"
            autoComplete="email"
            required
          />
        </div>

        <PillButton variant="primary" block type="submit" disabled={submitting || !email}>
          {submitting ? 'Sending…' : 'Send me a new link'}
        </PillButton>
      </form>

      <p style={{ fontSize: 14, marginTop: 16 }}>
        <Link to="/auth/sign-in" className="auth-link">Back to sign in</Link>
      </p>
    </AuthShell>
  )
}
