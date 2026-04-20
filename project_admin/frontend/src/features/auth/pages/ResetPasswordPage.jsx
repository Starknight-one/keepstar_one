import React, { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { api } from '../../../shared/api/apiClient.js'

export default function ResetPasswordPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const token = params.get('token') || ''
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
      await api.post('/auth/password/reset', { token, new_password: password })
      navigate('/auth/password-changed')
    } catch (err) {
      setError(err.message || 'Reset failed')
    } finally {
      setLoading(false)
    }
  }

  if (!token) {
    return (
      <AuthShell>
        <h1>Invalid reset link</h1>
        <p>This link is missing its token. Request a new one from sign-in.</p>
      </AuthShell>
    )
  }

  return (
    <AuthShell>
      <h1>Set a new password</h1>
      <p>Pick something you haven't used before — at least 10 characters.</p>

      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
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

        <PillButton variant="primary" block type="submit" disabled={loading}>
          {loading ? 'Updating…' : 'Update password'}
        </PillButton>
      </form>
    </AuthShell>
  )
}
