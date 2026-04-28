import { useEffect, useState } from 'react'
import { Routes, Route, NavLink, Navigate, useNavigate } from 'react-router-dom'
import { api } from './api.js'
import LoginPage from './pages/LoginPage.jsx'
import CandidatesPage from './pages/CandidatesPage.jsx'
import JunkPage from './pages/JunkPage.jsx'
import AuditPage from './pages/AuditPage.jsx'
import TenantsPage from './pages/TenantsPage.jsx'
import TenantDetailPage from './pages/TenantDetailPage.jsx'
import MasterCatalogPage from './pages/MasterCatalogPage.jsx'
import MasterDetailPage from './pages/MasterDetailPage.jsx'
import MergeReportPage from './pages/MergeReportPage.jsx'

function Layout({ user, onLogout, children }) {
  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="brand">Curator</div>
        <nav>
          <div className="nav-section">Operations</div>
          <NavLink to="/tenants" className={({ isActive }) => isActive ? 'active' : ''}>Tenants</NavLink>
          <NavLink to="/master" className={({ isActive }) => isActive ? 'active' : ''}>Master Catalog</NavLink>

          <div className="nav-section">Curation queues</div>
          <NavLink to="/candidates" className={({ isActive }) => isActive ? 'active' : ''}>Candidates</NavLink>
          <NavLink to="/junk" className={({ isActive }) => isActive ? 'active' : ''}>Junk</NavLink>

          <div className="nav-section">Activity</div>
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

  // One-shot auth check on mount. The /curator/me 401 case is handled by
  // setUser(null) → render LoginPage; api.js no longer does a hard redirect.
  useEffect(() => {
    api.get('/curator/me')
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setLoaded(true))
  }, [])

  async function handleLogout() {
    try { await api.post('/curator/auth/logout') } catch {}
    setUser(null)
    navigate('/login')
  }

  if (!loaded) return null

  if (!user) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage onLogin={(u) => { setUser(u); navigate('/tenants') }} />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    )
  }

  return (
    <Layout user={user} onLogout={handleLogout}>
      <Routes>
        <Route index element={<Navigate to="/tenants" replace />} />
        <Route path="/tenants" element={<TenantsPage />} />
        <Route path="/tenants/:id" element={<TenantDetailPage />} />
        <Route path="/tenants/:tenantId/merge-reports/:reportId" element={<MergeReportPage />} />
        <Route path="/master" element={<MasterCatalogPage />} />
        <Route path="/master/:id" element={<MasterDetailPage />} />
        <Route path="/candidates" element={<CandidatesPage />} />
        <Route path="/junk" element={<JunkPage />} />
        <Route path="/audit" element={<AuditPage />} />
        <Route path="/login" element={<Navigate to="/tenants" replace />} />
        <Route path="*" element={<Navigate to="/tenants" replace />} />
      </Routes>
    </Layout>
  )
}
