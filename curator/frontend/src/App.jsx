// App — curator shell + routing.
//
// 2026-05-15 rebuild: removed Candidates / Junk / MergeReport pages —
// their backing tables (master_attribute_candidates, junk_candidates,
// merge_reports) were dropped in the catalog flow rebuild. The page logic
// is gone from the React Router, the sidebar links are gone, and the
// backend handlers no longer exist either.
//
// New tabs surface inside TenantDetailPage: Catalog / Inbox / Mapping /
// Action Log / Agent Runs.

import { useEffect, useState } from 'react'
import { Routes, Route, NavLink, Navigate, useNavigate } from 'react-router-dom'
import { api } from './api.js'
import LoginPage from './pages/LoginPage.jsx'
import AuditPage from './pages/AuditPage.jsx'
import TenantsPage from './pages/TenantsPage.jsx'
import TenantDetailPage from './pages/TenantDetailPage.jsx'
import MasterCatalogPage from './pages/MasterCatalogPage.jsx'
import MasterDetailPage from './pages/MasterDetailPage.jsx'
import PendingPage from './pages/PendingPage.jsx'
import ChatsPage from './pages/ChatsPage.jsx'
import ChatDetailPage from './pages/ChatDetailPage.jsx'

function Layout({ user, onLogout, children }) {
  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="brand">Curator</div>
        <nav>
          <div className="nav-section">Operations</div>
          <NavLink to="/tenants" className={({ isActive }) => isActive ? 'active' : ''}>Tenants</NavLink>
          <NavLink to="/master" className={({ isActive }) => isActive ? 'active' : ''}>Master Catalog</NavLink>
          <NavLink to="/pending" className={({ isActive }) => isActive ? 'active' : ''}>Pending Approval</NavLink>

          <div className="nav-section">Activity</div>
          <NavLink to="/audit" className={({ isActive }) => isActive ? 'active' : ''}>Audit</NavLink>

          <div className="nav-section">Tracing</div>
          <NavLink to="/chats" className={({ isActive }) => isActive ? 'active' : ''}>Chat Sessions</NavLink>
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
        <Route path="/master" element={<MasterCatalogPage />} />
        <Route path="/master/:id" element={<MasterDetailPage />} />
        <Route path="/pending" element={<PendingPage />} />
        <Route path="/audit" element={<AuditPage />} />
        <Route path="/chats" element={<ChatsPage />} />
        <Route path="/chats/:sessionId" element={<ChatDetailPage />} />
        <Route path="/login" element={<Navigate to="/tenants" replace />} />
        <Route path="*" element={<Navigate to="/tenants" replace />} />
      </Routes>
    </Layout>
  )
}
