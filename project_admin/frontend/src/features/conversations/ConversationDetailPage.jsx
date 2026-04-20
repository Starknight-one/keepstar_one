import { useState, useEffect, useRef, useMemo } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, MousePointerClick, ExternalLink } from 'lucide-react'
import { api } from '../../shared/api/apiClient'
import { FormationRenderer } from '../../shared/replay/FormationRenderer'
import Spinner from '../../shared/ui/Spinner'
import Button from '../../shared/ui/Button'
import SessionInfoCard from './SessionInfoCard'
import WidgetThumb from './WidgetThumb'
import './conversations.css'

export default function ConversationDetailPage() {
  const { sessionId } = useParams()
  const navigate = useNavigate()
  const [session, setSession] = useState(null)
  const [timeline, setTimeline] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const messagesEndRef = useRef(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    api.get(`/sessions/${sessionId}`)
      .then(data => {
        setSession(data.session)
        setTimeline(buildTimeline(data.traces || [], data.userActions || []))
      })
      .catch(err => setError(err.message || 'Failed to load conversation'))
      .finally(() => setLoading(false))
  }, [sessionId])

  const traceCount = useMemo(() => timeline.filter(e => e.type === 'trace').length, [timeline])
  const totalCost = useMemo(
    () => timeline.filter(e => e.type === 'trace').reduce((s, e) => s + (e.costUsd || 0), 0),
    [timeline]
  )
  const widgets = useMemo(
    () => timeline
      .filter(e => e.type === 'trace' && e.formation)
      .map((e, i) => ({
        idx: i + 1,
        title: e.formation?.preset || e.formation?.mode || `Widget ${i + 1}`,
        sub: e.query ? `from "${e.query.slice(0, 24)}${e.query.length > 24 ? '…' : ''}"` : '',
      })),
    [timeline]
  )
  const lastFormation = useMemo(() => {
    const traces = timeline.filter(e => e.type === 'trace' && e.formation)
    return traces[traces.length - 1]?.formation || null
  }, [timeline])

  const sessionInfo = {
    preset: lastFormation?.preset,
    layout: lastFormation?.mode || lastFormation?.layout,
    itemsShown: lastFormation?.widgets?.length,
    ops: traceCount,
    cartShown: !!session?.cartItemCount,
  }

  if (loading) return <div className="conv-loading"><Spinner /></div>
  if (error) return <div className="conv-error">{error}</div>
  if (!session) return <div className="conv-empty">Conversation not found</div>

  const status = session.endedAt ? 'completed' : 'online'
  const statusCls = status === 'online' ? 'status-live' : 'status-done'

  return (
    <div className="conversation-detail">
      <div className="conv-detail-header">
        <div className="conv-detail-left">
          <button className="conv-back-btn" onClick={() => navigate('/conversations')}>
            <ArrowLeft size={14} /> All chats
          </button>
          <div className="conv-detail-user">
            <div className="conv-avatar">{(session.userIdentifier || session.id || '?').slice(0, 2).toUpperCase()}</div>
            <div>
              <div className="conv-detail-user-name">{session.userIdentifier || `Session #${session.id?.slice(0, 8)}`}</div>
              <div className="conv-detail-user-sub">{formatDate(session.startedAt)}</div>
            </div>
          </div>
        </div>
        <div className="conv-detail-meta">
          <div className="conv-detail-meta-item">
            <span className="conv-detail-meta-label">Cost</span>
            <span className="conv-detail-meta-value">${totalCost.toFixed(4)}</span>
          </div>
          <div className="conv-detail-meta-item">
            <span className="conv-detail-meta-label">Active</span>
            <span className="conv-detail-meta-value">{formatDuration(session.startedAt, session.endedAt)}</span>
          </div>
          <div className="conv-detail-meta-item">
            <span className="conv-detail-meta-label">Status</span>
            <span className={`status-badge ${statusCls}`}>{status === 'online' ? 'Live' : 'Done'}</span>
          </div>
        </div>
      </div>

      <div className="conv-detail-body">
        <div className="chat-replay">
          <div className="chat-replay-messages">
            {timeline.length === 0 && (
              <div className="conv-empty">No messages in this chat</div>
            )}
            {timeline.map((entry, i) => {
              if (entry.type === 'trace') return <TraceMessage key={`t-${i}`} entry={entry} />
              if (entry.type === 'action') return <ActionIndicator key={`a-${i}`} entry={entry} />
              return null
            })}
            <div ref={messagesEndRef} />
          </div>
          <div className="chat-replay-composer">
            <div className="chat-replay-composer-input">Read-only — composer disabled in replay</div>
          </div>
        </div>

        <aside className="conv-detail-rail">
          <div className="card">
            <div className="card-title">Widgets sent</div>
            {widgets.length === 0 ? (
              <div className="widget-thumb-sub">No widgets in this session.</div>
            ) : (
              <div className="widget-thumb-list">
                {widgets.map(w => (
                  <WidgetThumb key={w.idx} title={w.title} sub={w.sub} />
                ))}
              </div>
            )}
          </div>
          <SessionInfoCard info={sessionInfo} />
          <Button variant="primary" pill onClick={() => navigate('/canvas')}>
            <ExternalLink size={14} /> Open in Canvas
          </Button>
        </aside>
      </div>
    </div>
  )
}

function TraceMessage({ entry }) {
  const formation = entry.formation
  const time = formatTime(entry.timestamp)
  return (
    <>
      {entry.query && (
        <div className="replay-msg replay-msg-user">
          <div className="replay-bubble-user">{entry.query}</div>
          <div className="replay-msg-time">{time}</div>
        </div>
      )}
      <div className="replay-msg replay-msg-assistant">
        {formation ? (
          <div className="replay-formation"><FormationRenderer formation={formation} /></div>
        ) : entry.error ? (
          <div className="replay-bubble-assistant" style={{ color: 'var(--danger)' }}>Error: {entry.error}</div>
        ) : (
          <div className="replay-no-formation">Response without visual data</div>
        )}
      </div>
    </>
  )
}

function ActionIndicator({ entry }) {
  const labels = {
    user_click: 'Opened card',
    user_back: 'Went back',
    user_like: 'Liked',
    user_cart: 'Added to cart',
    user_expand: 'Expanded card',
  }
  const label = labels[entry.actorId] || entry.actorId || 'Action'
  return <div className="replay-action"><MousePointerClick size={11} /> {label}</div>
}

function buildTimeline(traces, userActions) {
  const items = []
  for (const t of traces) items.push({
    type: 'trace', timestamp: t.timestamp, query: t.query,
    formation: extractFormation(t), error: t.error, costUsd: t.costUsd || 0,
  })
  for (const a of userActions) items.push({
    type: 'action', timestamp: a.createdAt, actorId: a.actorId, deltaType: a.deltaType, path: a.path,
  })
  items.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp))
  return items
}

function extractFormation(trace) {
  try {
    const td = typeof trace.traceData === 'string' ? JSON.parse(trace.traceData) : trace.traceData
    if (!td?.formationResult?.fullJson) return null
    const fj = td.formationResult.fullJson
    return typeof fj === 'string' ? JSON.parse(fj) : fj
  } catch { return null }
}

function formatDate(ts) {
  if (!ts) return ''
  const d = new Date(ts)
  return d.toLocaleDateString('en-US', { day: '2-digit', month: 'short', year: 'numeric' }) +
    ' · ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false })
}
function formatTime(ts) {
  if (!ts) return ''
  return new Date(ts).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false })
}
function formatDuration(start, end) {
  if (!start) return '—'
  const s = new Date(start)
  const e = end ? new Date(end) : new Date()
  const mins = Math.floor((e - s) / 60000)
  if (mins < 1) return '<1m'
  if (mins < 60) return `${mins}m`
  const hrs = Math.floor(mins / 60)
  return `${hrs}h ${mins % 60}m`
}
