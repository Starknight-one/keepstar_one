import React, { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'

export default function SignInPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
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

  // Browser bfcache restores the page with stale loading=true when user hits Back.
  useEffect(() => {
    const reset = (e) => { if (e.persisted) { setGoogleLoading(false); setTelegramLoading(false) } }
    window.addEventListener('pageshow', reset)
    return () => window.removeEventListener('pageshow', reset)
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

  async function handleTelegram() {
    setError('')
    setTelegramLoading(true)
    try {
      const { url } = await authApi.telegramStart()
      window.location.href = url
    } catch (err) {
      setError(err.message || 'Failed to start Telegram sign-in')
      setTelegramLoading(false)
    }
  }

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
                {!googleLoading && (
                  <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
                    <path fill="#4285F4" d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844c-.209 1.125-.843 2.078-1.796 2.717v2.258h2.908C16.658 14.013 17.64 11.705 17.64 9.2z"/>
                    <path fill="#34A853" d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z"/>
                    <path fill="#FBBC05" d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z"/>
                    <path fill="#EA4335" d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 6.29C4.672 4.163 6.656 3.58 9 3.58z"/>
                  </svg>
                )}
                {googleLoading ? 'Redirecting\u2026' : 'Continue with Google'}
              </PillButton>
            )}
            {flags.telegram.enabled && (
              <PillButton variant="telegram" block onClick={handleTelegram} disabled={telegramLoading}>
                {!telegramLoading && (
                  <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
                    <path fill="#2AABEE" d="M12 0C5.373 0 0 5.373 0 12s5.373 12 12 12 12-5.373 12-12S18.627 0 12 0zm5.562 8.248-2.032 9.573c-.144.65-.529.806-1.072.502l-2.968-2.185-1.432 1.38c-.158.157-.291.291-.6.291l.213-3.02 5.51-4.976c.24-.213-.052-.332-.372-.12L7.033 14.3l-2.923-.913c-.637-.198-.65-.637.133-.944l10.9-4.203c.53-.192.994.13.82.985z"/>
                  </svg>
                )}
                {telegramLoading ? 'Redirecting\u2026' : 'Continue with Telegram'}
              </PillButton>
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
