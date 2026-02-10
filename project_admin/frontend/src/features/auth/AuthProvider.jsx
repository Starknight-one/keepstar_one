import { createContext, useContext, useState, useEffect } from 'react'
import { api, setToken, clearToken } from '../../shared/api/apiClient.js'

const AuthContext = createContext(null)

export function useAuth() {
  return useContext(AuthContext)
}

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) {
      setLoading(false)
      return
    }
    api.get('/auth/me')
      .then(setUser)
      .catch(() => clearToken())
      .finally(() => setLoading(false))
  }, [])

  async function login(email, password) {
    const data = await api.post('/auth/login', { email, password })
    setToken(data.token)
    setUser(data.user)
    return data
  }

  async function signup(email, password, companyName) {
    const data = await api.post('/auth/signup', { email, password, companyName })
    setToken(data.token)
    setUser(data.user)
    return data
  }

  function logout() {
    clearToken()
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{ user, loading, login, signup, logout }}>
      {children}
    </AuthContext.Provider>
  )
}
