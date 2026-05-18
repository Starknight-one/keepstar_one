import React, { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'
import { postSignInPath } from '../postSignIn.js'

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
        adoptSession(data, 'email')
        setStatus('Success — redirecting…')
        navigate(postSignInPath(data?.user), { replace: true })
      })
      .catch((err) => {
        const msg = (err && err.message) || ''
        // Scenarios 40, 41: friendly expired/used screen with a re-request
        // form. Backend collapses both cases into the same 401 + message;
        // we route on the message substring to the dedicated page.
        if (/expired|already used|single-use/i.test(msg)) {
          navigate('/auth/magic-expired', { replace: true })
          return
        }
        navigate(`/auth/error?reason=${encodeURIComponent(msg || 'magic_link_failed')}`, { replace: true })
      })
  }, [params, navigate, adoptSession])

  return (
    <AuthShell>
      <h1>Just a moment</h1>
      <p>{status}</p>
    </AuthShell>
  )
}
