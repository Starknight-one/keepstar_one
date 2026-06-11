// V5 backend API client. Talks to:
//   POST  {base}/session/init      — create a session for this tenant
//   POST  {base}/pipeline          — run one Agent1 → Agent2 turn
//   POST  {base}/pipeline/stream   — same turn, SSE progress frames
//
// Base URL flows in from the embed <script data-api> attribute (see
// widget.jsx); in dev it falls back to http://localhost:8082/api/v1.
//
// Both calls send X-Tenant-Slug per V4 convention; the V5 router's
// tenant middleware reads it.

export async function initSession({ baseUrl, tenantSlug }) {
  const res = await fetch(`${baseUrl}/session/init`, {
    method: 'POST',
    headers: tenantHeaders(tenantSlug),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`session/init ${res.status}: ${body}`)
  }
  return res.json()
}

export async function pipelineRequest({ baseUrl, tenantSlug, sessionId, query }) {
  const res = await fetch(`${baseUrl}/pipeline`, {
    method: 'POST',
    headers: {
      ...tenantHeaders(tenantSlug),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ sessionId, query }),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`pipeline ${res.status}: ${body}`)
  }
  return res.json()
}

// pipelineStreamRequest — SSE-over-POST variant of pipelineRequest.
// Emits each `stage` frame payload through onStage and resolves with
// the `result` frame payload (same shape as the /pipeline JSON body).
export async function pipelineStreamRequest({ baseUrl, tenantSlug, sessionId, query, onStage }) {
  const res = await fetch(`${baseUrl}/pipeline/stream`, {
    method: 'POST',
    headers: {
      ...tenantHeaders(tenantSlug),
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
    },
    body: JSON.stringify({ sessionId, query }),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`pipeline ${res.status}: ${body}`)
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let sawFrame = false
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      // Frames may split across chunks — accumulate, then peel off every
      // complete (\n\n-terminated) frame in the buffer. CRLF normalized
      // in case an intermediary rewrites newlines.
      buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, '\n')
      let sep
      while ((sep = buffer.indexOf('\n\n')) !== -1) {
        const frame = parseFrame(buffer.slice(0, sep))
        buffer = buffer.slice(sep + 2)
        if (!frame) continue
        sawFrame = true
        if (frame.event === 'stage') {
          // A buggy status callback must not abort the turn.
          try {
            onStage?.(frame.data)
          } catch (cbErr) {
            // eslint-disable-next-line no-console
            console.warn('[v5-widget] onStage callback failed', cbErr)
          }
        } else if (frame.event === 'result') {
          return frame.data
        } else if (frame.event === 'error') {
          const err = new Error(`pipeline ${frame.data.status}: ${frame.data.message}`)
          err.serverAnswered = true
          throw err
        }
      }
    }
    const err = new Error('pipeline stream ended early')
    err.midStream = sawFrame
    throw err
  } catch (err) {
    if (err.midStream === undefined && !err.serverAnswered) err.midStream = sawFrame
    throw err
  } finally {
    try {
      await reader.cancel?.()
    } catch {
      /* teardown only */
    }
  }
}

// pipelineSmartRequest — streaming with a one-shot fallback to the old
// JSON endpoint, but ONLY when the stream never really started (older
// backend without the route, proxy refusing the request). Once the
// server has spoken SSE — an error frame, or a connection lost after
// frames arrived — the turn may already have run and been charged;
// re-running the whole 2-agent pipeline would double LLM spend and
// duplicate the session turn, so those errors surface as-is. Lives
// here, not in WidgetApp, so it stays unit-testable.
export async function pipelineSmartRequest(opts) {
  try {
    return await pipelineStreamRequest(opts)
  } catch (err) {
    if (err && (err.serverAnswered || err.midStream)) throw err
    return pipelineRequest(opts)
  }
}

// One SSE frame = an `event:` line plus one or more `data:` lines whose
// concatenation is a JSON payload.
function parseFrame(raw) {
  let event = ''
  const dataLines = []
  for (const line of raw.split('\n')) {
    if (line.startsWith('event:')) {
      event = line.slice('event:'.length).trim()
    } else if (line.startsWith('data:')) {
      dataLines.push(line.slice('data:'.length).trimStart())
    }
  }
  if (!event || dataLines.length === 0) return null
  return { event, data: JSON.parse(dataLines.join('\n')) }
}

function tenantHeaders(tenantSlug) {
  return tenantSlug ? { 'X-Tenant-Slug': tenantSlug } : {}
}
