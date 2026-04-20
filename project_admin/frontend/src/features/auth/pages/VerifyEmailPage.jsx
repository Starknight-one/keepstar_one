import React, { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { api } from '../../../shared/api/apiClient.js'

export default function VerifyEmailPage() {
  const [params] = useSearchParams()
  const token = params.get('token') || ''
  const [state, setState] = useState(token ? 'verifying' : 'missing')
  const [error, setError] = useState('')

  useEffect(() => {
    if (!token) return
    api.post('/auth/email/verify', { token })
      .then(() => setState('success'))
      .catch((err) => {
        setError(err.message || 'Verification failed')
        setState('error')
      })
  }, [token])

  if (state === 'missing') {
    return (
      <AuthShell>
        <h1>Missing token</h1>
        <p>This verification link is incomplete. Check the email again or request a new one.</p>
      </AuthShell>
    )
  }

  if (state === 'verifying') {
    return (
      <AuthShell>
        <h1>Verifying…</h1>
        <p>Hang tight while we confirm your email.</p>
      </AuthShell>
    )
  }

  if (state === 'error') {
    return (
      <AuthShell>
        <h1>Verification failed</h1>
        <p>{error || 'The link may have expired.'}</p>
        <Link to="/auth/sign-in" style={{ textDecoration: 'none' }}>
          <PillButton variant="primary" block>Back to sign in</PillButton>
        </Link>
      </AuthShell>
    )
  }

  return (
    <AuthShell>
      <h1>Email verified</h1>
      <p>You're all set. You can now sign in to your workspace.</p>
      <Link to="/auth/sign-in" style={{ textDecoration: 'none' }}>
        <PillButton variant="primary" block>Continue to sign in</PillButton>
      </Link>
    </AuthShell>
  )
}
