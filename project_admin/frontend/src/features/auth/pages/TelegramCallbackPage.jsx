import React, { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'

// TelegramCallbackPage catches Telegram's OIDC redirect with ?code=…&state=…,
// POSTs them to the backend, and installs the returned session.
export default function TelegramCallbackPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { adoptSession } = useAuth()
  const [status, setStatus] = useState('Signing you in…')
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true

    const code = params.get('code')
    const state = params.get('state')
    const errParam = params.get('error')

    if (errParam) {
      navigate(`/auth/error?reason=${encodeURIComponent(errParam)}`, { replace: true })
      return
    }
    if (!code || !state) {
      navigate('/auth/error?reason=missing_params', { replace: true })
      return
    }

    authApi.telegramOIDCCallback(code, state)
      .then((data) => {
        adoptSession(data)
        setStatus('Success — redirecting…')
        navigate('/auth/pick-workspace', { replace: true })
      })
      .catch((err) => {
        const reason = (err && err.message) || 'telegram_failed'
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
