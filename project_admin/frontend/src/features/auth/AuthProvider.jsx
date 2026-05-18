import { createContext, useContext, useState, useEffect, useRef, useCallback } from 'react'
import { api, setToken, clearToken } from '../../shared/api/apiClient.js'

const AuthContext = createContext(null)

const REFRESH_KEY = 'refresh_token'
const LAST_METHOD_KEY = 'last_login_method'
const REMEMBER_KEY = 'remember_me'
// 15a: surface "Last time you signed in with X" on /signin so returning
// users skip retyping. Stored client-side only (per-browser, no backend
// endpoint) to avoid the anti-enumeration trap of an email→method lookup.
const LAST_METHOD_TTL_MS = 30 * 24 * 60 * 60 * 1000

// Scenario 14: "Remember me" checkbox toggles storage strategy for the
// refresh token. Backend always issues a 30-day refresh; what changes is
// where the frontend keeps it:
//   - remember=true  → localStorage (survives browser restart, 30 days)
//   - remember=false → sessionStorage (cleared when the tab closes)
// Access token always lives in apiClient (memory) so a closed-tab session
// can't ride on a stolen localStorage entry alone.
function setRefreshToken(t, remember) {
  if (!t) return
  // Always wipe both stores first to avoid drift when the user toggles
  // remember between sessions.
  sessionStorage.removeItem(REFRESH_KEY)
  localStorage.removeItem(REFRESH_KEY)
  if (remember) {
    localStorage.setItem(REFRESH_KEY, t)
    localStorage.setItem(REMEMBER_KEY, '1')
  } else {
    sessionStorage.setItem(REFRESH_KEY, t)
    localStorage.removeItem(REMEMBER_KEY)
  }
}

function getRefreshToken() {
  return localStorage.getItem(REFRESH_KEY) || sessionStorage.getItem(REFRESH_KEY)
}

function getRememberMe() {
  // Default to true so existing sessions (pre-feature) keep behaving like
  // "remember me on". Users who explicitly uncheck the box flip this to false.
  return localStorage.getItem(REMEMBER_KEY) !== '0'
}

function clearRefreshToken() {
  localStorage.removeItem(REFRESH_KEY)
  sessionStorage.removeItem(REFRESH_KEY)
}

export { getRememberMe }

// rememberLastMethod stores {method, email, ts} so the next /signin can show
// the hint. Called at every successful auth-state install (login, signup,
// OAuth adopt, magic-link adopt).
export function rememberLastMethod(method, email) {
  if (!method) return
  try {
    localStorage.setItem(LAST_METHOD_KEY, JSON.stringify({
      method, email: email || '', ts: Date.now(),
    }))
  } catch { /* localStorage full / disabled — silently skip */ }
}

// getLastMethod returns {method, email, ts} if recorded within 30 days, else null.
export function getLastMethod() {
  try {
    const raw = localStorage.getItem(LAST_METHOD_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (!parsed?.method || !parsed?.ts) return null
    if (Date.now() - parsed.ts > LAST_METHOD_TTL_MS) return null
    return parsed
  } catch {
    return null
  }
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
      // Preserve the user's remember-me choice across rotation — if they
      // signed in with the box unchecked, the new refresh stays in
      // sessionStorage too.
      setRefreshToken(data.refresh_token, getRememberMe())
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

  async function login(email, password, remember = true) {
    const data = await api.post('/auth/login', { email, password })
    // 2FA gate — caller must hand pre_2fa_token to /2fa/verify/*. Do NOT
    // install any tokens here; the user isn't authenticated yet.
    if (data?.requires_2fa) return data
    const access = data.access_token || data.token
    if (access) {
      setToken(access)
      scheduleRefresh(access)
    }
    if (data.refresh_token) setRefreshToken(data.refresh_token, remember)
    setUser(data.user)
    rememberLastMethod('email', email)
    return data
  }

  async function signup(email, password, companyName, remember = true) {
    const data = await api.post('/auth/signup', { email, password, companyName })
    setToken(data.access_token || data.token)
    if (data.refresh_token) setRefreshToken(data.refresh_token, remember)
    setUser(data.user)
    scheduleRefresh(data.access_token || data.token)
    rememberLastMethod('email', email)
    return data
  }

  // adoptSession takes a server auth response (from Google/Telegram OAuth /
  // magic-link / invite accept) and installs the tokens + user state exactly
  // as login() would have. `method` is one of 'email' | 'google' | 'telegram'
  // — feeds the 15a "last signed in with X" hint on /signin.
  //
  // OAuth/magic-link flows can't show a "remember me" checkbox (we're already
  // mid-redirect), so they inherit whatever the user picked the last time on
  // /signin via getRememberMe(). Default = true for first-ever OAuth users.
  function adoptSession(data, method) {
    const access = data.access_token || data.token
    setToken(access)
    if (data.refresh_token) setRefreshToken(data.refresh_token, getRememberMe())
    setUser(data.user)
    scheduleRefresh(access)
    if (method) rememberLastMethod(method, data?.user?.email)
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
