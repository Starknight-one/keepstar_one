import { log } from '../logger'

const BASE = '/admin/api'

function getToken() {
  return localStorage.getItem('token')
}

export function setToken(token) {
  localStorage.setItem('token', token)
}

export function clearToken() {
  localStorage.removeItem('token')
}

async function request(method, path, body, options = {}) {
  const start = performance.now()
  const headers = { 'Content-Type': 'application/json' }
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (options.headers) Object.assign(headers, options.headers)

  let status = 0
  try {
    const res = await fetch(`${BASE}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    })
    status = res.status

    if (res.status === 401) {
      clearToken()
      window.location.href = '/auth/sign-in'
      throw new Error('Unauthorized')
    }

    const data = await res.json()
    if (!res.ok) throw new Error(data.error || 'Request failed')
    return data
  } catch (err) {
    if (!status) log.api(method, path, 0, Math.round(performance.now() - start), err)
    throw err
  } finally {
    if (status) log.api(method, path, status, Math.round(performance.now() - start))
  }
}

export const api = {
  get: (path, options) => request('GET', path, undefined, options),
  post: (path, body, options) => request('POST', path, body, options),
  put: (path, body, options) => request('PUT', path, body, options),
  delete: (path, options) => request('DELETE', path, undefined, options),
}
