import React, { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'
import { postSignInPath } from '../postSignIn.js'

// InviteAcceptPage is the landing page for `/auth/accept-invite?token=...`
// linked from the invitation email. If the invitee is already signed in we
// just link the membership; otherwise we ask them to set a password.
export default function InviteAcceptPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { adoptSession, user } = useAuth()
  const token = params.get('token') || ''

  const [loading, setLoading] = useState(true)
  const [preview, setPreview] = useState(null)
  const [error, setError] = useState('')
  const [password, setPassword] = useState('')
  const [accepting, setAccepting] = useState(false)

  useEffect(() => {
    if (!token) {
      setError('Missing invitation token.')
      setLoading(false)
      return
    }
    authApi
      .previewInvite(token)
      .then(setPreview)
      .catch((err) => setError(err.message || 'Invitation is invalid or expired.'))
      .finally(() => setLoading(false))
  }, [token])

  async function handleAccept(e) {
    e?.preventDefault?.()
    setError('')
    setAccepting(true)
    try {
      const data = await authApi.acceptInvite(token, user ? '' : password)
      if (data?.access_token) {
        adoptSession(data, 'email')
      }
      navigate(postSignInPath(data?.user))
    } catch (err) {
      setError(err.message || 'Could not accept invitation.')
      setAccepting(false)
    }
  }

  if (loading) {
    return (
      <AuthShell>
        <h1>Loading invitation…</h1>
      </AuthShell>
    )
  }

  if (error && !preview) {
    return (
      <AuthShell>
        <h1>Invitation unavailable</h1>
        <p>{error}</p>
        <Link to="/auth/sign-in" style={{ textDecoration: 'none' }}>
          <PillButton variant="primary" block>Back to sign in</PillButton>
        </Link>
      </AuthShell>
    )
  }

  const needsPassword = !user

  return (
    <AuthShell>
      <h1>Join {preview.tenant_name}</h1>
      <p>
        {preview.inviter_name ? <><strong>{preview.inviter_name}</strong> invited </> : 'You were invited '}
        you to <strong>{preview.tenant_name}</strong> as{' '}
        <strong>{preview.role}</strong>.
      </p>

      <form onSubmit={handleAccept} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        {error && <div className="auth-alert auth-alert--error">{error}</div>}

        {needsPassword && (
          <div className="auth-field">
            <label className="auth-field__label" htmlFor="password">Set a password</label>
            <input
              id="password"
              type="password"
              className="auth-field__input"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="At least 10 characters"
              minLength={10}
              required
            />
          </div>
        )}

        <PillButton variant="primary" block type="submit" disabled={accepting || (needsPassword && password.length < 10)}>
          {accepting ? 'Joining…' : 'Accept invitation'}
        </PillButton>
      </form>
    </AuthShell>
  )
}
