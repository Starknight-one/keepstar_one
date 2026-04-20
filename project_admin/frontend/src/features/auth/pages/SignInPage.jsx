import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'
import { useTelegramWidget } from '../hooks/useTelegramWidget.js'

export default function SignInPage() {
  const { login, adoptSession } = useAuth()
  const navigate = useNavigate()
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
      setError(err.message || 'Failed to start Google sign-in')
      setGoogleLoading(false)
    }
  }

  const handleTelegramAuth = useCallback(async (tgUser) => {
    setError('')
    try {
      const data = await authApi.telegramCallback(tgUser)
      adoptSession(data)
      navigate('/auth/pick-workspace')
    } catch (err) {
      setError(err.message || 'Telegram sign-in failed')
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
      const data = await login(email, password)
      if (data?.requires_2fa) {
        navigate('/auth/2fa', { state: { pre2faToken: data.pre_2fa_token, email } })
        return
      }
      navigate('/auth/pick-workspace')
    } catch (err) {
      setError(err.message || 'Invalid email or password')
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthShell>
      <h1>Welcome back</h1>
      <p>Sign in to your Keepstar One workspace.</p>

      {flags.google || flags.telegram.enabled ? (
        <>
          <div className="auth-providers">
            {flags.google && (
              <PillButton variant="secondary" block onClick={handleGoogle} disabled={googleLoading}>
                {googleLoading ? 'Redirecting\u2026' : 'Continue with Google'}
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

        <div className="auth-field">
          <label className="auth-field__label" htmlFor="password">Password</label>
          <input
            id="password"
            type="password"
            className="auth-field__input"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="••••••••"
            autoComplete="current-password"
            required
          />
          {flags.email && (
            <div style={{ alignSelf: 'flex-end', marginTop: 4 }}>
              <Link to="/auth/forgot-password" className="auth-link" style={{ fontSize: 13, borderBottom: 'none' }}>
                Forgot password?
              </Link>
            </div>
          )}
        </div>

        <PillButton variant="primary" block type="submit" disabled={loading}>
          {loading ? 'Signing in…' : 'Sign in'}
        </PillButton>
      </form>

      <p style={{ fontSize: 14 }}>
        Don't have an account?{' '}
        <Link to="/auth/sign-up" className="auth-link">Sign up</Link>
      </p>
    </AuthShell>
  )
}
