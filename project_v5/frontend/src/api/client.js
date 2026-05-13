// V5 backend API client. Talks to:
//   POST  {base}/session/init      — create a session for this tenant
//   POST  {base}/pipeline          — run one Agent1 → Agent2 turn
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

function tenantHeaders(tenantSlug) {
  return tenantSlug ? { 'X-Tenant-Slug': tenantSlug } : {}
}
