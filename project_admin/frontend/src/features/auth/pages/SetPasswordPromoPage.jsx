import React, { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { authApi } from '../api/authApi.js'

// Scenario 39: shown right after a successful sign-in only when the user
// has no password set (HasPassword=false on AdminUser JSON). Skippable —
// the user lands in the workspace either way.
//
// Trigger heuristic on the caller side: any flow that lands a logged-in
// user (login, signup, OAuth adopt, magic-link adopt) checks
// `data.user.has_password`. False → navigate here; true → straight to the
// workspace picker as before.
export default function SetPasswordPromoPage() {
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    if (password !== confirm) {
      setError('Passwords do not match')
      return
    }
    setLoading(true)
    try {
      await authApi.setPassword(password)
      navigate('/auth/pick-workspace', { replace: true })
    } catch (err) {
      // 409 → already has a password (race: two tabs adopted the same
      // session). Drop them straight into the workspace instead of looping.
      if (err && String(err.message || '').toLowerCase().includes('already set')) {
        navigate('/auth/pick-workspace', { replace: true })
        return
      }
      setError(err.message || 'Failed to set password')
    } finally {
      setLoading(false)
    }
  }

  function handleSkip() {
    navigate('/auth/pick-workspace', { replace: true })
  }

  return (
    <AuthShell>
      <h1>Set a password</h1>
      <p>
        You signed in without a password. Set one now so you can sign in by
        email + password next time too. You can always do this later in
        Settings.
      </p>

      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 16, marginTop: 16 }}>
        {error && <div className="auth-alert auth-alert--error">{error}</div>}

        <div className="auth-field">
          <label className="auth-field__label" htmlFor="password">New password</label>
          <input
            id="password"
            type="password"
            className="auth-field__input"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            minLength={10}
            autoComplete="new-password"
            required
            autoFocus
          />
        </div>

        <div className="auth-field">
          <label className="auth-field__label" htmlFor="confirm">Confirm password</label>
          <input
            id="confirm"
            type="password"
            className="auth-field__input"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            minLength={10}
            autoComplete="new-password"
            required
          />
        </div>

        <PillButton variant="primary" block type="submit" disabled={loading || !password}>
          {loading ? 'Saving…' : 'Set password'}
        </PillButton>

        <button
          type="button"
          onClick={handleSkip}
          disabled={loading}
          className="auth-link-button"
        >
          Skip for now
        </button>
      </form>
    </AuthShell>
  )
}
