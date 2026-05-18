import { useState } from 'react'
import { ShieldCheck, X } from 'lucide-react'
import { authApi } from '../auth/api/authApi.js'
import Button from '../../shared/ui/Button.jsx'
import Input from '../../shared/ui/Input.jsx'

// SecurityPage — Settings → Security. Today exposes the TOTP disable flow
// with a re-auth modal (the backend requires the current TOTP code per sc 89).
// TOTP setup (QR + confirm) is reached from the login-time 2FA prompt; we
// don't yet host an "enable from settings" path — that's a separate task.
export default function SecurityPage() {
  const [modalOpen, setModalOpen] = useState(false)
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [done, setDone] = useState(false)

  async function handleDisable() {
    if (!/^\d{6}$/.test(code.trim())) {
      setError('Enter the 6-digit code from your authenticator app')
      return
    }
    setBusy(true)
    setError('')
    try {
      await authApi.disableTOTP(code.trim())
      setDone(true)
      setModalOpen(false)
      setCode('')
    } catch (e) {
      setError(e.message || 'Invalid code')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="ak-page">
      <div className="page-header">
        <div>
          <div className="page-title">Security</div>
          <div className="catalog-breadcrumb">Settings / <strong>Security</strong></div>
        </div>
      </div>

      <div className="ak-hint">
        Two-factor authentication adds a second proof of identity on sign-in.
        After turning it on, every sign-in asks for a code from your authenticator app.
      </div>

      {done && (
        <div className="ak-newkey">
          <div className="ak-newkey-warn"><ShieldCheck size={16} /> Two-factor authentication disabled.</div>
        </div>
      )}

      <div className="ak-form" style={{ maxWidth: 520 }}>
        <div style={{ fontWeight: 600, marginBottom: 6 }}>Authenticator app (TOTP)</div>
        <div style={{ color: '#5a6072', fontSize: 13, marginBottom: 12 }}>
          To disable, confirm with a fresh 6-digit code from your authenticator app.
          This blocks a stolen session from stripping your second factor.
        </div>
        <div className="ak-form-actions">
          <Button variant="primary" pill onClick={() => { setModalOpen(true); setError(''); setDone(false) }}>
            Disable 2FA
          </Button>
        </div>
      </div>

      {modalOpen && (
        <div
          style={{
            position: 'fixed', inset: 0, background: 'rgba(15, 23, 42, 0.45)',
            display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
          }}
          onClick={() => !busy && setModalOpen(false)}
        >
          <div
            style={{
              background: '#fff', borderRadius: 8, padding: 24, minWidth: 360,
              boxShadow: '0 12px 40px rgba(15, 23, 42, 0.2)',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
              <div style={{ fontWeight: 600, fontSize: 16 }}>Confirm disable 2FA</div>
              <button
                onClick={() => !busy && setModalOpen(false)}
                style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#5a6072' }}
                aria-label="Close"
              >
                <X size={18} />
              </button>
            </div>
            <div style={{ color: '#5a6072', fontSize: 13, marginBottom: 16 }}>
              Open your authenticator app and enter the current 6-digit code.
            </div>
            <Input
              label="Authenticator code"
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              placeholder="123456"
              autoFocus
            />
            {error && <div className="pd-message error" style={{ marginTop: 8 }}>{error}</div>}
            <div className="ak-form-actions" style={{ marginTop: 16 }}>
              <Button variant="primary" pill disabled={busy} onClick={handleDisable}>
                {busy ? 'Verifying…' : 'Disable 2FA'}
              </Button>
              <Button variant="secondary" pill disabled={busy} onClick={() => setModalOpen(false)}>
                Cancel
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
