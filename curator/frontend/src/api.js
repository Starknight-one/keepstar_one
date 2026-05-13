// Same-origin fetch helpers. Cookies carry the session, so credentials:include
// is required.

async function request(method, path, body) {
  const res = await fetch(path, {
    method,
    credentials: 'include',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  // 401 → throw a typed error; App.jsx detects it and shows LoginPage via React
  // state. Do NOT do window.location.href = '/login' here — that triggers a
  // hard browser reload which races with App's auth-check fetch and creates
  // a re-render flicker loop.
  if (res.status === 401) {
    const err = new Error('unauthorized')
    err.unauthorized = true
    throw err
  }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || 'request failed')
  return data
}

export const api = {
  get: (p) => request('GET', p),
  post: (p, b) => request('POST', p, b),
  delete: (p) => request('DELETE', p),
}

// V5 chat / trace inspection (chunk 13). Curator reads
// v5_chat_sessions + v5_chat_session_traces directly from the shared
// Neon DB.
export const chatsApi = {
  list: ({ tenant = '', status = '', q = '', limit = 50, offset = 0 } = {}) => {
    const params = new URLSearchParams()
    if (tenant) params.set('tenant', tenant)
    if (status) params.set('status', status)
    if (q) params.set('q', q)
    if (limit) params.set('limit', String(limit))
    if (offset) params.set('offset', String(offset))
    const qs = params.toString()
    return api.get(`/curator/chats${qs ? '?' + qs : ''}`)
  },
  timeline: (sessionId) => api.get(`/curator/chats/${sessionId}`),
  turn: (sessionId, requestId) => api.get(`/curator/chats/${sessionId}/turns/${requestId}`),
}

// Merge agent (Phase D3 backend, Phase 4 UI). All endpoints proxy through
// curator-backend → admin-backend internal API.
export const mergeApi = {
  run: (tenantId) => api.post(`/curator/tenants/${tenantId}/merge/run`),
  list: (tenantId, limit = 20) => api.get(`/curator/tenants/${tenantId}/merge-reports?limit=${limit}`),
  get: (reportId) => api.get(`/curator/merge-reports/${reportId}`),
  apply: (reportId, body) => api.post(`/curator/merge-reports/${reportId}/apply`, body),
  revert: (reportId, body) => api.post(`/curator/merge-reports/${reportId}/revert`, body || {}),
  discover: (tenantId) => api.post(`/curator/tenants/${tenantId}/discover`),
}
