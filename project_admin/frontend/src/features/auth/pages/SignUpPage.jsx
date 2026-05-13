import React, { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'

export default function SignUpPage() {
  const { signup } = useAuth()
  const navigate = useNavigate()
  const [companyName, setCompanyName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [flags, setFlags] = useState({ google: false, email: false, telegram: { enabled: false } })
  const [googleLoading, setGoogleLoading] = useState(false)
  const [telegramLoading, setTelegramLoading] = useState(false)

  useEffect(() => {
    authApi.config().then(setFlags).catch(() => {})
  }, [])

  async function handleGoogle() {
    setError('')
    setGoogleLoading(true)
    try {
      const { url } = await authApi.googleStart()
      window.location.href = url
    } catch (err) {
      setError(err.message || 'Failed to start Google sign-up')
      setGoogleLoading(false)
    }
  }

  async function handleTelegram() {
    setError('')
    setTelegramLoading(true)
    try {
      const { url } = await authApi.telegramStart()
      window.location.href = url
    } catch (err) {
      setError(err.message || 'Failed to start Telegram sign-up')
      setTelegramLoading(false)
    }
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await signup(email, password, companyName)
      navigate('/catalog')
    } catch (err) {
      setError(err.message || 'Signup failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthShell>
      <h1>Create your workspace</h1>
      <p>Start onboarding your catalog in minutes.</p>

      {flags.google || flags.telegram.enabled ? (
        <>
          <div className="auth-providers">
            {flags.google && (
              <PillButton variant="secondary" block onClick={handleGoogle} disabled={googleLoading}>
                {googleLoading ? 'Redirecting…' : 'Sign up with Google'}
              </PillButton>
            )}
            {flags.telegram.enabled && (
              <PillButton variant="telegram" block onClick={handleTelegram} disabled={telegramLoading}>
                {telegramLoading ? 'Redirecting…' : 'Sign up with Telegram'}
              </PillButton>
            )}
          </div>
          <div className="auth-divider">or</div>
        </>
      ) : null}

      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        {error && <div className="auth-alert auth-alert--error">{error}</div>}

        <div className="auth-field">
          <label className="auth-field__label" htmlFor="company">Company name</label>
          <input
            id="company"
            type="text"
            className="auth-field__input"
            value={companyName}
            onChange={(e) => setCompanyName(e.target.value)}
            placeholder="Acme Cosmetics"
            required
          />
        </div>

        <div className="auth-field">
          <label className="auth-field__label" htmlFor="email">Work email</label>
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

        <div className="auth-field">
          <label className="auth-field__label" htmlFor="password">Password</label>
          <input
            id="password"
            type="password"
            className="auth-field__input"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="At least 10 characters"
            autoComplete="new-password"
            minLength={10}
            required
          />
        </div>

        <PillButton variant="primary" block type="submit" disabled={loading}>
          {loading ? 'Creating workspace…' : 'Create workspace'}
        </PillButton>
      </form>

      <p style={{ fontSize: 14 }}>
        Already have an account?{' '}
        <Link to="/auth/sign-in" className="auth-link">Sign in</Link>
      </p>
    </AuthShell>
  )
}
