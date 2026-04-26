import { useState, useEffect } from 'react'
import { api } from '../api.js'

export default function AuditPage() {
  const [entries, setEntries] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    api.get('/curator/audit?limit=100')
      .then((d) => setEntries(d.entries || []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="loading">Loading…</div>

  return (
    <div className="page">
      <h1>Audit log</h1>
      {error && <div className="error">{error}</div>}
      {entries.length === 0 ? (
        <p className="empty">No audit entries yet.</p>
      ) : (
        <table className="audit">
          <thead>
            <tr>
              <th>Time</th><th>Actor</th><th>Entity</th><th>Action</th><th>Details</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.id}>
                <td>{new Date(e.createdAt).toLocaleString()}</td>
                <td>{e.actorKind} {e.actorId && `· ${e.actorId.slice(0, 8)}`}</td>
                <td>{e.entityKind} · {e.entityId.slice(0, 8)}</td>
                <td>{e.action}</td>
                <td className="audit-details">
                  {e.fieldChanges && JSON.stringify(e.fieldChanges)}
                  {e.aggregateMeta && JSON.stringify(e.aggregateMeta)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
