import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../../shared/api/apiClient.js'
import Button from '../../shared/ui/Button.jsx'
import Badge from '../../shared/ui/Badge.jsx'
import Table from '../../shared/ui/Table.jsx'
import './integrations.css'

const SOURCES = [
  {
    key: 'shopify',
    title: 'Shopify',
    description: 'OAuth install — full catalog sync with webhook updates.',
    cta: 'Connect Shopify',
    href: '/integrations/shopify',
  },
  {
    key: 'csv',
    title: 'CSV upload',
    description: 'Drop a CSV — AI proposes the column mapping, you confirm.',
    cta: 'Upload CSV',
    href: '/integrations/csv',
  },
  {
    key: 'google_sheets',
    title: 'Google Sheets',
    description: 'Coming soon — connect a sheet for scheduled sync.',
    cta: 'Coming soon',
    href: null,
  },
]

const columns = [
  {
    key: 'kind',
    label: 'Source',
    render: (row) => <span style={{ textTransform: 'capitalize' }}>{row.kind.replace('_', ' ')}</span>,
  },
  { key: 'displayName', label: 'Name' },
  { key: 'status', label: 'Status', render: (row) => <Badge status={row.status} /> },
  {
    key: 'lastSyncAt',
    label: 'Last sync',
    render: (row) => (row.lastSyncAt ? new Date(row.lastSyncAt).toLocaleString() : '—'),
  },
]

export default function IntegrationsPage() {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [params, setParams] = useSearchParams()
  const connectedBanner = params.get('connected')

  useEffect(() => {
    let cancelled = false
    api.get('/integrations')
      .then((data) => { if (!cancelled) setItems(Array.isArray(data) ? data : []) })
      .catch((err) => { if (!cancelled) setError(err.message) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [])

  async function handleDisconnect(id) {
    if (!confirm('Disconnect this integration? Synced products stay in your catalog.')) return
    try {
      await api.delete(`/integrations/${id}`)
      setItems((prev) => prev.filter((it) => it.id !== id))
    } catch (err) {
      setError(err.message)
    }
  }

  async function handleWipe(id, kind) {
    const first = confirm(
      `WIPE everything synced from this ${kind} integration?\n\n` +
      'This deletes ALL synced listings, raw imports, merge reports, ' +
      'tenant categories, and the discovered schema for this tenant. ' +
      'Master products are preserved (they may be linked to other tenants).\n\n' +
      'You\'ll need to re-install / re-connect to bring data back. NOT REVERSIBLE.'
    )
    if (!first) return
    const typed = prompt('Type WIPE to confirm:')
    if (typed !== 'WIPE') return
    try {
      const res = await api.post(`/integrations/${id}/wipe`)
      setItems((prev) => prev.filter((it) => it.id !== id))
      alert(
        `Wiped:\n` +
        `· ${res.listings} listings\n` +
        `· ${res.rawImports} raw imports\n` +
        `· ${res.mergeReports} merge reports\n` +
        `· ${res.categories} categories\n` +
        `· schema artifact: ${res.schemaArtifact ? 'removed' : 'none'}`
      )
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <div>
      <h1 className="page-title">Integrations</h1>
      <p className="text-secondary">Connect a catalog source — Keepstar syncs products for the chat widget automatically.</p>

      {connectedBanner && (
        <div className="integration-banner">
          Connected {connectedBanner}. Initial sync is running — products become searchable in ~2 minutes.
          <button
            style={{ marginLeft: 12, background: 'none', border: 'none', color: '#065f46', cursor: 'pointer', textDecoration: 'underline' }}
            onClick={() => setParams({})}
          >dismiss</button>
        </div>
      )}

      <div className="integrations-grid">
        {SOURCES.map((src) => (
          <div key={src.key} className="integration-source-card">
            <h3>{src.title}</h3>
            <p>{src.description}</p>
            {src.href ? (
              <Link to={src.href}><Button style={{ width: '100%' }}>{src.cta}</Button></Link>
            ) : (
              <Button disabled style={{ width: '100%' }}>{src.cta}</Button>
            )}
          </div>
        ))}
      </div>

      <div className="integration-connected">
        <h2 className="section-title">Connected sources</h2>
        {loading ? (
          <p className="text-secondary">Loading…</p>
        ) : error ? (
          <div className="auth-error">{error}</div>
        ) : items.length === 0 ? (
          <p className="text-secondary">No integrations yet — pick a source above.</p>
        ) : (
          <Table
            columns={[
              ...columns,
              {
                key: '_actions',
                label: '',
                render: (row) => (
                  <div style={{ display: 'flex', gap: 12, justifyContent: 'flex-end' }}>
                    <button
                      onClick={() => handleDisconnect(row.id)}
                      style={{ background: 'none', border: 'none', color: '#6b7280', cursor: 'pointer', fontSize: 13 }}
                      title="Drop the OAuth row only — synced data stays."
                    >Disconnect</button>
                    <button
                      onClick={() => handleWipe(row.id, row.kind)}
                      style={{ background: 'none', border: 'none', color: '#dc2626', cursor: 'pointer', fontSize: 13, fontWeight: 600 }}
                      title="Drop OAuth row AND all synced data. Not reversible."
                    >Wipe & disconnect</button>
                  </div>
                ),
              },
            ]}
            data={items}
          />
        )}
      </div>
    </div>
  )
}
