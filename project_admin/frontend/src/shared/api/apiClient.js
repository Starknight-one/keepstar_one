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

async function request(method, path, body) {
  const start = performance.now()
  const headers = { 'Content-Type': 'application/json' }
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`

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
      window.location.href = '/login'
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
  get: (path) => request('GET', path),
  post: (path, body) => request('POST', path, body),
  put: (path, body) => request('PUT', path, body),
}
