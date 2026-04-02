import { useState, useEffect, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Clock, MessageCircle, DollarSign, MousePointerClick } from 'lucide-react'
import { api } from '../../shared/api/apiClient'
import { FormationRenderer } from '../../shared/replay/FormationRenderer'
import Spinner from '../../shared/ui/Spinner'
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

  if (loading) {
    return <div className="conv-loading"><Spinner /></div>
  }

  if (error) {
    return <div className="conv-error">{error}</div>
  }

  if (!session) {
    return <div className="conv-empty">Conversation not found</div>
  }

  const totalCost = timeline
    .filter(e => e.type === 'trace')
    .reduce((sum, e) => sum + (e.costUsd || 0), 0)

  return (
    <div className="conversation-detail">
      <div className="conv-detail-header">
        <button className="conv-back-btn" onClick={() => navigate('/conversations')}>
          <ArrowLeft size={14} /> Назад
        </button>
        <div className="conv-detail-meta">
          <span><Clock size={13} /> {formatDate(session.startedAt)}</span>
          <span><MessageCircle size={13} /> {timeline.filter(e => e.type === 'trace').length} сообщений</span>
          <span><DollarSign size={13} /> ${totalCost.toFixed(4)}</span>
          {formatDuration(session.startedAt, session.endedAt) && (
            <span>{formatDuration(session.startedAt, session.endedAt)}</span>
          )}
        </div>
      </div>

      <div className="chat-replay">
        <div className="chat-replay-messages">
          {timeline.length === 0 && (
            <div className="conv-empty">Нет сообщений в этом чате</div>
          )}
          {timeline.map((entry, i) => {
            if (entry.type === 'trace') {
              return <TraceMessage key={`t-${i}`} entry={entry} />
            }
            if (entry.type === 'action') {
              return <ActionIndicator key={`a-${i}`} entry={entry} />
            }
            return null
          })}
          <div ref={messagesEndRef} />
        </div>
      </div>
    </div>
  )
}

function TraceMessage({ entry }) {
  const formation = entry.formation
  const time = formatTime(entry.timestamp)

  return (
    <>
      {/* User message */}
      {entry.query && (
        <div className="replay-msg replay-msg-user">
          <div className="replay-bubble-user">{entry.query}</div>
          <div className="replay-msg-time">{time}</div>
        </div>
      )}

      {/* Assistant response */}
      <div className="replay-msg replay-msg-assistant">
        {formation ? (
          <div className="replay-formation replay-container">
            <FormationRenderer formation={formation} />
          </div>
        ) : entry.error ? (
          <div className="replay-bubble-assistant" style={{ color: '#EF4444' }}>
            Ошибка: {entry.error}
          </div>
        ) : (
          <div className="replay-no-formation">Ответ без визуальных данных</div>
        )}
      </div>
    </>
  )
}

function ActionIndicator({ entry }) {
  const actionLabels = {
    user_click: 'Раскрыл карточку',
    user_back: 'Вернулся назад',
    user_like: 'Добавил в избранное',
    user_cart: 'Добавил в корзину',
    user_expand: 'Раскрыл карточку',
  }
  const label = actionLabels[entry.actorId] || entry.actorId || 'Действие'

  return (
    <div className="replay-action">
      <MousePointerClick size={11} /> {label}
    </div>
  )
}

// --- Helpers ---

function buildTimeline(traces, userActions) {
  const items = []

  for (const trace of traces) {
    const formation = extractFormation(trace)
    items.push({
      type: 'trace',
      timestamp: trace.timestamp,
      query: trace.query,
      formation,
      error: trace.error,
      costUsd: trace.costUsd || 0,
    })
  }

  for (const action of userActions) {
    items.push({
      type: 'action',
      timestamp: action.createdAt,
      actorId: action.actorId,
      deltaType: action.deltaType,
      path: action.path,
    })
  }

  items.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp))
  return items
}

function extractFormation(trace) {
  try {
    const td = typeof trace.traceData === 'string'
      ? JSON.parse(trace.traceData)
      : trace.traceData
    if (!td?.formationResult?.fullJson) return null
    const fj = td.formationResult.fullJson
    return typeof fj === 'string' ? JSON.parse(fj) : fj
  } catch {
    return null
  }
}

function formatDate(ts) {
  if (!ts) return ''
  const d = new Date(ts)
  return d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit' }) +
    ' ' + d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
}

function formatTime(ts) {
  if (!ts) return ''
  return new Date(ts).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function formatDuration(start, end) {
  if (!start) return ''
  const s = new Date(start)
  const e = end ? new Date(end) : new Date()
  const mins = Math.floor((e - s) / 60000)
  if (mins < 1) return '<1 мин'
  if (mins < 60) return `${mins} мин`
  const hrs = Math.floor(mins / 60)
  return `${hrs}ч ${mins % 60}м`
}
