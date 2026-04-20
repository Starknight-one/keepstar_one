import { createContext, useContext, useState, useEffect, useRef, useCallback } from 'react'
import { api, setToken, clearToken } from '../../shared/api/apiClient.js'

const AuthContext = createContext(null)

const REFRESH_KEY = 'refresh_token'

function setRefreshToken(t) {
  if (t) localStorage.setItem(REFRESH_KEY, t)
}

function getRefreshToken() {
  return localStorage.getItem(REFRESH_KEY)
}

function clearRefreshToken() {
  localStorage.removeItem(REFRESH_KEY)
}

// Decode JWT exp (seconds) without verifying — we only need the timestamp to
// schedule silent refresh 60s before expiry. If decode fails, we return 0 and
// skip scheduling (the 401 interceptor is the backup path).
function expOf(jwt) {
  try {
    const payload = jwt.split('.')[1]
    const decoded = JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/')))
    return Number(decoded.exp) || 0
  } catch {
    return 0
  }
}

export function useAuth() {
  return useContext(AuthContext)
}

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)
  const refreshTimer = useRef(null)

  const scheduleRefresh = useCallback((accessToken) => {
    if (refreshTimer.current) {
      clearTimeout(refreshTimer.current)
      refreshTimer.current = null
    }
    const exp = expOf(accessToken)
    if (!exp) return
    const msUntilRefresh = Math.max(5_000, exp * 1000 - Date.now() - 60_000)
    refreshTimer.current = setTimeout(() => doRefresh(), msUntilRefresh)
  }, [])

  const doRefresh = useCallback(async () => {
    const rt = getRefreshToken()
    if (!rt) return
    try {
      const data = await api.post('/auth/sessions/refresh', { refresh_token: rt })
      setToken(data.access_token)
      setRefreshToken(data.refresh_token)
      scheduleRefresh(data.access_token)
    } catch (_) {
      clearToken()
      clearRefreshToken()
      setUser(null)
      window.location.href = '/auth/session-expired'
    }
  }, [scheduleRefresh])

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) {
      setLoading(false)
      return
    }
    scheduleRefresh(token)
    api.get('/auth/me')
      .then(setUser)
      .catch(() => {
        clearToken()
        clearRefreshToken()
      })
      .finally(() => setLoading(false))
    return () => {
      if (refreshTimer.current) clearTimeout(refreshTimer.current)
    }
  }, [scheduleRefresh])

  async function login(email, password) {
    const data = await api.post('/auth/login', { email, password })
    // 2FA gate — caller must hand pre_2fa_token to /2fa/verify/*. Do NOT
    // install any tokens here; the user isn't authenticated yet.
    if (data?.requires_2fa) return data
    const access = data.access_token || data.token
    if (access) {
      setToken(access)
      scheduleRefresh(access)
    }
    if (data.refresh_token) setRefreshToken(data.refresh_token)
    setUser(data.user)
    return data
  }

  async function signup(email, password, companyName) {
    const data = await api.post('/auth/signup', { email, password, companyName })
    setToken(data.access_token || data.token)
    if (data.refresh_token) setRefreshToken(data.refresh_token)
    setUser(data.user)
    scheduleRefresh(data.access_token || data.token)
    return data
  }

  // adoptSession takes a server auth response (from Google/Telegram OAuth) and
  // installs the tokens + user state exactly as login() would have.
  function adoptSession(data) {
    const access = data.access_token || data.token
    setToken(access)
    if (data.refresh_token) setRefreshToken(data.refresh_token)
    setUser(data.user)
    scheduleRefresh(access)
  }

  async function logout() {
    const rt = getRefreshToken()
    try {
      if (rt) await api.post('/auth/logout', { refresh_token: rt })
    } catch (_) {}
    clearToken()
    clearRefreshToken()
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{ user, loading, login, signup, logout, adoptSession }}>
      {children}
    </AuthContext.Provider>
  )
}
