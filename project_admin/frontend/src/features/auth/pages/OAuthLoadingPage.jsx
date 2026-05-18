import React, { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'
import { postSignInPath } from '../postSignIn.js'

// OAuthLoadingPage catches Google's redirect back with ?code=…&state=…,
// POSTs both to the backend, and installs the returned tokens. On any error
// we forward to /auth/error so the user lands on a consistent message page.
export default function OAuthLoadingPage() {
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

    authApi.googleCallback(code, state)
      .then((data) => {
        adoptSession(data, 'google')
        setStatus('Success — redirecting…')
        // Auto-merge: backend attached google_sub to an existing email account.
        // Surface a "Welcome back" banner on the destination page so the user
        // sees what just happened instead of silently landing in a familiar inbox.
        const base = postSignInPath(data?.user)
        const linked = data && data.linked_from_email
        if (linked && base === '/auth/pick-workspace') {
          // Welcome-back banner only makes sense when landing on the picker —
          // not when we're showing the set-password promo.
          const q = `?welcome_back=1&email=${encodeURIComponent(linked)}`
          navigate(`${base}${q}`, { replace: true })
        } else {
          navigate(base, { replace: true })
        }
      })
      .catch((err) => {
        const reason = (err && err.message) || 'google_failed'
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
