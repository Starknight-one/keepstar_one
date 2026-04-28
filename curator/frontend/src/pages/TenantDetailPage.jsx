import { useEffect, useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { api, mergeApi } from '../api.js'

export default function TenantDetailPage() {
  const { id } = useParams()
  const [tenant, setTenant] = useState(null)
  const [tab, setTab] = useState('catalog')
  const [error, setError] = useState(null)

  useEffect(() => {
    api.get(`/curator/tenants/${id}`).then(setTenant).catch(e => setError(e.message))
  }, [id])

  if (error) return <div className="page"><div className="error">{error}</div></div>
  if (!tenant) return <div className="page"><div className="loading">loading…</div></div>

  return (
    <div className="page">
      <div className="breadcrumb"><Link to="/tenants">Tenants</Link> / {tenant.slug}</div>
      <h1>{tenant.name}</h1>
      <div className="tenant-meta">
        <Pill label="type" value={tenant.type || '—'} />
        <Pill label="products" value={tenant.productsCount.toLocaleString()} />
        <Pill label="master coverage" value={`${Math.round(tenant.masterLinkCoverage * 100)}% (${tenant.masterLinkedCount}/${tenant.productsCount})`} />
        <Pill label="schema" value={tenant.schemaStatus || 'none'} />
        {tenant.integrations.map(i => (
          <Pill key={i.kind} label={i.kind} value={i.status || '—'} />
        ))}
      </div>

      <div className="tabs">
        {['catalog', 'schema', 'reports', 'audit'].map(t => (
          <button key={t} className={`tab ${tab === t ? 'tab-active' : ''}`} onClick={() => setTab(t)}>
            {t}
          </button>
        ))}
      </div>

      {tab === 'catalog' && <CatalogTab tenantId={id} />}
      {tab === 'schema' && <SchemaTab tenantId={id} />}
      {tab === 'reports' && <ReportsTab tenantId={id} />}
      {tab === 'audit' && <AuditTab />}
    </div>
  )
}

function Pill({ label, value }) {
  return <div className="pill"><span className="pill-label">{label}</span><span className="pill-value">{value}</span></div>
}

function CatalogTab({ tenantId }) {
  const [products, setProducts] = useState([])
  const [total, setTotal] = useState(0)
  const [search, setSearch] = useState('')
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const limit = 50

  useEffect(() => { load() }, [tenantId, search, offset])
  async function load() {
    setLoading(true)
    try {
      const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
      if (search) params.set('search', search)
      const data = await api.get(`/curator/tenants/${tenantId}/products?${params}`)
      setProducts(data.products || [])
      setTotal(data.total || 0)
    } catch {} finally { setLoading(false) }
  }

  return (
    <div>
      <div className="toolbar">
        <input className="search" placeholder="search by name / SKU / brand…"
          value={search} onChange={e => { setSearch(e.target.value); setOffset(0) }} />
        <span className="muted">total {total.toLocaleString()}</span>
      </div>
      {loading ? <div className="loading">loading…</div> :
        products.length === 0 ? <div className="empty">no products</div> :
          <table className="products">
            <thead>
              <tr>
                <th></th><th>Name</th><th>SKU</th><th>Brand</th>
                <th>Price</th><th>Stock</th><th>Master</th><th>Source</th>
              </tr>
            </thead>
            <tbody>
              {products.map(p => (
                <tr key={p.id}>
                  <td>{p.imageUrl ? <img src={p.imageUrl} alt="" className="thumb" /> : <div className="thumb" />}</td>
                  <td>
                    <div className="prod-name">{p.name}</div>
                    {p.originalName && p.originalName !== p.name && <div className="prod-sub">{p.originalName}</div>}
                  </td>
                  <td className="mono">{p.sku || <span className="muted">—</span>}</td>
                  <td>{p.brand || <span className="muted">—</span>}</td>
                  <td className="num">{p.priceCents ? `${(p.priceCents / 100).toFixed(2)} ${p.currency || ''}` : <span className="muted">—</span>}</td>
                  <td className="num">{p.stock}</td>
                  <td>{p.hasMasterLink ?
                    <Link to={`/master/${p.masterId}`} className="link-master">linked</Link> :
                    <span className="muted">—</span>}</td>
                  <td><span className="muted">{p.sourceSystem || '—'}</span></td>
                </tr>
              ))}
            </tbody>
          </table>}
      <Pagination offset={offset} limit={limit} total={total} onOffset={setOffset} />
    </div>
  )
}

function SchemaTab({ tenantId }) {
  const [schema, setSchema] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.get(`/curator/tenants/${tenantId}/schema`).then(setSchema).finally(() => setLoading(false))
  }, [tenantId])

  if (loading) return <div className="loading">loading…</div>
  if (!schema || !schema.status) return <div className="empty">no mapping artifact yet — discovery hasn't been run for this tenant</div>

  return (
    <div className="schema-view">
      <div className="schema-meta">
        <Pill label="status" value={schema.status} />
        <Pill label="version" value={String(schema.artifactVersion)} />
        {schema.discoveredAt && <Pill label="discovered" value={new Date(schema.discoveredAt).toLocaleString()} />}
        {schema.validatedAt && <Pill label="validated" value={new Date(schema.validatedAt).toLocaleString()} />}
      </div>
      {schema.validationReport && (
        <details open className="schema-section">
          <summary>Validation report</summary>
          <pre className="json">{JSON.stringify(schema.validationReport, null, 2)}</pre>
        </details>
      )}
      {schema.mappingArtifact && (
        <details className="schema-section">
          <summary>Mapping artifact</summary>
          <pre className="json">{JSON.stringify(schema.mappingArtifact, null, 2)}</pre>
        </details>
      )}
    </div>
  )
}

function ReportsTab({ tenantId }) {
  const [reports, setReports] = useState([])
  const [loading, setLoading] = useState(true)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState(null)
  const navigate = useNavigate()

  useEffect(() => { load() }, [tenantId])
  async function load() {
    setLoading(true)
    try {
      const res = await mergeApi.list(tenantId, 20)
      setReports(res.reports || [])
      setError(null)
    } catch (e) { setError(e.message) }
    finally { setLoading(false) }
  }

  async function runAgent() {
    if (!confirm('Run merge agent for this tenant? Walks every listing and writes a fresh report (read-only — no master changes).')) return
    setRunning(true)
    try {
      const res = await mergeApi.run(tenantId)
      navigate(`/tenants/${tenantId}/merge-reports/${res.reportId}`)
    } catch (e) {
      alert(`Run failed: ${e.message}`)
    } finally {
      setRunning(false)
    }
  }

  if (loading) return <div className="loading">loading…</div>

  return (
    <div>
      <div className="toolbar">
        <button className="btn-primary" onClick={runAgent} disabled={running}>
          {running ? 'Running agent…' : '+ Run merge agent'}
        </button>
        <span className="muted">writes a new report; previous pending reports get superseded</span>
      </div>
      {error && <div className="error">{error}</div>}
      {reports.length === 0 ? (
        <div className="empty">No reports yet — run the merge agent to produce the first one.</div>
      ) : (
        <table className="products">
          <thead>
            <tr>
              <th>ID</th><th>Generated</th><th>Status</th>
              <th>Total</th><th>New master</th><th>Auto-link</th>
              <th>Needs review</th><th>Skip</th><th>Already linked</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {reports.map(r => (
              <tr key={r.id}>
                <td className="mono">#{r.id}</td>
                <td>{new Date(r.generatedAt).toLocaleString()}</td>
                <td><span className={`status-badge status-${r.status}`}>{r.status}</span></td>
                <td className="num">{r.totalListings}</td>
                <td className="num">{r.newMasterCount}</td>
                <td className="num">{r.autoLinkCount}</td>
                <td className="num">{r.needsReviewCount}</td>
                <td className="num">{r.skipCount}</td>
                <td className="num">{r.alreadyLinkedCount}</td>
                <td>
                  <Link to={`/tenants/${tenantId}/merge-reports/${r.id}`} className="link-master">Open</Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function AuditTab() {
  return (
    <div className="empty">
      Per-tenant audit filter coming with the global Audit page rework. Use the global Audit page for now.
    </div>
  )
}

function Pagination({ offset, limit, total, onOffset }) {
  if (total <= limit) return null
  const page = Math.floor(offset / limit) + 1
  const pages = Math.ceil(total / limit)
  return (
    <div className="pagination">
      <button disabled={offset === 0} onClick={() => onOffset(Math.max(0, offset - limit))}>← prev</button>
      <span className="muted">page {page} / {pages}</span>
      <button disabled={offset + limit >= total} onClick={() => onOffset(offset + limit)}>next →</button>
    </div>
  )
}
