import React, { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'
import { postSignInPath } from '../postSignIn.js'

// TelegramCallbackPage handles Telegram's Login Widget return.
//
// oauth.telegram.org is NOT a real OIDC server: it ignores response_type
// and redirect_uri, and posts the user payload as a URL hash
// `#tgAuthResult=<base64url(JSON widget payload)>`. The JSON is the same
// {id, first_name, ..., hash} shape the legacy Login Widget emits, with
// the hash being an HMAC the backend already knows how to verify via
// POST /admin/api/auth/telegram/callback.
export default function TelegramCallbackPage() {
  const navigate = useNavigate()
  const { adoptSession } = useAuth()
  const [status, setStatus] = useState('Signing you in…')
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true

    const hash = window.location.hash || ''
    const params = new URLSearchParams(hash.startsWith('#') ? hash.slice(1) : hash)
    const tgResult = params.get('tgAuthResult')

    if (!tgResult) {
      navigate('/auth/error?reason=missing_params', { replace: true })
      return
    }

    let payload
    try {
      // Telegram uses base64url without padding; restore standard base64.
      let b64 = tgResult.replace(/-/g, '+').replace(/_/g, '/')
      while (b64.length % 4) b64 += '='
      payload = JSON.parse(atob(b64))
    } catch (_) {
      navigate('/auth/error?reason=missing_params', { replace: true })
      return
    }

    authApi.telegramCallback(payload)
      .then((data) => {
        adoptSession(data, 'telegram')
        setStatus('Success — redirecting…')
        navigate(postSignInPath(data?.user), { replace: true })
      })
      .catch((err) => {
        const reason = (err && err.message) || 'telegram_failed'
        navigate(`/auth/error?reason=${encodeURIComponent(reason)}`, { replace: true })
      })
  }, [navigate, adoptSession])

  return (
    <AuthShell>
      <h1>Just a moment</h1>
      <p>{status}</p>
    </AuthShell>
  )
}
