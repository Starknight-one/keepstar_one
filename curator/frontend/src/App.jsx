import { useEffect, useState } from 'react'
import { Routes, Route, NavLink, Navigate, useNavigate, useLocation } from 'react-router-dom'
import { api } from './api.js'
import LoginPage from './pages/LoginPage.jsx'
import CandidatesPage from './pages/CandidatesPage.jsx'
import JunkPage from './pages/JunkPage.jsx'
import AuditPage from './pages/AuditPage.jsx'

function Layout({ user, onLogout, children }) {
  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="brand">Curator</div>
        <nav>
          <NavLink to="/candidates" className={({ isActive }) => isActive ? 'active' : ''}>Candidates</NavLink>
          <NavLink to="/junk" className={({ isActive }) => isActive ? 'active' : ''}>Junk</NavLink>
          <NavLink to="/audit" className={({ isActive }) => isActive ? 'active' : ''}>Audit</NavLink>
        </nav>
        <div className="footer">
          <div>{user?.email}</div>
          <button onClick={onLogout}>Logout</button>
        </div>
      </aside>
      <main>{children}</main>
    </div>
  )
}

export default function App() {
  const [user, setUser] = useState(null)
  const [loaded, setLoaded] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    api.get('/curator/me')
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setLoaded(true))
  }, [location.pathname])

  async function handleLogout() {
    try { await api.post('/curator/auth/logout') } catch {}
    setUser(null)
    navigate('/login')
  }

  if (!loaded) return null

  if (!user) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage onLogin={(u) => { setUser(u); navigate('/candidates') }} />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    )
  }

  return (
    <Layout user={user} onLogout={handleLogout}>
      <Routes>
        <Route index element={<Navigate to="/candidates" replace />} />
        <Route path="/candidates" element={<CandidatesPage />} />
        <Route path="/junk" element={<JunkPage />} />
        <Route path="/audit" element={<AuditPage />} />
        <Route path="/login" element={<Navigate to="/candidates" replace />} />
        <Route path="*" element={<Navigate to="/candidates" replace />} />
      </Routes>
    </Layout>
  )
}
