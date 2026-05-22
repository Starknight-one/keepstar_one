import { useState, useEffect, useCallback } from 'react'
import { AlertCircle, CheckCircle2, XCircle } from 'lucide-react'
import { api } from '../../shared/api/apiClient.js'
import Button from '../../shared/ui/Button.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './catalog.css'
import './schemaDrift.css'

const STATUS_TABS = [
  { value: 'classified', label: 'Pending' },
  { value: 'applied', label: 'Applied' },
  { value: 'dismissed', label: 'Dismissed' },
]

const DECISION_LABELS = {
  typo_of_existing: 'Likely typo',
  alias_of_attribute: 'Existing attribute (alias)',
  new: 'Genuinely new',
}

const DECISION_TONES = {
  typo_of_existing: 'tone-warning',
  alias_of_attribute: 'tone-info',
  new: 'tone-fresh',
}

export default function SchemaDriftPage() {
  const [status, setStatus] = useState('classified')
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(null)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await api.get(`/catalog/drift?status=${status}&limit=200`)
      setItems(data.findings || [])
    } catch (e) {
      setError(e.message || 'Failed to load drift findings')
    } finally {
      setLoading(false)
    }
  }, [status])

  useEffect(() => { refresh() }, [refresh])

  async function decide(id, action) {
    setBusy(id)
    try {
      await api.post(`/catalog/drift/${id}/${action}`, {})
      refresh()
    } catch (e) {
      setError(e.message || `Failed to ${action}`)
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="sd-page">
      <div className="page-header">
        <div>
          <div className="page-title">Schema drift</div>
          <div className="catalog-breadcrumb">
            Catalog / <strong>Schema drift</strong>
          </div>
        </div>
      </div>

      <div className="sd-tabs">
        {STATUS_TABS.map((t) => (
          <button
            key={t.value}
            className={`sd-tab ${status === t.value ? 'active' : ''}`}
            onClick={() => setStatus(t.value)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {error && <div className="pd-message error">{error}</div>}

      {loading ? (
        <div className="center-spinner"><Spinner /></div>
      ) : items.length === 0 ? (
        <div className="sd-empty">
          <AlertCircle size={28} />
          <p className="sd-empty-title">
            {status === 'classified'
              ? 'No pending drift findings. Every recently-imported field matches your current mapping.'
              : 'Nothing in this bucket yet.'}
          </p>
        </div>
      ) : (
        <ul className="sd-list">
          {items.map((f) => (
            <DriftRow
              key={f.id}
              finding={f}
              busy={busy === f.id}
              onApply={() => decide(f.id, 'apply')}
              onDismiss={() => decide(f.id, 'dismiss')}
              actionable={status === 'classified'}
            />
          ))}
        </ul>
      )}
    </div>
  )
}

function DriftRow({ finding, busy, onApply, onDismiss, actionable }) {
  const decisionLabel = DECISION_LABELS[finding.decision] || finding.decision || '—'
  const decisionTone = DECISION_TONES[finding.decision] || 'tone-info'
  const sa = finding.suggested_action || {}
  return (
    <li className="sd-row">
      <div className="sd-row-head">
        <span className="sd-field-name">{finding.field_name}</span>
        {finding.type_guess && <span className="sd-pill sd-pill-type">{finding.type_guess}</span>}
        <span className={`sd-pill ${decisionTone}`}>{decisionLabel}</span>
        {typeof finding.confidence === 'number' && (
          <span className="sd-confidence">conf {(finding.confidence * 100).toFixed(0)}%</span>
        )}
      </div>
      {sa && sa.kind && (
        <div className="sd-action">
          <strong>{sa.kind}</strong>
          {sa.vertical && <> · vertical: <code>{sa.vertical}</code></>}
          {sa.from && <> · from: <code>{sa.from}</code></>}
          {sa.to && <> · to: <code>{sa.to}</code></>}
          {sa.transform && <> · transform: <code>{sa.transform}</code></>}
        </div>
      )}
      <DriftStatsLine stats={finding.stats} />
      {actionable && (
        <div className="sd-actions">
          <Button onClick={onApply} disabled={busy} variant="primary">
            <CheckCircle2 size={14} /> Apply
          </Button>
          <Button onClick={onDismiss} disabled={busy} variant="secondary">
            <XCircle size={14} /> Dismiss
          </Button>
        </div>
      )}
    </li>
  )
}

function DriftStatsLine({ stats }) {
  if (!stats) return null
  const samples = Array.isArray(stats.sample_values) ? stats.sample_values.slice(0, 5) : []
  return (
    <div className="sd-stats">
      <span>rows: {stats.non_null_count ?? '—'}</span>
      <span>distinct: {stats.distinct_count ?? '—'}</span>
      {samples.length > 0 && (
        <span className="sd-samples">
          samples: {samples.map((s, i) => <code key={i}>{String(s)}</code>).reduce((prev, curr) => [prev, ', ', curr])}
        </span>
      )}
    </div>
  )
}
