// TenantDetailPage — full rebuild for the 6-step catalog flow (2026-05-15).
//
// Five tabs surface the per-tenant state of the new pipeline:
//   • Catalog     — listings with master-link (existing, kept verbatim)
//   • Inbox       — raw source rows in catalog.inbox_items
//   • Mapping     — current mapping artifact (JSON)
//   • Action Log  — admin.tenant_action_log timeline
//   • Agent Runs  — admin.agent_runs list + detail
//
// Header actions:
//   • Sync now    — calls admin /admin/api/catalog/v2/sync-now/{tenant_id}
//                   via curator proxy (skips the 24h webhook rate-limit).
//
// All data fetched lazily per tab — no upfront work for tabs the operator
// doesn't open.

import { useEffect, useMemo, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, catalogV2Api } from '../api.js'

export default function TenantDetailPage() {
  const { id } = useParams()
  const [tenant, setTenant] = useState(null)
  const [tab, setTab] = useState('catalog')
  const [error, setError] = useState(null)
  const [syncing, setSyncing] = useState(false)
  const [syncMsg, setSyncMsg] = useState('')

  useEffect(() => {
    api.get(`/curator/tenants/${id}`).then(setTenant).catch(e => setError(e.message))
  }, [id])

  async function handleSyncNow() {
    setSyncing(true); setSyncMsg('')
    try {
      const res = await catalogV2Api.syncNow(id)
      const counts = res?.apply || res || {}
      setSyncMsg(`synced: total=${counts.total ?? 0} applied=${counts.applied ?? 0} errors=${counts.errors ?? 0}`)
    } catch (e) {
      setSyncMsg(`failed: ${e.message}`)
    } finally {
      setSyncing(false)
    }
  }

  if (error) return <div className="page"><div className="error">{error}</div></div>
  if (!tenant) return <div className="page"><div className="loading">loading…</div></div>

  return (
    <div className="page">
      <div className="breadcrumb"><Link to="/tenants">Tenants</Link> / {tenant.slug}</div>
      <div className="tenant-header">
        <h1>{tenant.name}</h1>
        <div className="tenant-actions">
          <button className="btn-primary" onClick={handleSyncNow} disabled={syncing}>
            {syncing ? 'syncing…' : 'Sync now'}
          </button>
          {syncMsg && <span className="muted small">{syncMsg}</span>}
        </div>
      </div>
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
        {[
          ['catalog', 'Catalog'],
          ['inbox', 'Inbox'],
          ['mapping', 'Mapping'],
          ['action-log', 'Action Log'],
          ['agent-runs', 'Agent Runs'],
        ].map(([key, label]) => (
          <button key={key} className={`tab ${tab === key ? 'tab-active' : ''}`} onClick={() => setTab(key)}>
            {label}
          </button>
        ))}
      </div>

      {tab === 'catalog'    && <CatalogTab    tenantId={id} />}
      {tab === 'inbox'      && <InboxTab      tenantId={id} />}
      {tab === 'mapping'    && <MappingTab    tenantId={id} />}
      {tab === 'action-log' && <ActionLogTab  tenantId={id} />}
      {tab === 'agent-runs' && <AgentRunsTab  tenantId={id} />}
    </div>
  )
}

function Pill({ label, value }) {
  return <div className="pill"><span className="pill-label">{label}</span><span className="pill-value">{value}</span></div>
}

// =============================================================================
// CATALOG — existing tab, ported verbatim. Listings with master-link badge.
// =============================================================================

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
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>SKU</th>
                <th>Brand</th>
                <th>Price</th>
                <th>Stock</th>
                <th>Master</th>
              </tr>
            </thead>
            <tbody>
              {products.map(p => (
                <tr key={p.id}>
                  <td>{p.name}</td>
                  <td className="mono">{p.sku || '—'}</td>
                  <td>{p.brand || '—'}</td>
                  <td className="mono">{formatPrice(p.priceCents, p.currency)}</td>
                  <td className="mono">{p.stock ?? '—'}</td>
                  <td>{p.hasMasterLink
                    ? <Link className="badge badge-ok" to={`/master/${p.masterId}`}>master</Link>
                    : <span className="badge badge-warn">unlinked</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
      }
      <Pager total={total} limit={limit} offset={offset} setOffset={setOffset} />
    </div>
  )
}

// =============================================================================
// INBOX — raw source rows from catalog.inbox_items
// =============================================================================

function InboxTab({ tenantId }) {
  const [rows, setRows] = useState([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState(null)
  const limit = 50

  useEffect(() => { load() }, [tenantId, offset])
  async function load() {
    setLoading(true)
    try {
      const data = await catalogV2Api.listInbox(tenantId, limit, offset)
      setRows(data.items || [])
      setTotal(data.total || 0)
    } catch {} finally { setLoading(false) }
  }

  async function openRow(row) {
    try {
      const full = await catalogV2Api.getInboxItem(tenantId, row.id)
      setSelected(full)
    } catch (e) {
      setSelected({ error: e.message })
    }
  }

  return (
    <div>
      <div className="toolbar">
        <span className="muted">{total.toLocaleString()} raw rows in inbox</span>
      </div>
      {loading ? <div className="loading">loading…</div> :
        rows.length === 0 ? <div className="empty">inbox is empty — no source data has been pulled for this tenant yet</div> :
          <table className="table">
            <thead>
              <tr>
                <th>Source</th>
                <th>External ID</th>
                <th>Fetched</th>
                <th>Applied</th>
                <th>Preview</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(r => (
                <tr key={r.id} className="clickable" onClick={() => openRow(r)}>
                  <td><span className="badge badge-neutral">{r.source_kind}</span></td>
                  <td className="mono small">{truncate(r.external_id, 40)}</td>
                  <td className="mono small">{formatTime(r.fetched_at)}</td>
                  <td className="mono small">{r.applied_at
                    ? <span className="badge badge-ok">{formatTime(r.applied_at)}</span>
                    : <span className="badge badge-warn">pending</span>}
                  </td>
                  <td className="mono small muted">{previewJson(r.preview)}</td>
                </tr>
              ))}
            </tbody>
          </table>
      }
      <Pager total={total} limit={limit} offset={offset} setOffset={setOffset} />

      {selected && <Modal onClose={() => setSelected(null)} title={`Inbox item · ${selected.external_id || ''}`}>
        {selected.error
          ? <div className="error">{selected.error}</div>
          : <pre className="json-view">{JSON.stringify(selected.raw, null, 2)}</pre>}
      </Modal>}
    </div>
  )
}

// =============================================================================
// MAPPING — current mapping artifact JSON
// =============================================================================

function MappingTab({ tenantId }) {
  const [state, setState] = useState({ loading: true })

  useEffect(() => {
    (async () => {
      try {
        const data = await catalogV2Api.getMapping(tenantId)
        setState({ data })
      } catch (e) {
        setState({ error: e.message })
      }
    })()
  }, [tenantId])

  if (state.loading) return <div className="loading">loading…</div>
  if (state.error) return <div className="error">{state.error}</div>
  if (!state.data?.exists) return <div className="empty">no mapping artifact yet — run discovery to create one (Sync now also cascades to discovery on first install)</div>

  const a = state.data.artifact
  const body = a.body || {}

  return (
    <div>
      <div className="tenant-meta">
        <Pill label="status" value={a.status} />
        <Pill label="version" value={a.artifact_version} />
        <Pill label="vertical" value={body.vertical || '—'} />
        <Pill label="rules" value={(body.field_map || []).length} />
        <Pill label="discovered" value={formatTime(a.discovered_at)} />
        {a.validated_at && <Pill label="validated" value={formatTime(a.validated_at)} />}
      </div>
      {body.notes && <div className="note-block">
        <div className="note-label">agent notes</div>
        <div>{body.notes}</div>
      </div>}
      <h3 className="section-h3">Field mapping</h3>
      {(body.field_map || []).length === 0 ? <div className="empty">no rules</div> :
        <table className="table">
          <thead>
            <tr><th>From (source)</th><th>To (our schema)</th><th>Transform</th><th>Default</th></tr>
          </thead>
          <tbody>
            {body.field_map.map((r, i) => (
              <tr key={i}>
                <td className="mono small">{r.from}</td>
                <td className="mono small">{r.to}</td>
                <td className="mono small muted">{r.transform || '—'}</td>
                <td className="mono small muted">{r.default || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>}

      <h3 className="section-h3">Raw artifact JSON</h3>
      <pre className="json-view">{JSON.stringify(body, null, 2)}</pre>
    </div>
  )
}

// =============================================================================
// ACTION LOG — admin.tenant_action_log timeline
// =============================================================================

function ActionLogTab({ tenantId }) {
  const [rows, setRows] = useState([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const limit = 100

  useEffect(() => { load() }, [tenantId, offset])
  async function load() {
    setLoading(true)
    try {
      const data = await catalogV2Api.listActionLog(tenantId, limit, offset)
      setRows(data.items || [])
      setTotal(data.total || 0)
    } catch {} finally { setLoading(false) }
  }

  return (
    <div>
      <div className="toolbar">
        <span className="muted">{total.toLocaleString()} events</span>
      </div>
      {loading ? <div className="loading">loading…</div> :
        rows.length === 0 ? <div className="empty">no events yet — first action will appear when source data is pulled</div> :
          <table className="table">
            <thead>
              <tr><th>When</th><th>Action</th><th>Status</th><th>Payload</th></tr>
            </thead>
            <tbody>
              {rows.map(r => (
                <tr key={r.id}>
                  <td className="mono small">{formatTime(r.occurred_at)}</td>
                  <td><span className={`badge ${actionBadge(r.action)}`}>{r.action}</span></td>
                  <td><span className={`badge ${statusBadge(r.status)}`}>{r.status}</span></td>
                  <td className="mono small muted">{previewJson(r.payload)}</td>
                </tr>
              ))}
            </tbody>
          </table>
      }
      <Pager total={total} limit={limit} offset={offset} setOffset={setOffset} />
    </div>
  )
}

// =============================================================================
// AGENT RUNS — admin.agent_runs list + tool-call detail
// =============================================================================

function AgentRunsTab({ tenantId }) {
  const [rows, setRows] = useState([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState(null)
  const limit = 50

  useEffect(() => { load() }, [tenantId, offset])
  async function load() {
    setLoading(true)
    try {
      const data = await catalogV2Api.listAgentRuns(tenantId, limit, offset)
      setRows(data.items || [])
      setTotal(data.total || 0)
    } catch {} finally { setLoading(false) }
  }

  async function openRun(row) {
    try {
      const detail = await catalogV2Api.getAgentRun(tenantId, row.id)
      setSelected(detail)
    } catch (e) {
      setSelected({ error: e.message })
    }
  }

  return (
    <div>
      <div className="toolbar">
        <span className="muted">{total.toLocaleString()} discovery runs</span>
      </div>
      {loading ? <div className="loading">loading…</div> :
        rows.length === 0 ? <div className="empty">no discovery runs yet</div> :
          <table className="table">
            <thead>
              <tr>
                <th>Started</th>
                <th>Trigger</th>
                <th>Status</th>
                <th>Duration</th>
                <th>Tokens (in/out)</th>
                <th>Cost</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(r => (
                <tr key={r.id} className="clickable" onClick={() => openRun(r)}>
                  <td className="mono small">{formatTime(r.started_at)}</td>
                  <td><span className="badge badge-neutral">{r.trigger}</span></td>
                  <td><span className={`badge ${runStatusBadge(r.status)}`}>{r.status}</span></td>
                  <td className="mono small">{durationOf(r.started_at, r.finished_at)}</td>
                  <td className="mono small">{r.tokens_in.toLocaleString()} / {r.tokens_out.toLocaleString()}</td>
                  <td className="mono small">${r.cost_usd.toFixed(4)}</td>
                </tr>
              ))}
            </tbody>
          </table>
      }
      <Pager total={total} limit={limit} offset={offset} setOffset={setOffset} />

      {selected && <Modal onClose={() => setSelected(null)} title="Agent run detail">
        {selected.error ? <div className="error">{selected.error}</div> : <AgentRunDetail run={selected} />}
      </Modal>}
    </div>
  )
}

function AgentRunDetail({ run }) {
  const calls = useMemo(() => Array.isArray(run.tools_called) ? run.tools_called : [], [run.tools_called])
  return (
    <div>
      <div className="tenant-meta">
        <Pill label="trigger" value={run.trigger} />
        <Pill label="status" value={run.status} />
        <Pill label="tools" value={calls.length} />
        <Pill label="tokens" value={`${run.tokens_in} / ${run.tokens_out}`} />
        <Pill label="cost" value={`$${run.cost_usd.toFixed(4)}`} />
        <Pill label="duration" value={durationOf(run.started_at, run.finished_at)} />
      </div>
      {run.trigger_payload && run.trigger_payload !== '{}' && (
        <div className="note-block">
          <div className="note-label">trigger payload</div>
          <pre className="json-view inline">{JSON.stringify(typeof run.trigger_payload === 'string' ? JSON.parse(run.trigger_payload) : run.trigger_payload, null, 2)}</pre>
        </div>
      )}
      <h3 className="section-h3">Tool waterfall ({calls.length} calls)</h3>
      {calls.length === 0 ? <div className="empty">no tool calls recorded</div> :
        <table className="table">
          <thead>
            <tr><th>#</th><th>Tool</th><th>Args</th><th>Result preview</th><th>Duration</th></tr>
          </thead>
          <tbody>
            {calls.map((c, i) => (
              <tr key={i} className={c.error ? 'row-error' : ''}>
                <td className="mono">{i + 1}</td>
                <td><span className="badge badge-neutral">{c.name}</span></td>
                <td className="mono small">{previewJson(c.args)}</td>
                <td className="mono small muted">{c.error ? <span className="error-text">{c.error}</span> : previewJson(c.result_preview)}</td>
                <td className="mono small">{c.duration_ms}ms</td>
              </tr>
            ))}
          </tbody>
        </table>}
    </div>
  )
}

// =============================================================================
// Shared bits
// =============================================================================

function Pager({ total, limit, offset, setOffset }) {
  if (total <= limit) return null
  const page = Math.floor(offset / limit) + 1
  const pages = Math.ceil(total / limit)
  return (
    <div className="pager">
      <button disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - limit))}>prev</button>
      <span className="muted">page {page} / {pages}</span>
      <button disabled={offset + limit >= total} onClick={() => setOffset(offset + limit)}>next</button>
    </div>
  )
}

function Modal({ title, children, onClose }) {
  // Close on Esc
  useEffect(() => {
    const onKey = (e) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <div className="modal-title">{title}</div>
          <button className="modal-close" onClick={onClose}>×</button>
        </div>
        <div className="modal-body">{children}</div>
      </div>
    </div>
  )
}

// =============================================================================
// formatting helpers
// =============================================================================

function formatTime(ts) {
  if (!ts) return '—'
  const d = new Date(ts)
  if (isNaN(d.getTime())) return ts
  return d.toLocaleString('en-GB', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
}

function formatPrice(cents, currency) {
  if (cents == null) return '—'
  const amount = (cents / 100).toFixed(2)
  return `${amount} ${currency || ''}`.trim()
}

function durationOf(start, end) {
  if (!start) return '—'
  if (!end) return 'running…'
  const ms = new Date(end).getTime() - new Date(start).getTime()
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60_000)}m${Math.floor((ms % 60_000) / 1000)}s`
}

function previewJson(v) {
  if (v == null) return '—'
  if (typeof v === 'string') {
    if (v.length > 120) return v.slice(0, 117) + '…'
    return v
  }
  try {
    const s = JSON.stringify(v)
    return s.length > 120 ? s.slice(0, 117) + '…' : s
  } catch {
    return String(v)
  }
}

function truncate(s, n) { return s && s.length > n ? s.slice(0, n - 1) + '…' : s }

function actionBadge(action) {
  switch (action) {
    case 'inbox_pull':
    case 'manual_sync':
      return 'badge-neutral'
    case 'discovery_start':
    case 'discovery_done':
      return 'badge-info'
    case 'apply':
      return 'badge-ok'
    case 'webhook_received':
      return 'badge-info'
    case 'mapping_miss':
    case 'disconnect':
      return 'badge-warn'
    default:
      return 'badge-neutral'
  }
}

function statusBadge(status) {
  if (status === 'ok')      return 'badge-ok'
  if (status === 'warning') return 'badge-warn'
  if (status === 'error')   return 'badge-err'
  return 'badge-neutral'
}

function runStatusBadge(status) {
  if (status === 'success')         return 'badge-ok'
  if (status === 'running')         return 'badge-info'
  if (status === 'budget_exhausted') return 'badge-warn'
  return 'badge-err'
}
