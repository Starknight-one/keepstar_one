import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../../shared/api/apiClient.js'
import Table from '../../shared/ui/Table.jsx'
import Pagination from '../../shared/ui/Pagination.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './traces.css'

const LIMIT = 50

function formatTime(ts) {
  const d = new Date(ts)
  return d.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function timeAgo(ts) {
  const diff = Date.now() - new Date(ts).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

function extractAgent1Tool(traceData) {
  try {
    const data = typeof traceData === 'string' ? JSON.parse(traceData) : traceData
    return data?.agent1?.toolName || '—'
  } catch { return '—' }
}

function extractAgent2Tool(traceData) {
  try {
    const data = typeof traceData === 'string' ? JSON.parse(traceData) : traceData
    return data?.agent2?.toolName || '—'
  } catch { return '—' }
}

function extractMode(traceData) {
  try {
    const data = typeof traceData === 'string' ? JSON.parse(traceData) : traceData
    return data?.formationResult?.mode || '—'
  } catch { return '—' }
}

function extractWidgetCount(traceData) {
  try {
    const data = typeof traceData === 'string' ? JSON.parse(traceData) : traceData
    return data?.formationResult?.widgetCount ?? '—'
  } catch { return '—' }
}

const traceColumns = [
  {
    key: 'timestamp',
    label: 'Time',
    width: '140px',
    render: (row) => <span className="trace-time">{formatTime(row.timestamp)}</span>,
  },
  {
    key: 'query',
    label: 'Query',
    render: (row) => <span className="trace-query">{row.query?.substring(0, 80)}{row.query?.length > 80 ? '…' : ''}</span>,
  },
  {
    key: 'agent1Tool',
    label: 'Agent1 Tool',
    width: '130px',
    render: (row) => <span className="trace-tool">{extractAgent1Tool(row.traceData)}</span>,
  },
  {
    key: 'agent2Tool',
    label: 'Agent2 Tool',
    width: '130px',
    render: (row) => <span className="trace-tool">{extractAgent2Tool(row.traceData)}</span>,
  },
  {
    key: 'mode',
    label: 'Mode',
    width: '90px',
    render: (row) => extractMode(row.traceData),
  },
  {
    key: 'widgets',
    label: 'Widgets',
    width: '70px',
    render: (row) => extractWidgetCount(row.traceData),
  },
  {
    key: 'totalMs',
    label: 'Duration',
    width: '80px',
    render: (row) => <span className="trace-duration">{row.totalMs}ms</span>,
  },
  {
    key: 'costUsd',
    label: 'Cost',
    width: '70px',
    render: (row) => <span className="trace-cost">${row.costUsd?.toFixed(4)}</span>,
  },
  {
    key: 'status',
    label: 'Status',
    width: '60px',
    render: (row) => row.error
      ? <span className="trace-status error">ERR</span>
      : <span className="trace-status ok">OK</span>,
  },
]

function SessionsPanel({ onKill }) {
  const [sessions, setSessions] = useState([])
  const [loading, setLoading] = useState(true)
  const [killing, setKilling] = useState(null)

  const fetchSessions = useCallback(async () => {
    setLoading(true)
    try {
      const data = await api.get('/sessions')
      setSessions(data.sessions || [])
    } catch { /* ignore */ }
    finally { setLoading(false) }
  }, [])

  useEffect(() => { fetchSessions() }, [fetchSessions])

  async function handleKill(sessionId) {
    if (!confirm(`Kill session ${sessionId.substring(0, 8)}…?`)) return
    setKilling(sessionId)
    try {
      await api.post('/sessions/kill', { sessionId })
      setSessions(prev => prev.map(s =>
        s.id === sessionId ? { ...s, status: 'closed' } : s
      ))
      onKill?.()
    } catch { /* ignore */ }
    finally { setKilling(null) }
  }

  const active = sessions.filter(s => s.status === 'active')
  const closed = sessions.filter(s => s.status !== 'active')

  if (loading) return <div className="sessions-loading"><Spinner /></div>

  return (
    <div className="sessions-panel">
      <div className="sessions-panel-header">
        <h2 className="sessions-title">Sessions</h2>
        <span className="sessions-count">{active.length} active</span>
        <button className="btn btn-secondary btn-sm" onClick={fetchSessions}>Refresh</button>
      </div>

      {active.length === 0 && <div className="sessions-empty">No active sessions</div>}

      {active.map(s => (
        <div key={s.id} className="session-row">
          <div className="session-info">
            <code className="session-id">{s.id.substring(0, 8)}</code>
            <span className="session-status session-status--active">active</span>
            <span className="session-time">{timeAgo(s.lastActivityAt)}</span>
          </div>
          <button
            className="btn btn-danger btn-sm"
            onClick={() => handleKill(s.id)}
            disabled={killing === s.id}
          >
            {killing === s.id ? '...' : 'Kill'}
          </button>
        </div>
      ))}

      {closed.length > 0 && (
        <details className="sessions-closed-details">
          <summary className="sessions-closed-summary">{closed.length} closed sessions</summary>
          {closed.slice(0, 10).map(s => (
            <div key={s.id} className="session-row session-row--closed">
              <div className="session-info">
                <code className="session-id">{s.id.substring(0, 8)}</code>
                <span className="session-status session-status--closed">closed</span>
                <span className="session-time">{timeAgo(s.lastActivityAt)}</span>
              </div>
            </div>
          ))}
        </details>
      )}
    </div>
  )
}

export default function TracesPage() {
  const navigate = useNavigate()
  const [traces, setTraces] = useState([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)

  const fetchTraces = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams({ limit: LIMIT, offset })
      const data = await api.get(`/traces?${params}`)
      setTraces(data.traces || [])
      setTotal(data.total || 0)
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [offset])

  useEffect(() => {
    fetchTraces()
  }, [fetchTraces])

  return (
    <div>
      <div className="traces-header">
        <h1 className="page-title">Pipeline Traces</h1>
        <button className="btn btn-secondary" onClick={fetchTraces} disabled={loading}>
          Refresh
        </button>
      </div>

      <SessionsPanel onKill={fetchTraces} />

      {loading ? (
        <div className="center-spinner"><Spinner /></div>
      ) : (
        <>
          <Table
            columns={traceColumns}
            data={traces}
            onRowClick={(row) => navigate(`/traces/${row.id}`)}
          />
          <Pagination total={total} limit={LIMIT} offset={offset} onChange={setOffset} />
        </>
      )}
    </div>
  )
}
