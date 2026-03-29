import { useState, useEffect, useMemo } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { api } from '../../shared/api/apiClient.js'
import Spinner from '../../shared/ui/Spinner.jsx'
import TraceInline from './TraceInline.jsx'
import { MetricCard, Tag } from './TraceComponents.jsx'
import './traces.css'

function formatDateTime(ts) {
  if (!ts) return '—'
  return new Date(ts).toLocaleString('ru-RU', {
    day: '2-digit', month: '2-digit', year: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

function formatDuration(startedAt, endedAt) {
  if (!startedAt) return '—'
  const start = new Date(startedAt).getTime()
  const end = endedAt ? new Date(endedAt).getTime() : Date.now()
  const mins = Math.floor((end - start) / 60000)
  if (mins < 1) return '<1m'
  if (mins < 60) return `${mins}m`
  const hours = Math.floor(mins / 60)
  return `${hours}h ${mins % 60}m`
}

function actionLabel(action) {
  const actorMap = {
    user_expand: 'Expand',
    user_back: 'Back',
    user_action: 'Action',
  }
  return actorMap[action.actorId] || action.actorId
}

function actionDetail(action) {
  try {
    const a = typeof action.action === 'string' ? JSON.parse(action.action) : action.action
    if (a?.type) {
      const parts = [a.type]
      if (a.params?.entityId) parts.push(a.params.entityId.substring(0, 12))
      if (a.params?.entityType) parts.push(`(${a.params.entityType})`)
      return parts.join(' ')
    }
  } catch { /* ignore */ }
  return ''
}

function buildTimeline(traces, userActions) {
  const items = []

  for (const trace of (traces || [])) {
    const td = typeof trace.traceData === 'string' ? JSON.parse(trace.traceData) : trace.traceData
    items.push({
      type: 'trace',
      timestamp: new Date(trace.timestamp).getTime(),
      trace,
      traceData: td,
    })
  }

  for (const action of (userActions || [])) {
    items.push({
      type: 'action',
      timestamp: new Date(action.createdAt).getTime(),
      action,
    })
  }

  items.sort((a, b) => a.timestamp - b.timestamp)
  return items
}

function TraceResponseSummary({ trace, td }) {
  const formation = td?.formationResult
  const agent1 = td?.agent1
  const agent2 = td?.agent2

  return (
    <div className="timeline-response">
      <div className="timeline-response-metrics">
        {formation?.mode && <Tag color="green">{formation.mode}</Tag>}
        {formation?.widgetCount != null && <span className="trace-mono">{formation.widgetCount} widgets</span>}
        {agent1?.toolName && <Tag color="blue">{agent1.toolName}</Tag>}
        {agent2?.toolName && <Tag color="purple">{agent2.toolName}</Tag>}
        <span className="trace-duration">{trace.totalMs}ms</span>
        <span className="trace-cost">${trace.costUsd?.toFixed(4)}</span>
      </div>
      {trace.error && <div className="timeline-response-error">{trace.error}</div>}
    </div>
  )
}

export default function SessionDetail() {
  const { sessionId } = useParams()
  const navigate = useNavigate()
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [killing, setKilling] = useState(false)

  useEffect(() => {
    api.get(`/sessions/${sessionId}`)
      .then(setData)
      .catch(() => navigate('/traces'))
      .finally(() => setLoading(false))
  }, [sessionId, navigate])

  const timeline = useMemo(() => {
    if (!data) return []
    return buildTimeline(data.traces, data.userActions)
  }, [data])

  if (loading) return <div className="center-spinner"><Spinner /></div>
  if (!data) return null

  const { session, traces } = data
  const totalCost = (traces || []).reduce((sum, t) => sum + (t.costUsd || 0), 0)
  const totalTokens = (traces || []).reduce((sum, t) => {
    const td = typeof t.traceData === 'string' ? JSON.parse(t.traceData) : t.traceData
    return sum +
      (td?.agent1?.inputTokens || 0) + (td?.agent1?.outputTokens || 0) +
      (td?.agent2?.inputTokens || 0) + (td?.agent2?.outputTokens || 0)
  }, 0)

  async function handleKill() {
    if (!confirm(`Kill session ${sessionId.substring(0, 8)}...?`)) return
    setKilling(true)
    try {
      await api.post('/sessions/kill', { sessionId })
      setData(prev => ({ ...prev, session: { ...prev.session, status: 'closed' } }))
    } catch { /* ignore */ }
    finally { setKilling(false) }
  }

  return (
    <div className="session-detail">
      <button className="btn btn-secondary trace-back" onClick={() => navigate('/traces')}>
        ← Sessions
      </button>

      {/* Session header */}
      <div className="session-header">
        <div className="session-header-top">
          <h2 className="session-header-title">
            Session <code>{session.id.substring(0, 8)}</code>
          </h2>
          <span className={`session-status session-status--${session.status === 'active' ? 'active' : 'closed'}`}>
            {session.status}
          </span>
          {session.status === 'active' && (
            <button className="btn btn-danger btn-sm" onClick={handleKill} disabled={killing}>
              {killing ? '...' : 'Kill'}
            </button>
          )}
        </div>
        <div className="session-header-metrics">
          <MetricCard label="Started" value={formatDateTime(session.startedAt)} />
          <MetricCard label="Duration" value={formatDuration(session.startedAt, session.endedAt)} />
          <MetricCard label="Cost" value={`$${totalCost.toFixed(4)}`} />
          <MetricCard label="Requests" value={traces?.length || 0} />
          <MetricCard label="Actions" value={data.userActions?.length || 0} />
          <MetricCard label="Tokens" value={totalTokens.toLocaleString()} />
        </div>
      </div>

      {/* Timeline */}
      <div className="session-timeline">
        {timeline.map((item, i) => {
          if (item.type === 'trace') {
            return (
              <div key={`trace-${item.trace.id}`} className="timeline-entry">
                <div className="timeline-step-num">{i + 1}</div>
                <div className="timeline-content">
                  <div className="timeline-user-msg">
                    {item.trace.query}
                  </div>
                  <TraceResponseSummary trace={item.trace} td={item.traceData} />
                  <TraceInline trace={item.trace} />
                </div>
              </div>
            )
          }

          if (item.type === 'action') {
            const a = item.action
            return (
              <div key={`action-${a.id}`} className="timeline-entry timeline-entry--action">
                <div className="timeline-step-num timeline-step-num--action">{i + 1}</div>
                <div className="timeline-content">
                  <div className="timeline-action">
                    <Tag color={a.actorId === 'user_expand' ? 'blue' : a.actorId === 'user_back' ? 'gray' : 'purple'}>
                      {actionLabel(a)}
                    </Tag>
                    <span className="timeline-action-detail">{actionDetail(a)}</span>
                    <span className="timeline-action-meta">
                      <code>{a.deltaType}</code> → <code>{a.path}</code>
                    </span>
                    <span className="trace-time">{formatDateTime(a.createdAt)}</span>
                  </div>
                </div>
              </div>
            )
          }

          return null
        })}

        {timeline.length === 0 && (
          <div className="timeline-empty">No interactions in this session</div>
        )}
      </div>
    </div>
  )
}
