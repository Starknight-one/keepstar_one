import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'
import { useTelegramWidget } from '../hooks/useTelegramWidget.js'

export default function SignUpPage() {
  const { signup, adoptSession } = useAuth()
  const navigate = useNavigate()
  const [companyName, setCompanyName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [flags, setFlags] = useState({ google: false, email: false, telegram: { enabled: false, bot_username: '' } })
  const [googleLoading, setGoogleLoading] = useState(false)
  const telegramRef = useRef(null)

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

  const handleTelegramAuth = useCallback(async (tgUser) => {
    setError('')
    try {
      const data = await authApi.telegramCallback(tgUser)
      adoptSession(data)
      navigate('/catalog')
    } catch (err) {
      setError(err.message || 'Telegram sign-up failed')
    }
  }, [adoptSession, navigate])

  useTelegramWidget({
    containerRef: telegramRef,
    botUsername: flags.telegram.bot_username,
    onAuth: handleTelegramAuth,
    enabled: !!flags.telegram.enabled,
  })

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
                {googleLoading ? 'Redirecting\u2026' : 'Sign up with Google'}
              </PillButton>
            )}
            {flags.telegram.enabled && (
              <div ref={telegramRef} className="auth-telegram-widget" />
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
