import React from 'react'
import { Link } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'

export default function PasswordChangedPage() {
  return (
    <AuthShell>
      <h1>Password updated</h1>
      <p>Your password has been changed. Use it to sign in from now on.</p>
      <Link to="/auth/sign-in" style={{ textDecoration: 'none' }}>
        <PillButton variant="primary" block>Back to sign in</PillButton>
      </Link>
    </AuthShell>
  )
}
