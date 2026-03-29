import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../../shared/api/apiClient.js'
import Table from '../../shared/ui/Table.jsx'
import Pagination from '../../shared/ui/Pagination.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './traces.css'

const LIMIT = 50

function formatDateTime(ts) {
  if (!ts) return '—'
  const d = new Date(ts)
  return d.toLocaleString('ru-RU', {
    day: '2-digit', month: '2-digit', year: '2-digit',
    hour: '2-digit', minute: '2-digit',
  })
}

function formatDuration(startedAt, endedAt, status) {
  if (!startedAt) return '—'
  const start = new Date(startedAt).getTime()
  const end = endedAt ? new Date(endedAt).getTime() : Date.now()
  const diffMs = end - start
  const mins = Math.floor(diffMs / 60000)
  if (mins < 1) return '<1m'
  if (mins < 60) return `${mins}m`
  const hours = Math.floor(mins / 60)
  const remainMins = mins % 60
  if (hours < 24) return `${hours}h ${remainMins}m`
  const days = Math.floor(hours / 24)
  return `${days}d ${hours % 24}h`
}

const sessionColumns = [
  {
    key: 'id',
    label: 'Session',
    width: '90px',
    render: (row) => <code className="session-id">{row.id.substring(0, 8)}</code>,
  },
  {
    key: 'startedAt',
    label: 'Started',
    width: '130px',
    render: (row) => <span className="trace-time">{formatDateTime(row.startedAt)}</span>,
  },
  {
    key: 'endedAt',
    label: 'Ended',
    width: '130px',
    render: (row) => row.endedAt
      ? <span className="trace-time">{formatDateTime(row.endedAt)}</span>
      : <span className="session-status session-status--active">active</span>,
  },
  {
    key: 'duration',
    label: 'Duration',
    width: '80px',
    render: (row) => <span className="trace-duration">{formatDuration(row.startedAt, row.endedAt, row.status)}</span>,
  },
  {
    key: 'totalCostUsd',
    label: 'Cost',
    width: '80px',
    render: (row) => <span className="trace-cost">${row.totalCostUsd?.toFixed(4)}</span>,
  },
  {
    key: 'traceCount',
    label: 'Requests',
    width: '70px',
    render: (row) => <span className="trace-mono">{row.traceCount}</span>,
  },
  {
    key: 'actionCount',
    label: 'Actions',
    width: '70px',
    render: (row) => <span className="trace-mono">{row.actionCount}</span>,
  },
  {
    key: 'totalTokens',
    label: 'Tokens',
    width: '80px',
    render: (row) => <span className="trace-mono">{row.totalTokens?.toLocaleString()}</span>,
  },
  {
    key: 'firstQuery',
    label: 'First Message',
    render: (row) => <span className="trace-query">{row.firstQuery?.substring(0, 60)}{row.firstQuery?.length > 60 ? '...' : ''}</span>,
  },
  {
    key: 'actions',
    label: '',
    width: '60px',
    render: (row, { onKill, killing }) => row.status === 'active'
      ? (
        <button
          className="btn btn-danger btn-sm"
          onClick={(e) => { e.stopPropagation(); onKill(row.id) }}
          disabled={killing === row.id}
        >
          {killing === row.id ? '...' : 'Kill'}
        </button>
      )
      : null,
  },
]

export default function TracesPage() {
  const navigate = useNavigate()
  const [sessions, setSessions] = useState([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const [killing, setKilling] = useState(null)

  const fetchSessions = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams({ limit: LIMIT, offset })
      const data = await api.get(`/sessions?${params}`)
      setSessions(data.sessions || [])
      setTotal(data.total || 0)
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [offset])

  useEffect(() => {
    fetchSessions()
  }, [fetchSessions])

  async function handleKill(sessionId) {
    if (!confirm(`Kill session ${sessionId.substring(0, 8)}...?`)) return
    setKilling(sessionId)
    try {
      await api.post('/sessions/kill', { sessionId })
      setSessions(prev => prev.map(s =>
        s.id === sessionId ? { ...s, status: 'closed', endedAt: new Date().toISOString() } : s
      ))
    } catch { /* ignore */ }
    finally { setKilling(null) }
  }

  return (
    <div>
      <div className="traces-header">
        <h1 className="page-title">Sessions</h1>
        <button className="btn btn-secondary" onClick={fetchSessions} disabled={loading}>
          Refresh
        </button>
      </div>

      {loading ? (
        <div className="center-spinner"><Spinner /></div>
      ) : (
        <>
          <Table
            columns={sessionColumns}
            data={sessions}
            onRowClick={(row) => navigate(`/traces/session/${row.id}`)}
            extraProps={{ onKill: handleKill, killing }}
          />
          <Pagination total={total} limit={LIMIT} offset={offset} onChange={setOffset} />
        </>
      )}
    </div>
  )
}
