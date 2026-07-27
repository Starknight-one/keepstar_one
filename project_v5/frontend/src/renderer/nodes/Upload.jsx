// Upload — form-primitive leaf (§5.2, R25 async two-phase):
//   {type:"upload", name, accept:[".csv",".json"], maxSizeMb?, disarmed?,
//    token?, note?}
//
// Beat 2 renders the node DISARMED (disarmed:true — file picking is off,
// the note explains it activates after the plan is confirmed). The beat-3
// re-render arms it: disarmed:false + the minted ingest token bound via
// render-time ops. Armed flow:
//   pick file → validate extension/size client-side →
//   POST {base}/onboard/upload (multipart, token BEFORE file) → {jobId} →
//   poll GET {base}/onboard/upload/{jobId}?sessionId= →
//   completed {processed, projectionRows, invalidated} → summary line;
//   failed {errors[]} → error line + the picker re-enables (re-upload, R25).
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
  const hasToken = typeof node.token === 'string' && node.token !== ''
  const armed = !node.disarmed && hasToken
  const busy = phase === 'uploading' || phase === 'processing'

  // Disarmed presentation: the explicit disarmed flag keeps the seeded
  // note; an armed-without-token render is a document bug — say so loudly
  // instead of a dead button.
  const note = node.disarmed
    ? node.note || 'The uploader activates once you confirm the plan.'
    : !hasToken
      ? 'Upload door is not armed yet — no upload token on this widget.'
      : node.note || ''

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
      const job = await uploadOnboardFile({ baseUrl: ctx?.apiBaseUrl, token: node.token, file })
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
      setMessage('Upload failed — please try again.')
    }
  }

  return (
    <div className="kw-upload" data-id={node.id || ''} data-armed={armed ? 'true' : 'false'} data-phase={phase}>
      <input
        ref={inputRef}
        className="kw-upload-input"
        type="file"
        name={node.name || 'file'}
        accept={accept.join(',')}
        disabled={!armed || busy || undefined}
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
        disabled={!armed || busy || undefined}
        aria-busy={busy || undefined}
        onClick={(e) => {
          e.stopPropagation()
          e.preventDefault()
          if (inputRef.current) inputRef.current.click()
        }}
      >
        {busy ? 'Uploading…' : `Choose a file (${accept.join(', ')})`}
      </button>
      {note ? <p className="kw-upload-note">{note}</p> : null}
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

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
