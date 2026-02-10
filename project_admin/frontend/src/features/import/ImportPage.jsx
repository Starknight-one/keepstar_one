import { useState, useEffect, useRef, useCallback } from 'react'
import { api } from '../../shared/api/apiClient.js'
import Button from '../../shared/ui/Button.jsx'
import Badge from '../../shared/ui/Badge.jsx'
import Table from '../../shared/ui/Table.jsx'
import Spinner from '../../shared/ui/Spinner.jsx'
import './import.css'

const historyColumns = [
  { key: 'fileName', label: 'File' },
  { key: 'totalItems', label: 'Items' },
  { key: 'processedItems', label: 'Processed' },
  { key: 'errorCount', label: 'Errors' },
  { key: 'status', label: 'Status', render: (row) => <Badge status={row.status} /> },
  { key: 'createdAt', label: 'Date', render: (row) => new Date(row.createdAt).toLocaleString() },
]

export default function ImportPage() {
  const [file, setFile] = useState(null)
  const [preview, setPreview] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [activeJob, setActiveJob] = useState(null)
  const [imports, setImports] = useState([])
  const [error, setError] = useState('')
  const pollRef = useRef(null)
  const fileRef = useRef(null)

  const fetchImports = useCallback(async () => {
    try {
      const data = await api.get('/catalog/imports?limit=20')
      setImports(data.imports || [])
    } catch { /* ignore */ }
  }, [])

  useEffect(() => {
    fetchImports()
    return () => { if (pollRef.current) clearInterval(pollRef.current) }
  }, [fetchImports])

  function handleFileChange(e) {
    const f = e.target.files?.[0]
    if (!f) return
    setError('')
    setFile(f)

    const reader = new FileReader()
    reader.onload = (ev) => {
      try {
        const data = JSON.parse(ev.target.result)
        if (!data.products?.length) {
          setError('JSON must contain a "products" array')
          setFile(null)
          return
        }
        setPreview(data.products.slice(0, 5))
      } catch {
        setError('Invalid JSON file')
        setFile(null)
      }
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
      setActiveJob(result)
      setFile(null)
      setPreview(null)
      if (fileRef.current) fileRef.current.value = ''

      // Start polling
      pollRef.current = setInterval(async () => {
        try {
          const job = await api.get(`/catalog/import/${result.jobId}`)
          setActiveJob(job)
          if (job.status === 'completed' || job.status === 'failed') {
            clearInterval(pollRef.current)
            pollRef.current = null
            fetchImports()
          }
        } catch { /* ignore */ }
      }, 2000)
    } catch (err) {
      setError(err.message)
    } finally {
      setUploading(false)
    }
  }

  const progress = activeJob
    ? activeJob.totalItems > 0
      ? Math.round((activeJob.processedItems / activeJob.totalItems) * 100)
      : 0
    : 0

  return (
    <div>
      <h1 className="page-title">Import Catalog</h1>

      <div className="import-upload-card">
        <div className="import-drop-zone" onClick={() => fileRef.current?.click()}>
          <input ref={fileRef} type="file" accept=".json" onChange={handleFileChange} hidden />
          <p>{file ? file.name : 'Click to select a .json file'}</p>
        </div>

        {error && <div className="auth-error">{error}</div>}

        {preview && (
          <div className="import-preview">
            <h3>Preview ({preview.length} of {file ? '...' : '0'} items)</h3>
            <table className="table">
              <thead>
                <tr>
                  <th>SKU</th><th>Name</th><th>Brand</th><th>Category</th><th>Price</th>
                </tr>
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
            <Button onClick={handleUpload} disabled={uploading} style={{ marginTop: 12 }}>
              {uploading ? 'Uploading...' : 'Start Import'}
            </Button>
          </div>
        )}

        {activeJob && (
          <div className="import-progress">
            <div className="import-progress-header">
              <span>Import: <Badge status={activeJob.status} /></span>
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
      </div>

      <h2 className="section-title">Import History</h2>
      {imports.length === 0 ? (
        <p className="text-secondary">No imports yet</p>
      ) : (
        <Table columns={historyColumns} data={imports} />
      )}
    </div>
  )
}
