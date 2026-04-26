import { useState, useEffect, useCallback } from 'react'
import { api } from '../api.js'

export default function JunkPage() {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(null)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const data = await api.get('/curator/junk?status=pending')
      setItems(data.candidates || [])
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  async function classify(item, classification) {
    setBusy(item.id); setError('')
    try {
      await api.post(`/curator/junk/${item.id}/classify`, { classification })
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
      <h1>Junk candidates</h1>
      {error && <div className="error">{error}</div>}
      {items.length === 0 ? (
        <p className="empty">No pending junk candidates across any tenant.</p>
      ) : (
        <ul className="cands">
          {items.map((j) => (
            <li key={j.id} className="cand">
              <div className="cand-row">
                <div>
                  <div className="cand-key">tenant {j.tenantId.slice(0, 8)} · listing {j.listingId.slice(0, 8)}</div>
                  <div className="cand-samples">{JSON.stringify(j.detectedReason)}</div>
                </div>
                <div className="cand-actions">
                  <button className="btn-promote" disabled={busy === j.id}
                    onClick={() => classify(j, 'confirmed_addon')}>Mark add-on</button>
                  <button className="btn-dismiss" disabled={busy === j.id}
                    onClick={() => classify(j, 'false_positive')}>Mark real</button>
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
