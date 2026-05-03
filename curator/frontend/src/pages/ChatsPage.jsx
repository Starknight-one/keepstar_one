import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { chatsApi } from '../api.js'

// V5 chat sessions across all tenants. Filter by tenant slug, status
// (active = activity within 30 min, closed otherwise), and free-text
// search (matches session id / tenant slug / latest user query).

const PAGE_SIZE = 50

export default function ChatsPage() {
  const [sessions, setSessions] = useState([])
  const [total, setTotal] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [filters, setFilters] = useState({ tenant: '', status: '', q: '' })
  const [offset, setOffset] = useState(0)

  async function load(nextOffset = 0, append = false) {
    setLoading(true)
    setError('')
    try {
      const data = await chatsApi.list({ ...filters, limit: PAGE_SIZE, offset: nextOffset })
      setTotal(data.total || 0)
      setHasMore(!!data.hasMore)
      setOffset(nextOffset)
      const next = data.sessions || []
      setSessions(append ? [...sessions, ...next] : next)
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }

  // Reset offset on filter change.
  useEffect(() => {
    load(0, false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.tenant, filters.status, filters.q])

  return (
    <div className="page">
      <h1>Chat sessions</h1>
      <p className="subtitle">
        Cross-tenant V5 chat trace inspection. Read-only.
      </p>

      <div className="chats-filters">
        <input
          type="text"
          placeholder="Tenant slug or id"
          value={filters.tenant}
          onChange={(e) => setFilters({ ...filters, tenant: e.target.value })}
        />
        <select
          value={filters.status}
          onChange={(e) => setFilters({ ...filters, status: e.target.value })}
        >
          <option value="">All</option>
          <option value="active">Active (last 30 min)</option>
          <option value="closed">Closed</option>
        </select>
        <input
          type="text"
          placeholder="Search session id / query"
          value={filters.q}
          onChange={(e) => setFilters({ ...filters, q: e.target.value })}
        />
      </div>

      {error && <div className="error">{error}</div>}

      {loading && sessions.length === 0 ? (
        <div className="loading">Loading…</div>
      ) : sessions.length === 0 ? (
        <p className="empty">No sessions match these filters.</p>
      ) : (
        <>
          <div className="chats-meta">
            Showing {sessions.length} of {total}
          </div>
          <table className="chats">
            <thead>
              <tr>
                <th>Tenant</th>
                <th>Session</th>
                <th>Started</th>
                <th>Last activity</th>
                <th className="num">Turns</th>
                <th className="num">Tokens</th>
                <th className="num">Cost</th>
                <th>Status</th>
                <th>Latest query</th>
              </tr>
            </thead>
            <tbody>
              {sessions.map((s) => (
                <tr key={s.sessionId} className={s.hasErrors ? 'has-errors' : ''}>
                  <td>
                    <span className="tenant-slug">{s.tenantSlug || '—'}</span>
                    {s.tenantName ? <span className="tenant-name"> · {s.tenantName}</span> : null}
                  </td>
                  <td>
                    <Link to={`/chats/${s.sessionId}`}>
                      <code>{s.sessionId.slice(0, 8)}</code>
                    </Link>
                  </td>
                  <td>{new Date(s.createdAt).toLocaleString()}</td>
                  <td>{new Date(s.lastActivityAt).toLocaleString()}</td>
                  <td className="num">{s.turnCount}</td>
                  <td className="num">{formatNumber(s.totalTokens)}</td>
                  <td className="num">{formatCost(s.totalCostUsd)}</td>
                  <td>
                    <span className={`badge badge-${s.status}`}>{s.status}</span>
                    {s.hasErrors && <span className="badge badge-error">errors</span>}
                  </td>
                  <td className="latest-query" title={s.latestQuery}>
                    {(s.latestQuery || '').slice(0, 80)}
                    {(s.latestQuery || '').length > 80 ? '…' : ''}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {hasMore && (
            <div className="load-more">
              <button
                type="button"
                disabled={loading}
                onClick={() => load(offset + PAGE_SIZE, true)}
              >
                {loading ? 'Loading…' : 'Load more'}
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function formatCost(v) {
  if (v == null || v === 0) return '—'
  return `$${Number(v).toFixed(4)}`
}

function formatNumber(v) {
  if (v == null || v === 0) return '—'
  if (v >= 1e6) return (v / 1e6).toFixed(1) + 'M'
  if (v >= 1e3) return (v / 1e3).toFixed(1) + 'k'
  return String(v)
}
