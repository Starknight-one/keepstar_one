import { useState, useEffect, useRef, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { ShoppingBag, FileSpreadsheet, Sheet, ChevronDown, ChevronUp, CheckCircle2 } from 'lucide-react'
import { api } from '../../shared/api/apiClient.js'
import useJobPolling from '../../shared/hooks/useJobPolling.js'
import Button from '../../shared/ui/Button.jsx'
import Badge from '../../shared/ui/Badge.jsx'
import Table from '../../shared/ui/Table.jsx'
import './import.css'

const SOURCES = [
  {
    key: 'shopify',
    title: 'Shopify',
    icon: ShoppingBag,
    description: 'OAuth install — full catalog sync with live webhooks for create / update / delete.',
    cta: 'Connect',
    href: '/integrations/shopify',
  },
  {
    key: 'csv',
    title: 'CSV upload',
    icon: FileSpreadsheet,
    description: 'Drop a CSV — AI proposes the column mapping, you confirm and import.',
    cta: 'Upload CSV',
    href: '/integrations/csv',
  },
  {
    key: 'gsheets',
    title: 'Google Sheets',
    icon: Sheet,
    description: 'Coming soon — connect a sheet for scheduled sync.',
    cta: 'Coming soon',
    href: null,
  },
]

const historyColumns = [
  { key: 'fileName', label: 'File', render: (r) => r.fileName || `Job #${r.id?.slice(0, 8) || ''}` },
  { key: 'totalItems', label: 'Items', width: '90px' },
  { key: 'processedItems', label: 'Processed', width: '110px' },
  { key: 'errorCount', label: 'Errors', width: '90px' },
  { key: 'status', label: 'Status', width: '110px', render: (row) => <Badge status={row.status} /> },
  { key: 'createdAt', label: 'Date', width: '160px', render: (row) => new Date(row.createdAt).toLocaleString() },
]

export default function ImportPage() {
  const [imports, setImports] = useState([])
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [file, setFile] = useState(null)
  const [preview, setPreview] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [jobId, setJobId] = useState(null)
  const [error, setError] = useState('')
  const fileRef = useRef(null)

  const fetchImports = useCallback(async () => {
    try {
      const data = await api.get('/catalog/imports?limit=20')
      setImports(data.imports || [])
    } catch { /* ignore */ }
  }, [])

  const { job: activeJob } = useJobPolling(jobId, { onComplete: () => fetchImports() })

  useEffect(() => { fetchImports() }, [fetchImports])
  useEffect(() => {
    if (activeJob && activeJob.status === 'failed') fetchImports()
  }, [activeJob, fetchImports])

  function handleFileChange(e) {
    const f = e.target.files?.[0]
    if (!f) return
    setError('')
    setFile(f)
    const reader = new FileReader()
    reader.onload = (ev) => {
      try {
        const data = JSON.parse(ev.target.result)
        if (!data.products?.length) { setError('JSON must contain a "products" array'); setFile(null); return }
        setPreview(data.products.slice(0, 5))
      } catch { setError('Invalid JSON file'); setFile(null) }
    }
    reader.readAsText(f)
  }

  async function handleUpload() {
    if (!file) return
    setUploading(true)
    setError('')
    try {
      const text = await file.text()
      const data = JSON.parse(text)
      const result = await api.post('/catalog/import', data)
      setJobId(result.jobId || result.id)
      setFile(null)
      setPreview(null)
      if (fileRef.current) fileRef.current.value = ''
    } catch (err) {
      setError(err.message)
    } finally {
      setUploading(false)
    }
  }

  const progress = activeJob && activeJob.totalItems > 0
    ? Math.round((activeJob.processedItems / activeJob.totalItems) * 100)
    : 0

  return (
    <div className="import-page">
      <div className="page-header">
        <div>
          <div className="page-title">Import</div>
          <div className="page-subtitle">Connect a catalog source — Keepstar syncs products automatically.</div>
        </div>
      </div>

      <div className="source-grid">
        {SOURCES.map(src => {
          const Icon = src.icon
          const card = (
            <div className="source-card">
              <div className="source-card-icon"><Icon size={20} /></div>
              <div className="source-card-title">{src.title}</div>
              <div className="source-card-desc">{src.description}</div>
              <div className="source-card-cta">
                {src.href
                  ? <Button variant="primary" pill style={{ width: '100%' }}>{src.cta}</Button>
                  : <Button variant="secondary" pill disabled style={{ width: '100%' }}>{src.cta}</Button>}
              </div>
            </div>
          )
          return src.href
            ? <Link key={src.key} to={src.href} style={{ textDecoration: 'none' }}>{card}</Link>
            : <div key={src.key}>{card}</div>
        })}
      </div>

      {activeJob && (
        <div className="import-progress">
          <div className="import-progress-header">
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
              <CheckCircle2 size={14} /> Import in progress · <Badge status={activeJob.status} />
            </span>
            <span>{activeJob.processedItems}/{activeJob.totalItems} items</span>
          </div>
          <div className="import-progress-bar">
            <div className="import-progress-fill" style={{ width: `${progress}%` }} />
          </div>
          {activeJob.errorCount > 0 && (
            <p className="import-error-count">{activeJob.errorCount} errors</p>
          )}
        </div>
      )}

      <div>
        <div className="import-section-header">
          <div className="section-title">Recent import jobs</div>
        </div>
        {imports.length === 0
          ? <div className="import-empty">No imports yet — pick a source above to begin.</div>
          : <Table columns={historyColumns} data={imports} />}
      </div>

      <div className="import-disclosure">
        <button className="import-disclosure-toggle" onClick={() => setAdvancedOpen(o => !o)}>
          <span>Advanced — JSON upload</span>
          {advancedOpen ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
        </button>
        {advancedOpen && (
          <div className="import-disclosure-body">
            <div className="import-drop-zone" onClick={() => fileRef.current?.click()}>
              <input ref={fileRef} type="file" accept=".json" onChange={handleFileChange} hidden />
              <p>{file ? file.name : 'Click to select a .json file'}</p>
            </div>

            {error && <div className="import-error-msg" style={{ marginTop: 12 }}>{error}</div>}

            {preview && (
              <div className="import-preview">
                <h3>Preview ({preview.length} items)</h3>
                <table className="table">
                  <thead>
                    <tr><th>SKU</th><th>Name</th><th>Brand</th><th>Category</th><th>Price</th></tr>
                  </thead>
                  <tbody>
                    {preview.map((item, i) => (
                      <tr key={i}>
                        <td>{item.sku}</td>
                        <td>{item.name}</td>
                        <td>{item.brand}</td>
                        <td>{item.category}</td>
                        <td>{item.price}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                <Button variant="primary" pill onClick={handleUpload} disabled={uploading} style={{ marginTop: 12 }}>
                  {uploading ? 'Uploading…' : 'Start import'}
                </Button>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
