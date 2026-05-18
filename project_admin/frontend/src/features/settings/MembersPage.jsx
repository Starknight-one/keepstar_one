import { useState, useEffect, useCallback } from 'react'
import { Mail, Plus, Send } from 'lucide-react'
import { authApi } from '../auth/api/authApi.js'
import Button from '../../shared/ui/Button.jsx'
import Input from '../../shared/ui/Input.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'

// MembersPage — Settings → Members. Shows pending + accepted invitations and
// lets the owner re-send the email when the initial mailer.Send failed (sc 82).
export default function MembersPage() {
  const [invitations, setInvitations] = useState([])
  const [loading, setLoading] = useState(true)
  const [showInvite, setShowInvite] = useState(false)
  const [email, setEmail] = useState('')
  const [role, setRole] = useState('member')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [resendingId, setResendingId] = useState('')
  const [info, setInfo] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await authApi.listInvitations()
      setInvitations(data.invitations || [])
    } catch (e) {
      setError(e.message || 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  async function handleInvite() {
    setBusy(true)
    setError('')
    setInfo('')
    try {
      await authApi.createInvite(email.trim().toLowerCase(), role)
      setEmail('')
      setShowInvite(false)
      setInfo('Invitation sent.')
      refresh()
    } catch (e) {
      setError(e.message || 'Failed to invite')
    } finally {
      setBusy(false)
    }
  }

  async function handleResend(id) {
    setResendingId(id)
    setError('')
    setInfo('')
    try {
      await authApi.resendInvitation(id)
      setInfo('Invitation re-sent.')
      refresh()
    } catch (e) {
      setError(e.message || 'Failed to resend')
    } finally {
      setResendingId('')
    }
  }

  if (loading) return <div className="center-spinner"><Spinner /></div>

  return (
    <div className="ak-page">
      <div className="page-header">
        <div>
          <div className="page-title">Members</div>
          <div className="catalog-breadcrumb">Settings / <strong>Members</strong></div>
        </div>
        <Button variant="primary" pill onClick={() => setShowInvite(true)}>
          <Plus size={14} /> Invite member
        </Button>
      </div>

      <div className="ak-hint">
        Invite teammates to this workspace. They receive an email with a single-use
        link. If the email never arrived, use <strong>Resend</strong> to issue a fresh link.
      </div>

      {error && <div className="pd-message error">{error}</div>}
      {info && <div className="pd-message" style={{ color: '#1b8c4a' }}>{info}</div>}

      {showInvite && (
        <div className="ak-form">
          <Input
            label="Email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="colleague@example.com"
            type="email"
          />
          <div style={{ marginTop: 12, marginBottom: 12 }}>
            <label style={{ display: 'block', fontSize: 13, fontWeight: 600, marginBottom: 4 }}>Role</label>
            <select
              value={role}
              onChange={(e) => setRole(e.target.value)}
              style={{ padding: '8px 12px', borderRadius: 6, border: '1px solid #d6dae3', fontSize: 14 }}
            >
              <option value="owner">Owner</option>
              <option value="admin">Admin</option>
              <option value="member">Member</option>
              <option value="viewer">Viewer</option>
            </select>
          </div>
          <div className="ak-form-actions">
            <Button variant="primary" pill disabled={busy || !email.trim()} onClick={handleInvite}>
              {busy ? 'Sending…' : 'Send invitation'}
            </Button>
            <Button variant="secondary" pill onClick={() => { setShowInvite(false); setEmail('') }}>
              Cancel
            </Button>
          </div>
        </div>
      )}

      {invitations.length === 0 ? (
        <div className="ak-empty">
          <Mail size={28} />
          <p>No invitations yet.</p>
        </div>
      ) : (
        <table className="ak-table">
          <thead>
            <tr>
              <th>Email</th>
              <th>Role</th>
              <th>Status</th>
              <th>Sent</th>
              <th>Expires</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {invitations.map((inv) => {
              const accepted = !!inv.accepted_at
              const expired = !accepted && new Date(inv.expires_at) < new Date()
              return (
                <tr key={inv.id}>
                  <td>{inv.email}</td>
                  <td>{inv.role}</td>
                  <td>
                    {accepted ? (
                      <span className="ak-status active">joined</span>
                    ) : expired ? (
                      <span className="ak-status revoked">expired</span>
                    ) : (
                      <span className="ak-status">pending</span>
                    )}
                  </td>
                  <td>{new Date(inv.created_at).toLocaleDateString()}</td>
                  <td>{new Date(inv.expires_at).toLocaleDateString()}</td>
                  <td>
                    {!accepted && (
                      <Button
                        variant="secondary"
                        pill
                        disabled={resendingId === inv.id}
                        onClick={() => handleResend(inv.id)}
                      >
                        <Send size={13} /> {resendingId === inv.id ? 'Sending…' : 'Resend'}
                      </Button>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
    </div>
  )
}
