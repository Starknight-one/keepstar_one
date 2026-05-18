import React, { useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import CodeInput from '../layout/CodeInput.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'
import { postSignInPath } from '../postSignIn.js'

// TwoFactorPage finishes a login flow that returned requires_2fa:true. It
// toggles between TOTP (default for users with an authenticator app) and an
// email-code fallback. The pre-2FA token is passed in navigation state so
// refresh = back to /auth/sign-in.
export default function TwoFactorPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const { adoptSession } = useAuth()
  const pre2faToken = location.state?.pre2faToken
  const userEmail = location.state?.email || ''

  const [mode, setMode] = useState('totp')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [sent, setSent] = useState(false)

  if (!pre2faToken) {
    return (
      <AuthShell>
        <h1>Two-factor</h1>
        <p>Your sign-in session has expired. Please sign in again.</p>
        <Link to="/auth/sign-in" style={{ textDecoration: 'none' }}>
          <PillButton variant="primary" block>Back to sign in</PillButton>
        </Link>
      </AuthShell>
    )
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const data = mode === 'totp'
        ? await authApi.verifyTOTP(code, pre2faToken)
        : await authApi.verifyEmail2FA(code, pre2faToken)
      adoptSession(data, 'email')
      navigate(postSignInPath(data?.user))
    } catch (err) {
      setError(err.message || 'Invalid code')
    } finally {
      setLoading(false)
    }
  }

  async function handleSendEmail() {
    setError('')
    try {
      await authApi.sendEmail2FA(pre2faToken)
      setSent(true)
    } catch (err) {
      setError(err.message || 'Failed to send code')
    }
  }

  return (
    <AuthShell>
      <h1>Two-factor verification</h1>
      <p>
        {mode === 'totp'
          ? 'Enter the 6-digit code from your authenticator app.'
          : `We sent a code to ${userEmail || 'your email'}. Enter it below.`}
      </p>

      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        {error && <div className="auth-alert auth-alert--error">{error}</div>}

        <CodeInput value={code} onChange={setCode} length={6} />

        <PillButton variant="primary" block type="submit" disabled={loading || code.length !== 6}>
          {loading ? 'Verifying\u2026' : 'Verify'}
        </PillButton>
      </form>

      <div className="auth-divider">or</div>

      {mode === 'totp' ? (
        <PillButton variant="secondary" block onClick={() => { setMode('email'); setCode(''); handleSendEmail() }}>
          Send a code by email instead
        </PillButton>
      ) : (
        <>
          <PillButton variant="secondary" block onClick={() => { setMode('totp'); setCode(''); setSent(false) }}>
            Use authenticator app instead
          </PillButton>
          {sent && <p style={{ fontSize: 13, color: 'var(--av-fg-muted)' }}>Check your inbox for the code.</p>}
        </>
      )}
    </AuthShell>
  )
}
