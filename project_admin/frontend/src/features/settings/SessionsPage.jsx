import { useEffect, useState } from 'react'
import { authApi } from '../auth/api/authApi.js'
import Button from '../../shared/ui/Button.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './settings.css'

// Settings → Sessions: shows every active refresh-token family for the
// current user. The row tagged current_session is the device the user is
// looking at right now — we never let them revoke it from here (use Sign
// out instead). Other rows can be revoked one-by-one when the user
// remembers leaving an admin session open on a borrowed laptop.
export default function SessionsPage() {
  const [sessions, setSessions] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [revoking, setRevoking] = useState(null)

  async function load() {
    setLoading(true)
    setError('')
    try {
      const list = await authApi.listSessions()
      setSessions(Array.isArray(list) ? list : [])
    } catch (err) {
      setError(err.message || 'Failed to load sessions')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  async function handleRevoke(id) {
    if (!confirm('Sign out this device? They will need to sign in again.')) return
    setRevoking(id)
    try {
      await authApi.revokeSession(id)
      await load()
    } catch (err) {
      setError(err.message || 'Failed to revoke session')
    } finally {
      setRevoking(null)
    }
  }

  if (loading) return <div className="center-spinner"><Spinner /></div>

  return (
    <div>
      <h1 className="page-title">Active sessions</h1>
      <p className="page-subtitle">
        Each row is a device that's signed in to your account. Sign out anyone
        you don't recognize. Your current device is highlighted.
      </p>

      {error && <div className="auth-error">{error}</div>}

      {sessions.length === 0 ? (
        <p style={{ color: 'var(--text-muted)' }}>No active sessions found.</p>
      ) : (
        <table className="sessions-table">
          <thead>
            <tr>
              <th>Device</th>
              <th>Location</th>
              <th>Signed in</th>
              <th>Expires</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((s) => (
              <tr key={s.id} className={s.current_session ? 'sessions-table__row--current' : ''}>
                <td>
                  <div className="sessions-table__device">
                    {formatDevice(s)}
                    {s.current_session && <span className="sessions-table__current-badge">This device</span>}
                  </div>
                  <div className="sessions-table__ua-raw" title={s.user_agent}>
                    {truncate(s.user_agent, 80)}
                  </div>
                </td>
                <td>{s.geo || s.ip || '—'}</td>
                <td>{formatDate(s.created_at)}</td>
                <td>{formatDate(s.expires_at)}</td>
                <td>
                  {!s.current_session && (
                    <Button
                      variant="ghost"
                      onClick={() => handleRevoke(s.id)}
                      disabled={revoking === s.id}
                    >
                      {revoking === s.id ? 'Signing out…' : 'Sign out'}
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function formatDevice(s) {
  const parts = []
  if (s.browser) {
    parts.push(s.browser_version ? `${s.browser} ${s.browser_version}` : s.browser)
  }
  if (s.os) parts.push(s.os)
  if (s.device_kind && s.device_kind !== 'desktop') parts.push(`(${s.device_kind})`)
  return parts.length > 0 ? parts.join(' · ') : 'Unknown device'
}

function formatDate(iso) {
  if (!iso) return '—'
  try {
    const d = new Date(iso)
    return d.toLocaleString(undefined, {
      year: 'numeric', month: 'short', day: 'numeric',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return iso
  }
}

function truncate(s, n) {
  if (!s) return ''
  return s.length <= n ? s : s.slice(0, n - 1) + '…'
}
