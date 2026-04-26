import { useState, useEffect, useCallback } from 'react'
import { Plus, Trash2, Copy, AlertTriangle, Key } from 'lucide-react'
import { api } from '../../shared/api/apiClient.js'
import Button from '../../shared/ui/Button.jsx'
import Input from '../../shared/ui/Input.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './apiKeys.css'

export default function ApiKeysPage() {
  const [keys, setKeys] = useState([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [label, setLabel] = useState('')
  const [busy, setBusy] = useState(false)
  const [createdKey, setCreatedKey] = useState(null) // { plainKey, label, ... }
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await api.get('/api-keys')
      setKeys(data.keys || [])
    } catch (e) {
      setError(e.message || 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  async function handleCreate() {
    setBusy(true)
    setError('')
    try {
      const res = await api.post('/api-keys', { label })
      setCreatedKey(res)
      setLabel('')
      refresh()
    } catch (e) {
      setError(e.message || 'Failed to create')
    } finally {
      setBusy(false)
    }
  }

  async function handleRevoke(id) {
    if (!confirm('Revoke this key? Calls using it will fail immediately.')) return
    setError('')
    try {
      await api.delete(`/api-keys/${id}`)
      refresh()
    } catch (e) {
      setError(e.message || 'Failed to revoke')
    }
  }

  function copyToClipboard(text) {
    if (navigator?.clipboard) navigator.clipboard.writeText(text)
  }

  if (loading) return <div className="center-spinner"><Spinner /></div>

  return (
    <div className="ak-page">
      <div className="page-header">
        <div>
          <div className="page-title">API access</div>
          <div className="catalog-breadcrumb">Settings / <strong>API access</strong></div>
        </div>
        <Button variant="primary" pill onClick={() => setShowCreate(true)}>
          <Plus size={14} /> Generate key
        </Button>
      </div>

      <div className="ak-hint">
        Tokens authorize programmatic access to <code>/api/v1/*</code> via the
        <code> X-API-Key</code> header. Plain text is shown once on creation —
        store it in your secret manager.
      </div>

      {error && <div className="pd-message error">{error}</div>}

      {createdKey && (
        <div className="ak-newkey">
          <div className="ak-newkey-warn"><AlertTriangle size={16} /> Copy this key now. It will not be shown again.</div>
          <div className="ak-newkey-row">
            <code className="ak-newkey-code">{createdKey.plainKey}</code>
            <Button variant="secondary" pill onClick={() => copyToClipboard(createdKey.plainKey)}>
              <Copy size={14} /> Copy
            </Button>
            <Button variant="primary" pill onClick={() => setCreatedKey(null)}>
              I saved it
            </Button>
          </div>
        </div>
      )}

      {showCreate && !createdKey && (
        <div className="ak-form">
          <Input
            label="Label (optional)"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="production"
          />
          <div className="ak-form-actions">
            <Button variant="primary" pill disabled={busy} onClick={handleCreate}>
              {busy ? 'Generating…' : 'Generate'}
            </Button>
            <Button variant="secondary" pill onClick={() => { setShowCreate(false); setLabel('') }}>
              Cancel
            </Button>
          </div>
        </div>
      )}

      {keys.length === 0 ? (
        <div className="ak-empty">
          <Key size={28} />
          <p>No API keys yet.</p>
        </div>
      ) : (
        <table className="ak-table">
          <thead>
            <tr>
              <th>Label</th>
              <th>Prefix</th>
              <th>Created</th>
              <th>Last used</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {keys.map((k) => (
              <tr key={k.id} className={k.revokedAt ? 'ak-row-revoked' : ''}>
                <td>{k.label || '—'}</td>
                <td><code>{k.keyPrefix || '—'}</code></td>
                <td>{new Date(k.createdAt).toLocaleDateString()}</td>
                <td>{k.lastUsedAt ? new Date(k.lastUsedAt).toLocaleDateString() : '—'}</td>
                <td>
                  {k.revokedAt
                    ? <span className="ak-status revoked">revoked</span>
                    : <span className="ak-status active">active</span>}
                </td>
                <td>
                  {!k.revokedAt && (
                    <button className="ce-action ce-action-danger" onClick={() => handleRevoke(k.id)} title="Revoke">
                      <Trash2 size={13} />
                    </button>
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
