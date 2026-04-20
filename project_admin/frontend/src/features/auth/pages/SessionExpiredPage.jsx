import React from 'react'
import { Link } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'

export default function SessionExpiredPage() {
  return (
    <AuthShell>
      <h1>Session expired</h1>
      <p>For your security we signed you out. Sign in again to keep working.</p>
      <Link to="/auth/sign-in" style={{ textDecoration: 'none' }}>
        <PillButton variant="primary" block>Sign in</PillButton>
      </Link>
    </AuthShell>
  )
}
