import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api.js'

export default function TenantsPage() {
  const [tenants, setTenants] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  useEffect(() => { load() }, [])
  async function load() {
    setLoading(true); setError(null)
    try {
      const { tenants } = await api.get('/curator/tenants')
      setTenants(tenants || [])
    } catch (e) { setError(e.message) }
    finally { setLoading(false) }
  }

  return (
    <div className="page">
      <h1>Tenants</h1>
      {error && <div className="error">{error}</div>}
      {loading ? <div className="loading">loading…</div> :
        tenants.length === 0 ? <div className="empty">no tenants</div> :
          <table className="tenants">
            <thead>
              <tr>
                <th>Slug</th><th>Name</th><th>Type</th>
                <th>Products</th><th>Master coverage</th>
                <th>Integrations</th><th>Schema</th>
              </tr>
            </thead>
            <tbody>
              {tenants.map(t => (
                <tr key={t.id}>
                  <td><Link to={`/tenants/${t.id}`} className="tenant-slug">{t.slug}</Link></td>
                  <td>{t.name}</td>
                  <td><span className="muted">{t.type || '—'}</span></td>
                  <td className="num">{t.productsCount.toLocaleString()}</td>
                  <td>
                    {t.productsCount > 0 ? (
                      <CoverageBar value={t.masterLinkCoverage} count={t.masterLinkedCount} total={t.productsCount} />
                    ) : <span className="muted">—</span>}
                  </td>
                  <td>{(t.integrations || []).map(i => (
                    <span key={i.kind} className={`int-badge int-${i.status}`}>{i.kind}</span>
                  )) || <span className="muted">—</span>}</td>
                  <td>{t.schemaStatus ? <span className={`schema-${t.schemaStatus}`}>{t.schemaStatus}</span> : <span className="muted">—</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>}
    </div>
  )
}

function CoverageBar({ value, count, total }) {
  const pct = Math.round(value * 100)
  const cls = pct >= 80 ? 'cov-high' : pct >= 30 ? 'cov-mid' : 'cov-low'
  return (
    <div className="coverage">
      <div className="cov-track"><div className={`cov-fill ${cls}`} style={{ width: `${pct}%` }} /></div>
      <span className="cov-text">{count}/{total} ({pct}%)</span>
    </div>
  )
}
