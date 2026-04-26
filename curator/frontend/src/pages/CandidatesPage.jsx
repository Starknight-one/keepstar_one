import { useState, useEffect, useCallback } from 'react'
import { api } from '../api.js'

const TYPE_OPTIONS = [
  { value: 'text', label: 'TEXT' },
  { value: 'text_array', label: 'TEXT[]' },
  { value: 'integer', label: 'INTEGER' },
  { value: 'numeric', label: 'NUMERIC' },
  { value: 'boolean', label: 'BOOLEAN' },
]

export default function CandidatesPage() {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(null)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const data = await api.get('/curator/candidates/attributes?status=pending')
      setItems(data.candidates || [])
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  async function promote(c, columnType) {
    setBusy(c.id); setError('')
    try {
      await api.post(`/curator/candidates/attributes/${c.id}/promote`, {
        key: c.key,
        vertical: c.vertical,
        columnType,
      })
      refresh()
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(null)
    }
  }

  async function dismiss(c) {
    if (!confirm(`Dismiss "${c.key}"?`)) return
    setBusy(c.id); setError('')
    try {
      await api.post(`/curator/candidates/attributes/${c.id}/dismiss`)
      refresh()
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(null)
    }
  }

  if (loading) return <div className="loading">Loading…</div>

  return (
    <div className="page">
      <h1>Attribute candidates</h1>
      {error && <div className="error">{error}</div>}
      {items.length === 0 ? (
        <p className="empty">
          No pending candidates. They will accumulate as tenants import catalogs whose fields
          aren't in the typed master schema yet.
        </p>
      ) : (
        <ul className="cands">
          {items.map((c) => (
            <li key={c.id} className="cand">
              <div className="cand-row">
                <div>
                  <div className="cand-key">{c.key}</div>
                  <div className="cand-meta">
                    {c.vertical} · seen in {c.seenInTenants} tenants · {c.proposedType || 'unknown type'}
                  </div>
                  {c.sampleValues?.length > 0 && (
                    <div className="cand-samples">
                      Samples: {c.sampleValues.slice(0, 6).join(', ')}
                      {c.sampleValues.length > 6 && '…'}
                    </div>
                  )}
                  {c.agentMeta && <div className="cand-meta">{c.agentMeta}</div>}
                </div>
                <div className="cand-actions">
                  <PromoteForm c={c} busy={busy === c.id} onPromote={(t) => promote(c, t)} />
                  <button
                    className="btn-dismiss"
                    disabled={busy === c.id}
                    onClick={() => dismiss(c)}
                  >Dismiss</button>
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function PromoteForm({ c, busy, onPromote }) {
  const [type, setType] = useState(c.proposedType || 'text')
  return (
    <div className="promote">
      <select value={type} onChange={(e) => setType(e.target.value)}>
        {TYPE_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
      <button className="btn-promote" disabled={busy} onClick={() => onPromote(type)}>
        {busy ? 'Promoting…' : 'Promote'}
      </button>
    </div>
  )
}
