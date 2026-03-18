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

const columns = [
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

      {loading ? (
        <div className="center-spinner"><Spinner /></div>
      ) : (
        <>
          <Table
            columns={columns}
            data={traces}
            onRowClick={(row) => navigate(`/traces/${row.id}`)}
          />
          <Pagination total={total} limit={LIMIT} offset={offset} onChange={setOffset} />
        </>
      )}
    </div>
  )
}
