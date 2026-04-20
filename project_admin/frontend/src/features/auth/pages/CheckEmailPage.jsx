import React, { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { api } from '../../../shared/api/apiClient.js'

export default function CheckEmailPage() {
  const location = useLocation()
  const email = location.state?.email || ''
  const [cooldown, setCooldown] = useState(0)

  async function handleResend() {
    if (!email || cooldown > 0) return
    try {
      await api.post('/auth/password/forgot', { email })
      setCooldown(45)
      const interval = setInterval(() => {
        setCooldown((c) => {
          if (c <= 1) {
            clearInterval(interval)
            return 0
          }
          return c - 1
        })
      }, 1000)
    } catch (_) {}
  }

  return (
    <AuthShell>
      <h1>Check your email</h1>
      <p>
        We sent a reset link {email ? <>to <strong>{email}</strong></> : 'to your email'}.
        Click it to pick a new password. The link expires in 1 hour.
      </p>

      <PillButton variant="secondary" block onClick={handleResend} disabled={cooldown > 0}>
        {cooldown > 0 ? `Resend in 0:${String(cooldown).padStart(2, '0')}` : 'Resend email'}
      </PillButton>

      <p style={{ fontSize: 14 }}>
        Wrong email?{' '}
        <Link to="/auth/forgot-password" className="auth-link">Try another</Link>
      </p>
    </AuthShell>
  )
}
