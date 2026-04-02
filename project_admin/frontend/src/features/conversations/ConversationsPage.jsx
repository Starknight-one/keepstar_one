import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../../shared/api/apiClient'
import Table from '../../shared/ui/Table'
import Pagination from '../../shared/ui/Pagination'
import Spinner from '../../shared/ui/Spinner'
import './conversations.css'

const LIMIT = 30

export default function ConversationsPage() {
  const [conversations, setConversations] = useState([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
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

  function formatDate(ts) {
    if (!ts) return '—'
    const d = new Date(ts)
    return d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit' }) +
      ' ' + d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
  }

  function formatDuration(start, end) {
    if (!start) return '—'
    const s = new Date(start)
    const e = end ? new Date(end) : new Date()
    const diffMs = e - s
    const mins = Math.floor(diffMs / 60000)
    if (mins < 1) return '<1 мин'
    if (mins < 60) return `${mins} мин`
    const hrs = Math.floor(mins / 60)
    const remMins = mins % 60
    return `${hrs}ч ${remMins}м`
  }

  function formatCost(cost) {
    if (!cost || cost === 0) return '$0.00'
    return `$${cost.toFixed(4)}`
  }

  function renderOutcome(conv) {
    if (conv.cartItemCount > 0) {
      return <span className="conv-badge conv-badge-cart">Корзина ({conv.cartItemCount})</span>
    }
    if (conv.likedCount > 0) {
      return <span className="conv-badge conv-badge-liked">Избранное ({conv.likedCount})</span>
    }
    return <span className="conv-badge conv-badge-browse">Просмотр</span>
  }

  const columns = [
    {
      key: 'firstQuery',
      label: 'Сообщение',
      render: (row) => {
        const val = row.firstQuery
        return (
          <span className="conv-query-preview" title={val || ''}>
            {val ? (val.length > 60 ? val.slice(0, 60) + '...' : val) : '(пустой запрос)'}
          </span>
        )
      },
    },
    {
      key: 'startedAt',
      label: 'Дата',
      width: '130px',
      render: (row) => formatDate(row.startedAt),
    },
    {
      key: 'duration',
      label: 'Длительность',
      width: '100px',
      render: (row) => formatDuration(row.startedAt, row.endedAt),
    },
    {
      key: 'messageCount',
      label: 'Сообщений',
      width: '90px',
      render: (row) => row.messageCount || 0,
    },
    {
      key: 'totalCostUsd',
      label: 'Стоимость',
      width: '90px',
      render: (row) => formatCost(row.totalCostUsd),
    },
    {
      key: 'outcome',
      label: 'Результат',
      width: '130px',
      render: (row) => renderOutcome(row),
    },
  ]

  if (loading && conversations.length === 0) {
    return <div className="conv-loading"><Spinner /></div>
  }

  return (
    <div className="conversations-page">
      <div className="conv-header">
        <h1>Conversations</h1>
        <span className="conv-total">{total} чатов</span>
      </div>

      <Table
        columns={columns}
        data={conversations}
        onRowClick={(row) => navigate(`/conversations/${row.id}`)}
      />

      {total > LIMIT && (
        <Pagination
          total={total}
          limit={LIMIT}
          offset={offset}
          onChange={setOffset}
        />
      )}
    </div>
  )
}
