import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import AuthShell from '../layout/AuthShell.jsx'
import PillButton from '../layout/PillButton.jsx'
import { useAuth } from '../AuthProvider.jsx'
import { authApi } from '../api/authApi.js'

export default function WorkspacePickerPage() {
  const { adoptSession, user } = useAuth()
  const navigate = useNavigate()
  const [tenants, setTenants] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selecting, setSelecting] = useState('')

  useEffect(() => {
    authApi
      .listTenants()
      .then((res) => {
        const list = res?.tenants || []
        setTenants(list)
        if (list.length <= 1) {
          navigate('/catalog', { replace: true })
        }
      })
      .catch((err) => setError(err.message || 'Failed to load workspaces'))
      .finally(() => setLoading(false))
  }, [navigate])

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
