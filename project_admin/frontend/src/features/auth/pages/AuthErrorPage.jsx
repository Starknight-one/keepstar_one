import React from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'

const REASONS = {
  missing_params: 'The sign-in link was incomplete. Please try again.',
  state_mismatch: 'The sign-in session didn\u2019t match. Please start over.',
  expired: 'That sign-in link has expired. Start again.',
  google_failed: 'Google could not complete sign-in. Please retry.',
  access_denied: 'You declined the Google consent screen.',
}

export default function AuthErrorPage() {
  const [params] = useSearchParams()
  const reason = params.get('reason') || 'unknown'
  const message = REASONS[reason] || 'Something went wrong during sign-in. Please try again.'

  return (
    <AuthShell>
      <h1>Sign-in failed</h1>
      <p>{message}</p>
      <Link to="/auth/sign-in" style={{ textDecoration: 'none' }}>
        <PillButton variant="primary" block>Back to sign in</PillButton>
      </Link>
    </AuthShell>
  )
}
