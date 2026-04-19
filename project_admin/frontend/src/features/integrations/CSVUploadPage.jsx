import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import Papa from 'papaparse'
import { api } from '../../shared/api/apiClient.js'
import useJobPolling from '../../shared/hooks/useJobPolling.js'
import Button from '../../shared/ui/Button.jsx'
import Badge from '../../shared/ui/Badge.jsx'
import MappingEditor from './MappingEditor.jsx'
import './integrations.css'

const MAX_ROWS = 25000

export default function CSVUploadPage() {
  const navigate = useNavigate()
  const [file, setFile] = useState(null)
  const [headers, setHeaders] = useState([])
  const [rows, setRows] = useState([])
  const [mapping, setMapping] = useState({})
  const [confidence, setConfidence] = useState({})
  const [suggesting, setSuggesting] = useState(false)
  const [importing, setImporting] = useState(false)
  const [jobId, setJobId] = useState(null)
  const [error, setError] = useState('')

  const { job } = useJobPolling(jobId, {
    onComplete: () => setTimeout(() => navigate('/integrations?connected=csv'), 800),
  })

  async function handleFile(e) {
    const f = e.target.files?.[0]
    if (!f) return
    setError('')
    setFile(f)
    setHeaders([])
    setRows([])
    setMapping({})
    setConfidence({})

    Papa.parse(f, {
      header: false,
      skipEmptyLines: true,
      complete: async (result) => {
        if (!result.data?.length) {
          setError('CSV looks empty')
          setFile(null)
          return
        }
        const [hdr, ...body] = result.data
        if (body.length > MAX_ROWS) {
          setError(`CSV has ${body.length.toLocaleString()} rows — cap is ${MAX_ROWS.toLocaleString()}. Split or use Shopify integration.`)
          setFile(null)
          return
        }
        setHeaders(hdr)
        setRows(body)
        await fetchSuggestion(hdr, body.slice(0, 5))
      },
      error: (err) => {
        setError(`Parse error: ${err.message}`)
        setFile(null)
      },
    })
  }

  async function fetchSuggestion(hdr, sampleRows) {
    setSuggesting(true)
    try {
      const res = await api.post('/integrations/csv/suggest-mapping', {
        headers: hdr,
        sampleRows,
      })
      setMapping(res.mapping || {})
      setConfidence(res.confidence || {})
    } catch (err) {
      // Mapping may be unavailable (no ANTHROPIC_API_KEY) — user just picks
      // fields manually in that case.
      setError(`AI mapping unavailable: ${err.message}. Pick columns manually.`)
      const fallback = {}
      hdr.forEach((h) => { fallback[h] = 'ignore' })
      setMapping(fallback)
      setConfidence({})
    } finally {
      setSuggesting(false)
    }
  }

  function updateMapping(header, field) {
    setMapping((prev) => ({ ...prev, [header]: field }))
    // User override clears AI confidence on that column.
    setConfidence((prev) => ({ ...prev, [header]: 0 }))
  }

  async function handleImport() {
    if (!file) return
    setImporting(true)
    setError('')
    try {
      const res = await api.post('/integrations/csv/import', {
        headers,
        rows,
        mapping,
        filename: file.name,
      })
      setJobId(res.jobId)
    } catch (err) {
      setError(err.message)
    } finally {
      setImporting(false)
    }
  }

  const progress = job
    ? job.totalItems > 0 ? Math.round((job.processedItems / job.totalItems) * 100) : 0
    : 0

  return (
    <div>
      <h1 className="page-title">CSV upload</h1>
      <p className="text-secondary">Drop a CSV — we propose the column mapping, you confirm, then the import runs.</p>

      {!headers.length && (
        <div
          className={`csv-dropzone ${file ? 'has-file' : ''}`}
          onClick={() => document.getElementById('csv-file-input')?.click()}
        >
          <input id="csv-file-input" type="file" accept=".csv,text/csv" onChange={handleFile} hidden />
          <p style={{ margin: 0, fontWeight: 500 }}>{file ? file.name : 'Click to pick a CSV file'}</p>
          <p className="text-secondary" style={{ marginTop: 8 }}>Max {MAX_ROWS.toLocaleString()} rows · UTF-8</p>
        </div>
      )}

      {error && <div className="auth-error" style={{ marginTop: 16 }}>{error}</div>}

      {suggesting && <p className="text-secondary" style={{ marginTop: 16 }}>AI is proposing a mapping…</p>}

      {headers.length > 0 && !suggesting && !jobId && (
        <>
          <div style={{ marginTop: 20, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span className="text-secondary">{rows.length.toLocaleString()} rows detected</span>
            <Button variant="secondary" onClick={() => { setFile(null); setHeaders([]); setRows([]) }}>
              Pick a different file
            </Button>
          </div>
          <MappingEditor
            headers={headers}
            mapping={mapping}
            confidence={confidence}
            onChange={updateMapping}
          />
          <div style={{ marginTop: 20 }}>
            <Button onClick={handleImport} disabled={importing}>
              {importing ? 'Starting import…' : `Import ${rows.length.toLocaleString()} products`}
            </Button>
          </div>
        </>
      )}

      {job && (
        <div className="import-progress" style={{ marginTop: 24 }}>
          <div className="import-progress-header">
            <span>Import <Badge status={job.status} /></span>
            <span>{job.processedItems}/{job.totalItems}</span>
          </div>
          <div className="import-progress-bar">
            <div className="import-progress-fill" style={{ width: `${progress}%` }} />
          </div>
          {job.errorCount > 0 && <p className="import-error-count">{job.errorCount} errors</p>}
        </div>
      )}
    </div>
  )
}
