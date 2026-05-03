import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { chatsApi } from '../api.js'

// V5 chat detail — session header + per-turn timeline + per-turn
// spans waterfall (lazy-loaded on click). Designed as a debugging
// tool: token / cost / latency exposed directly, full attrs on each
// span available on expand.

export default function ChatDetailPage() {
  const { sessionId } = useParams()
  const [timeline, setTimeline] = useState(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    chatsApi.timeline(sessionId)
      .then(setTimeline)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [sessionId])

  if (loading) return <div className="loading">Loading session…</div>
  if (error) return <div className="page"><div className="error">{error}</div><p><Link to="/chats">← back to sessions</Link></p></div>
  if (!timeline) return null

  const s = timeline.session

  return (
    <div className="page">
      <p><Link to="/chats">← back to sessions</Link></p>
      <h1>Session <code>{s.sessionId.slice(0, 8)}</code></h1>

      <div className="chat-header">
        <div><strong>Tenant:</strong> {s.tenantSlug || '—'}{s.tenantName ? ` · ${s.tenantName}` : ''}</div>
        <div><strong>Started:</strong> {new Date(s.createdAt).toLocaleString()}</div>
        <div><strong>Last activity:</strong> {new Date(s.lastActivityAt).toLocaleString()}</div>
        <div><strong>Status:</strong> <span className={`badge badge-${s.status}`}>{s.status}</span></div>
        <div><strong>Total turns:</strong> {timeline.turns.length}</div>
        <div><strong>Total tokens:</strong> {formatNumber(timeline.totalTokens)}</div>
        <div><strong>Total cost:</strong> {formatCost(timeline.totalCostUsd)}</div>
      </div>

      {timeline.turns.length === 0 ? (
        <p className="empty">No traces persisted for this session yet.</p>
      ) : (
        <table className="chat-turns">
          <thead>
            <tr>
              <th>#</th>
              <th>Time</th>
              <th>Query</th>
              <th className="num">Tokens in</th>
              <th className="num">Tokens out</th>
              <th className="num">Cache read</th>
              <th className="num">Cost</th>
              <th className="num">A1 ms</th>
              <th className="num">A2 ms</th>
              <th className="num">Total ms</th>
              <th>Status</th>
              <th>Spans</th>
            </tr>
          </thead>
          <tbody>
            {timeline.turns.map((t, i) => (
              <TurnRow key={t.requestId} index={i + 1} turn={t} sessionId={sessionId} />
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function TurnRow({ index, turn, sessionId }) {
  const [expanded, setExpanded] = useState(false)
  const [detail, setDetail] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function toggle() {
    if (expanded) {
      setExpanded(false)
      return
    }
    setExpanded(true)
    if (!detail) {
      setLoading(true)
      try {
        const d = await chatsApi.turn(sessionId, turn.requestId)
        setDetail(d)
      } catch (e) {
        setError(e.message)
      } finally {
        setLoading(false)
      }
    }
  }

  return (
    <>
      <tr className={turn.status === 'error' ? 'has-errors' : ''}>
        <td>{index}</td>
        <td>{new Date(turn.createdAt).toLocaleTimeString()}</td>
        <td className="latest-query" title={turn.userQuery}>
          {turn.userQuery.slice(0, 60)}{turn.userQuery.length > 60 ? '…' : ''}
        </td>
        <td className="num">{formatNumber(turn.tokensInput)}</td>
        <td className="num">{formatNumber(turn.tokensOutput)}</td>
        <td className="num">—</td>
        <td className="num">{formatCost(turn.costUsd)}</td>
        <td className="num">{turn.agent1Ms || '—'}</td>
        <td className="num">{turn.agent2Ms || '—'}</td>
        <td className="num">{turn.latencyMs}</td>
        <td><span className={`badge badge-${turn.status}`}>{turn.status}</span></td>
        <td>
          <button type="button" className="link-btn" onClick={toggle}>
            {expanded ? 'hide' : `${turn.spanCount} spans`}
          </button>
        </td>
      </tr>
      {expanded && (
        <tr className="span-row">
          <td colSpan={12}>
            {loading && <div className="loading">Loading spans…</div>}
            {error && <div className="error">{error}</div>}
            {detail && <SpansWaterfall spans={detail.spans} turnError={turn.errorMessage} />}
          </td>
        </tr>
      )}
    </>
  )
}

// SpansWaterfall — text-table «Gantt»: name (indented by depth via
// parent_id chain), start_offset_ms, duration_ms, status, attrs JSON.
// Not a real SVG-Gantt for this iteration; the data is the value, not
// the visualisation.
function SpansWaterfall({ spans, turnError }) {
  if (!spans || spans.length === 0) {
    return <div className="empty">No spans recorded for this turn.</div>
  }
  // Build depth map by walking parent_id chain.
  const byID = new Map()
  for (const s of spans) byID.set(s.id, s)
  function depthOf(s) {
    let d = 0
    let cur = s
    while (cur && cur.parent_id) {
      cur = byID.get(cur.parent_id)
      d += 1
      if (d > 20) break // safety
    }
    return d
  }
  // Find anchor — earliest start_ms across all spans.
  const anchor = Math.min(...spans.map((s) => s.start_ms))

  return (
    <div className="spans-waterfall">
      {turnError && <div className="error">turn error: {turnError}</div>}
      <table className="spans-table">
        <thead>
          <tr>
            <th>Name</th>
            <th className="num">Start (ms)</th>
            <th className="num">Duration (ms)</th>
            <th>Status</th>
            <th>Attrs</th>
          </tr>
        </thead>
        <tbody>
          {spans.map((s) => (
            <tr key={s.id} className={s.status === 'error' ? 'span-error' : ''}>
              <td>
                <span style={{ paddingLeft: `${depthOf(s) * 16}px` }}>
                  {depthOf(s) > 0 ? '↳ ' : ''}
                  <code>{s.name}</code>
                </span>
              </td>
              <td className="num">{s.start_ms - anchor}</td>
              <td className="num">{s.duration_ms}</td>
              <td>{s.status || 'ok'}</td>
              <td className="span-attrs">
                {s.attrs ? <code>{JSON.stringify(s.attrs)}</code> : '—'}
                {s.error ? <div className="error">{s.error}</div> : null}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
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
