import React, { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { api } from '../../../shared/api/apiClient.js'

export default function ForgotPasswordPage() {
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await api.post('/auth/password/forgot', { email })
      navigate('/auth/check-email', { state: { email } })
    } catch (err) {
      setError(err.message || 'Something went wrong')
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthShell>
      <Link to="/auth/sign-in" className="auth-back">← Back to sign in</Link>
      <h1>Forgot password?</h1>
      <p>Enter the email on your account and we'll send you a reset link.</p>

      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
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

        <PillButton variant="primary" block type="submit" disabled={loading}>
          {loading ? 'Sending…' : 'Send reset link'}
        </PillButton>
      </form>
    </AuthShell>
  )
}
