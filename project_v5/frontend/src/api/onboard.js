// Onboarding API client (RUNTIME_SPEC R5 + §5.1). Talks to:
//   POST {base}/onboard/gate      — {password} → sets the HttpOnly
//                                   ks_onboard cookie (R5); 403 wrong
//                                   password, 429 rate-limited (5/min/IP),
//                                   503 onboarding disabled (env unset)
//   POST {base}/onboard/session   — create a mode=onboarding session
//                                   (cookie-gated) → {sessionId, mode,
//                                   tenant:{slug,name}}
//   GET  {base}/onboard/session?sessionId=
//                                 — resume (refresh-safe, all truth in
//                                   DB) → {sessionId, manifest,
//                                   lastTemplate, transcript}; 404 stale
//                                   id, 409 killed session
//
// All calls ride credentials:'include' so the ks_onboard cookie flows in
// dev setups where the API origin differs from the page origin (prod is
// same-origin per §5.1 — zero CORS). Errors carry `.status` so the shell
// can route: 403 → password gate, 503 → disabled page, 429 → slow down.

export async function submitGate({ baseUrl, password }) {
  const res = await fetch(`${baseUrl}/onboard/gate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ password }),
  })
  if (!res.ok) {
    throw await statusError(res, 'onboard gate')
  }
  return parseJSONBody(res)
}

export async function createOnboardSession({ baseUrl }) {
  const res = await fetch(`${baseUrl}/onboard/session`, {
    method: 'POST',
    credentials: 'include',
  })
  if (!res.ok) {
    throw await statusError(res, 'onboard session')
  }
  return parseJSONBody(res)
}

export async function resumeOnboardSession({ baseUrl, sessionId }) {
  const res = await fetch(`${baseUrl}/onboard/session?sessionId=${encodeURIComponent(sessionId)}`, {
    method: 'GET',
    credentials: 'include',
  })
  if (!res.ok) {
    throw await statusError(res, 'onboard resume')
  }
  return parseJSONBody(res)
}

// uploadOnboardFile — phase 1 of the R25 two-phase uploader: POST the
// multipart body to {base}/onboard/upload. Metadata fields (sessionId,
// and token when the document bound one) are appended BEFORE the file —
// the server streams the file through to admin and must resolve the
// session/token first (field order is preserved on the wire). The token
// is OPTIONAL (owner decision 2026-07-28): the upload works token-less —
// the server resolves the session's ingest door from sessionId, auto-
// applying the staged manifest when needed; the user's upload IS the
// approval.
// → 202 {jobId, status, totalItems, sessionId}
export async function uploadOnboardFile({ baseUrl, token, sessionId, file }) {
  const form = new FormData()
  if (token) form.append('token', token)
  if (sessionId) form.append('sessionId', sessionId)
  form.append('file', file, file.name)
  const res = await fetch(`${baseUrl}/onboard/upload`, {
    method: 'POST',
    credentials: 'include',
    body: form,
  })
  if (!res.ok) {
    throw await statusError(res, 'onboard upload')
  }
  return parseJSONBody(res)
}

// onboardUploadStatus — phase 2: poll the honest admin job status.
// → {jobId, status, processed, totalItems, projectionRows, invalidated, errors[]}
export async function onboardUploadStatus({ baseUrl, jobId, sessionId }) {
  const res = await fetch(
    `${baseUrl}/onboard/upload/${encodeURIComponent(jobId)}?sessionId=${encodeURIComponent(sessionId)}`,
    { method: 'GET', credentials: 'include' },
  )
  if (!res.ok) {
    throw await statusError(res, 'onboard upload status')
  }
  return parseJSONBody(res)
}

async function statusError(res, label) {
  const body = await res.text().catch(() => '')
  const err = new Error(`${label} ${res.status}: ${body}`)
  err.status = res.status
  // The raw response body — callers surface the server's reason inline
  // (e.g. the upload node) instead of a generic "try again".
  err.body = body.trim()
  return err
}

// Gate success may answer with an empty body (the cookie IS the result).
async function parseJSONBody(res) {
  const text = await res.text()
  if (!text) return {}
  try {
    return JSON.parse(text)
  } catch (_) {
    return {}
  }
}
