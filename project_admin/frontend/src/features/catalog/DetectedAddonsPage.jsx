import { useState, useEffect, useCallback } from 'react'
import { CheckCircle2, XCircle, AlertCircle } from 'lucide-react'
import { api } from '../../shared/api/apiClient.js'
import Button from '../../shared/ui/Button.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './catalog.css'
import './detectedAddons.css'

const STATUS_TABS = [
  { value: 'pending', label: 'Pending' },
  { value: 'confirmed_addon', label: 'Confirmed add-ons' },
  { value: 'false_positive', label: 'False positives' },
]

const REASON_LABELS = {
  axis_name_pattern: 'Axis matches addon pattern',
  no_identifiers: 'No GTIN / SKU',
  no_dimensions: 'No weight/volume',
  small_price_delta: 'Small price delta',
}

export default function DetectedAddonsPage() {
  const [status, setStatus] = useState('pending')
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(null)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await api.get(`/junk?status=${status}`)
      setItems(data.candidates || [])
    } catch (e) {
      setError(e.message || 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [status])

  useEffect(() => { refresh() }, [refresh])

  async function classify(id, classification) {
    setBusy(id)
    try {
      await api.post(`/junk/${id}/classify`, { classification })
      refresh()
    } catch (e) {
      setError(e.message || 'Failed to classify')
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="da-page">
      <div className="page-header">
        <div>
          <div className="page-title">Detected add-ons</div>
          <div className="catalog-breadcrumb">
            Catalog / <strong>Detected add-ons</strong>
          </div>
        </div>
      </div>

      <div className="da-tabs">
        {STATUS_TABS.map((t) => (
          <button
            key={t.value}
            className={`da-tab ${status === t.value ? 'active' : ''}`}
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
        <div className="da-empty">
          <AlertCircle size={28} />
          <p className="da-empty-title">
            {status === 'pending'
              ? 'No detected add-ons yet.'
              : 'Nothing in this bucket.'}
          </p>
          <p className="da-empty-hint">
            Suspicious variants (gift wrap, warranty, engraving) will appear here once your Shopify
            import detects them. Heuristic flags 2+ signals per variant.
          </p>
        </div>
      ) : (
        <ul className="da-list">
          {items.map((item) => (
            <li key={item.id} className="da-item">
              <div className="da-item-main">
                <div className="da-item-id">listing {item.listingId.slice(0, 8)}</div>
                <div className="da-reasons">
                  {Object.entries(item.detectedReason || {}).map(([k, v]) => (
                    <span key={k} className="da-reason">
                      <strong>{REASON_LABELS[k] || k}:</strong>{' '}
                      {typeof v === 'object' ? JSON.stringify(v) : String(v)}
                    </span>
                  ))}
                </div>
              </div>
              {status === 'pending' && (
                <div className="da-actions">
                  <Button
                    variant="primary"
                    pill
                    disabled={busy === item.id}
                    onClick={() => classify(item.id, 'confirmed_addon')}
                  >
                    <CheckCircle2 size={14} /> Mark as add-on
                  </Button>
                  <Button
                    variant="secondary"
                    pill
                    disabled={busy === item.id}
                    onClick={() => classify(item.id, 'false_positive')}
                  >
                    <XCircle size={14} /> Mark as real
                  </Button>
                </div>
              )}
              {status !== 'pending' && (
                <div className="da-meta">
                  classified by {item.classifiedBy || '—'}
                  {item.classifiedAt && ` · ${new Date(item.classifiedAt).toLocaleDateString()}`}
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
