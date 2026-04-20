import { useState, useEffect, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search, Download } from 'lucide-react'
import { api } from '../../shared/api/apiClient'
import Table from '../../shared/ui/Table'
import Pagination from '../../shared/ui/Pagination'
import Spinner from '../../shared/ui/Spinner'
import Button from '../../shared/ui/Button'
import ChipGroup from '../../shared/ui/ChipGroup'
import './conversations.css'

const LIMIT = 30

const STATUS_LABELS = {
  online: 'Online',
  idle: 'Idle',
  completed: 'Completed',
}

function deriveStatus(conv) {
  const last = conv.lastActivityAt || conv.endedAt || conv.startedAt
  if (!last) return 'completed'
  if (conv.endedAt) return 'completed'
  const ageMin = (Date.now() - new Date(last).getTime()) / 60000
  if (ageMin <= 2) return 'online'
  if (ageMin <= 30) return 'idle'
  return 'completed'
}

export default function ConversationsPage() {
  const [conversations, setConversations] = useState([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('all')
  const [query, setQuery] = useState('')
  const navigate = useNavigate()

  useEffect(() => {
    setLoading(true)
    api.get(`/conversations?limit=${LIMIT}&offset=${offset}`)
      .then(data => {
        setConversations(data.conversations || [])
        setTotal(data.total || 0)
      })
      .catch(() => setConversations([]))
      .finally(() => setLoading(false))
  }, [offset])

  const enriched = useMemo(
    () => conversations.map(c => ({ ...c, _status: deriveStatus(c) })),
    [conversations]
  )

  const filtered = useMemo(() => {
    let rows = enriched
    if (filter !== 'all') rows = rows.filter(r => r._status === filter)
    if (query.trim()) {
      const q = query.toLowerCase()
      rows = rows.filter(r =>
        (r.firstQuery || '').toLowerCase().includes(q) ||
        (r.userIdentifier || r.id || '').toLowerCase().includes(q)
      )
    }
    return rows
  }, [enriched, filter, query])

  const counts = useMemo(() => {
    const c = { all: enriched.length, online: 0, idle: 0, completed: 0 }
    for (const r of enriched) c[r._status] += 1
    return c
  }, [enriched])

  const grouped = useMemo(() => {
    const groups = { online: [], idle: [], completed: [] }
    for (const r of filtered) groups[r._status].push(r)
    return groups
  }, [filtered])

  const columns = [
    {
      key: 'user',
      label: 'User',
      render: (row) => (
        <div className="conv-user-cell">
          <div className="conv-avatar">{userInitials(row)}</div>
          <span className="conv-user-id">{row.userIdentifier || `#${row.id?.slice(0, 8)}`}</span>
        </div>
      ),
    },
    {
      key: 'firstQuery',
      label: 'First message',
      render: (row) => (
        <span className="conv-query-preview" title={row.firstQuery || ''}>
          {row.firstQuery
            ? (row.firstQuery.length > 60 ? row.firstQuery.slice(0, 60) + '…' : row.firstQuery)
            : <span className="conv-empty-msg">(empty)</span>}
        </span>
      ),
    },
    { key: 'startedAt', label: 'Started', width: '140px', render: (r) => formatDate(r.startedAt) },
    { key: 'duration', label: 'Active time', width: '110px', render: (r) => formatDuration(r.startedAt, r.endedAt) },
    { key: 'totalCostUsd', label: 'Cost', width: '90px', render: (r) => formatCost(r.totalCostUsd) },
    {
      key: 'status',
      label: 'Status',
      width: '110px',
      render: (r) => <StatusPill status={r._status} />,
    },
  ]

  if (loading && conversations.length === 0) {
    return <div className="conv-loading"><Spinner /></div>
  }

  return (
    <div className="conversations-page">
      <div className="page-header">
        <div>
          <div className="page-title">Chats</div>
          <div className="page-subtitle">{total} total conversations</div>
        </div>
        <div className="page-toolbar">
          <ChipGroup
            value={filter}
            onChange={setFilter}
            options={[
              { value: 'all', label: 'All', count: counts.all },
              { value: 'online', label: 'Online', count: counts.online },
              { value: 'idle', label: 'Idle', count: counts.idle },
              { value: 'completed', label: 'Completed', count: counts.completed },
            ]}
          />
          <div className="search-input">
            <Search size={14} />
            <input
              placeholder="Search by user or message…"
              value={query}
              onChange={e => setQuery(e.target.value)}
            />
          </div>
          <Button variant="secondary" pill>
            <Download size={14} /> Export
          </Button>
        </div>
      </div>

      <div className="conv-groups">
        {(filter === 'all' ? ['online', 'idle', 'completed'] : [filter]).map(group => (
          grouped[group].length > 0 && (
            <div className="conv-group" key={group}>
              <div className="conv-group-header">
                <span className="conv-group-title">{STATUS_LABELS[group]}</span>
                <span className="conv-group-count">{grouped[group].length}</span>
              </div>
              <Table
                columns={columns}
                data={grouped[group]}
                onRowClick={(row) => navigate(`/conversations/${row.id}`)}
              />
            </div>
          )
        ))}
        {filtered.length === 0 && (
          <div className="conv-empty">No conversations match this filter.</div>
        )}
      </div>

      {total > LIMIT && (
        <Pagination total={total} limit={LIMIT} offset={offset} onChange={setOffset} />
      )}
    </div>
  )
}

function StatusPill({ status }) {
  const cls = status === 'online' ? 'status-live' : status === 'idle' ? 'status-idle' : 'status-done'
  const label = status === 'online' ? 'Live' : status === 'idle' ? 'Idle' : 'Done'
  return <span className={`status-badge ${cls}`}>{label}</span>
}

function userInitials(row) {
  const id = row.userIdentifier || row.id || '?'
  return id.slice(0, 2).toUpperCase()
}

function formatDate(ts) {
  if (!ts) return '—'
  const d = new Date(ts)
  return d.toLocaleDateString('en-US', { day: '2-digit', month: 'short' }) +
    ' · ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false })
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

function formatCost(cost) {
  if (!cost) return '$0.00'
  return `$${cost.toFixed(4)}`
}
