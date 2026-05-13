import React, { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'

// Mirror of InstallCompletePage's storage key — must stay in sync.
const PENDING_SHOP_LINK_KEY = 'keepstar.pending_shop_link'

export default function WorkspacePickerPage() {
  const { adoptSession, user } = useAuth()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [tenants, setTenants] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selecting, setSelecting] = useState('')
  const [shopLinked, setShopLinked] = useState(false)

  // welcome_back=1 marks the OAuth auto-merge case: hold on the picker even
  // when the user owns only one workspace, so the banner is actually visible.
  const welcomeBack = params.get('welcome_back') === '1'
  const welcomeEmail = params.get('email') || ''

  useEffect(() => {
    // Pending shop link: the Shopify install flow couldn't email a magic-link
    // (shop had no owner email), so we left a token in sessionStorage. Now
    // that the user is authenticated, attach the orphan tenant to them
    // before listing tenants so the just-linked shop appears in the picker.
    const pendingToken = sessionStorage.getItem(PENDING_SHOP_LINK_KEY)
    const doConsume = async () => {
      if (pendingToken) {
        try {
          await authApi.consumePendingShopLink(pendingToken)
          setShopLinked(true)
        } catch (e) {
          // Token expired / already used — show a soft note, keep going.
          setError(e?.message || 'Could not attach the pending Shopify store.')
        } finally {
          sessionStorage.removeItem(PENDING_SHOP_LINK_KEY)
        }
      }
      try {
        const res = await authApi.listTenants()
        const list = res?.tenants || []
        setTenants(list)
        if (list.length <= 1 && !welcomeBack && !pendingToken) {
          navigate('/catalog', { replace: true })
        }
      } catch (err) {
        setError(err.message || 'Failed to load workspaces')
      } finally {
        setLoading(false)
      }
    }
    doConsume()
  }, [navigate, welcomeBack])

  async function handleSelect(tenantId) {
    setError('')
    setSelecting(tenantId)
    try {
      const pair = await authApi.selectTenant(tenantId)
      adoptSession({ ...pair, user })
      navigate('/catalog')
    } catch (err) {
      setError(err.message || 'Could not switch workspace')
      setSelecting('')
    }
  }

  return (
    <AuthShell>
      <h1>Pick a workspace</h1>
      <p>You belong to more than one workspace. Choose which one to open.</p>

      {welcomeBack && welcomeEmail && (
        <div className="auth-alert auth-alert--success">
          Welcome back — we connected Google to your existing account
          {' at '}<strong>{welcomeEmail}</strong>.
        </div>
      )}

      {shopLinked && (
        <div className="auth-alert auth-alert--success">
          Shopify store linked to your account. Pick the workspace to open.
        </div>
      )}

      {loading && <div className="auth-alert">Loading…</div>}
      {error && <div className="auth-alert auth-alert--error">{error}</div>}

      <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 12 }}>
        {tenants.map((t) => (
          <button
            key={t.tenant_id}
            type="button"
            className="auth-workspace-row"
            onClick={() => handleSelect(t.tenant_id)}
            disabled={!!selecting}
          >
            <div className="auth-workspace-row__name">{t.tenant_name || t.tenant_slug}</div>
            <div className="auth-workspace-row__meta">
              <span className="auth-workspace-row__role">{t.role}</span>
              {selecting === t.tenant_id && <span> · opening…</span>}
            </div>
          </button>
        ))}
      </div>
    </AuthShell>
  )
}
