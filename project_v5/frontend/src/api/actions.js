// Action + navigation API client. Talks to:
//   POST  {base}/actions                       — like / cart actions
//   POST  {base}/navigation/expand             — drill into detail preset
//   POST  {base}/navigation/back               — pop view stack
//
// Fire-and-forget pattern (sync=true) is supported on /actions so the
// frontend can update its local state without waiting for the response
// when no echo is needed.

export async function postAction({ baseUrl, tenantSlug, sessionId, kind, entity, params, sync = false }) {
  const url = `${baseUrl}/actions${sync ? '?sync=true' : ''}`
  const res = await fetch(url, {
    method: 'POST',
    headers: {
      ...tenantHeaders(tenantSlug),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ sessionId, kind, entity, params }),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`actions ${res.status}: ${body}`)
  }
  // 204 (sync mode) → no body to parse.
  if (res.status === 204) return null
  return res.json()
}

export async function expandView({ baseUrl, tenantSlug, sessionId, entityType, entityId }) {
  const res = await fetch(`${baseUrl}/navigation/expand`, {
    method: 'POST',
    headers: {
      ...tenantHeaders(tenantSlug),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ sessionId, entityType, entityId }),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`navigation/expand ${res.status}: ${body}`)
  }
  return res.json()
}

export async function goBack({ baseUrl, tenantSlug, sessionId }) {
  const res = await fetch(`${baseUrl}/navigation/back`, {
    method: 'POST',
    headers: {
      ...tenantHeaders(tenantSlug),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ sessionId }),
  })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`navigation/back ${res.status}: ${body}`)
  }
  return res.json()
}

function tenantHeaders(tenantSlug) {
  return tenantSlug ? { 'X-Tenant-Slug': tenantSlug } : {}
}
