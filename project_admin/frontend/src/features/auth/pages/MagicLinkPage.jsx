import React, { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'

// MagicLinkPage handles the click-target of a magic-link email. The backend
// signs a single-use code into the URL; we POST it for exchange, install the
// returned tokens, and forward to the workspace picker (same shape as the
// Google OAuth landing). On any error we hop to /auth/error so the user lands
// on a consistent message page.
export default function MagicLinkPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { adoptSession } = useAuth()
  const [status, setStatus] = useState('Signing you in…')
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true

    const code = params.get('code')
    if (!code) {
      navigate('/auth/error?reason=missing_code', { replace: true })
      return
    }

    authApi.magicConsume(code)
      .then((data) => {
        adoptSession(data)
        setStatus('Success — redirecting…')
        navigate('/auth/pick-workspace', { replace: true })
      })
      .catch((err) => {
        const reason = (err && err.message) || 'magic_link_failed'
        navigate(`/auth/error?reason=${encodeURIComponent(reason)}`, { replace: true })
      })
  }, [params, navigate, adoptSession])

  return (
    <AuthShell>
      <h1>Just a moment</h1>
      <p>{status}</p>
    </AuthShell>
  )
}
