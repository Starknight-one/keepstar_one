// Upload — form-primitive leaf (§5.2, R25 async two-phase):
//   {type:"upload", name, accept:[".csv",".json"], maxSizeMb?, disarmed?,
//    token?, note?}
//
// The control is ALWAYS usable (owner decision 2026-07-28): the user's
// action IS the approval — the uploader must never be locked behind the
// model having called apply_manifest. The legacy `disarmed` flag and a
// missing token no longer disable anything; the request carries the
// sessionId and the server resolves the session's ingest door (auto-
// applying the staged manifest when needed). A bound token still rides
// along when the document has one. Flow:
//   pick file → validate extension/size client-side →
//   POST {base}/onboard/upload (multipart: sessionId [+ token] BEFORE
//   the file) → {jobId} →
//   poll GET {base}/onboard/upload/{jobId}?sessionId= →
//   completed {processed, projectionRows, invalidated} → summary line;
//   failed {errors[]} → error line + the picker re-enables (re-upload,
//   R25). Server rejections surface their reason inline, not a generic
//   "try again".
//
// The upload endpoints are cookie-gated (R5) — credentials:'include' so
// ks_onboard rides along in cross-origin dev; prod is same-origin (§5.1).

import { useEffect, useRef, useState } from 'react'
import { useRenderContext } from '../RenderContext'
import { uploadOnboardFile, onboardUploadStatus } from '../../api/onboard'

const DEFAULT_ACCEPT = ['.csv', '.json']
const DEFAULT_MAX_MB = 20
const POLL_MS = 1500
const MAX_POLLS = 200 // ~5 min of processing before we stop polling

export default function Upload({ node }) {
  const ctx = useRenderContext()
  const inputRef = useRef(null)
  const aliveRef = useRef(true)
  const [phase, setPhase] = useState('idle') // idle|uploading|processing|done|failed
  const [message, setMessage] = useState('')

  useEffect(() => {
    aliveRef.current = true
    return () => {
      aliveRef.current = false
    }
  }, [])

  const accept = Array.isArray(node.accept) && node.accept.length > 0 ? node.accept : DEFAULT_ACCEPT
  const maxMb = typeof node.maxSizeMb === 'number' && node.maxSizeMb > 0 ? node.maxSizeMb : DEFAULT_MAX_MB
  const token = typeof node.token === 'string' && node.token !== '' ? node.token : undefined
  const busy = phase === 'uploading' || phase === 'processing'

  async function handleFile(file) {
    if (!file) return
    const name = String(file.name || '').toLowerCase()
    if (!accept.some((ext) => name.endsWith(String(ext).toLowerCase()))) {
      setPhase('failed')
      setMessage(`Unsupported file type — expected ${accept.join(' or ')}.`)
      return
    }
    if (file.size > maxMb * 1024 * 1024) {
      setPhase('failed')
      setMessage(`File is too large — up to ${maxMb} MB.`)
      return
    }

    setPhase('uploading')
    setMessage('Uploading…')
    try {
      const job = await uploadOnboardFile({
        baseUrl: ctx?.apiBaseUrl,
        token,
        sessionId: ctx?.sessionId || undefined,
        file,
      })
      if (!aliveRef.current) return
      setPhase('processing')
      setMessage('Processing…')
      const sessionId = job.sessionId || ctx?.sessionId || ''
      for (let i = 0; i < MAX_POLLS; i++) {
        await sleep(POLL_MS)
        if (!aliveRef.current) return
        const status = await onboardUploadStatus({ baseUrl: ctx?.apiBaseUrl, jobId: job.jobId, sessionId })
        if (!aliveRef.current) return
        if (status.status === 'completed') {
          setPhase('done')
          setMessage(completedMessage(status))
          return
        }
        if (status.status === 'failed') {
          setPhase('failed')
          setMessage(failedMessage(status))
          return
        }
        if (typeof status.processed === 'number' && status.processed > 0) {
          setMessage(`Processing… ${status.processed} items`)
        }
      }
      setPhase('failed')
      setMessage('Import is taking too long — please check back or re-upload.')
    } catch (err) {
      if (!aliveRef.current) return
      // eslint-disable-next-line no-console
      console.error('[v5-upload] upload failed', err.message)
      setPhase('failed')
      setMessage(serverReason(err) || 'Upload failed — please try again.')
    }
  }

  return (
    <div className="kw-upload" data-id={node.id || ''} data-phase={phase}>
      <input
        ref={inputRef}
        className="kw-upload-input"
        type="file"
        name={node.name || 'file'}
        accept={accept.join(',')}
        disabled={busy || undefined}
        onClick={(e) => e.stopPropagation()}
        onChange={(e) => {
          const file = e.target.files && e.target.files[0]
          // Reset so re-picking the same file after a failure re-fires.
          e.target.value = ''
          handleFile(file)
        }}
      />
      <button
        type="button"
        className="kw-upload-button"
        disabled={busy || undefined}
        aria-busy={busy || undefined}
        onClick={(e) => {
          e.stopPropagation()
          e.preventDefault()
          if (inputRef.current) inputRef.current.click()
        }}
      >
        {busy ? 'Uploading…' : `Choose a file (${accept.join(', ')})`}
      </button>
      {node.note ? <p className="kw-upload-note">{node.note}</p> : null}
      {message ? (
        <p
          className="kw-upload-status"
          data-status={phase}
          role={phase === 'failed' ? 'alert' : 'status'}
        >
          {message}
        </p>
      ) : null}
    </div>
  )
}

function completedMessage(status) {
  const rows = typeof status.projectionRows === 'number' ? status.projectionRows : 0
  const processed = typeof status.processed === 'number' ? status.processed : rows
  let msg = `Imported ${processed} items — ${rows} searchable listings.`
  if (status.invalidated === false) {
    msg += ' (Search cache refresh pending.)'
  }
  return msg
}

function failedMessage(status) {
  const errs = Array.isArray(status.errors) ? status.errors.filter(Boolean) : []
  if (errs.length === 0) return 'Import failed — please check the file and re-upload.'
  return `Import failed: ${errs.slice(0, 3).join('; ')} — please fix the file and re-upload.`
}

// serverReason — the server's own words when it rejected the upload
// (onboard.js attaches the response body as err.body). Long or empty
// bodies fall back to the generic line.
function serverReason(err) {
  const body = typeof err?.body === 'string' ? err.body.trim() : ''
  if (!body || body.length > 200) return ''
  return body
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
