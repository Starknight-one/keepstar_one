import { Outlet, NavLink, useNavigate } from 'react-router-dom'
import { Package, Upload, Code, Settings, LogOut, Activity, MessageSquare, Palette } from 'lucide-react'
import { useAuth } from '../auth/AuthProvider.jsx'
import './layout.css'

export default function DashboardLayout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  function handleLogout() {
    logout()
    navigate('/login')
  }

  return (
    <div className="dashboard">
      <aside className="sidebar">
        <div className="sidebar-brand">Keepstar</div>
        <nav className="sidebar-nav">
          <NavLink to="/catalog" className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`}>
            <Package size={18} /> Catalog
          </NavLink>
          <NavLink to="/import" className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`}>
            <Upload size={18} /> Import
          </NavLink>
          <NavLink to="/widget" className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`}>
            <Code size={18} /> Widget
          </NavLink>
          <NavLink to="/conversations" className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`}>
            <MessageSquare size={18} /> Conversations
          </NavLink>
          <NavLink to="/canvas" className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`}>
            <Palette size={18} /> Canvas
          </NavLink>
          <NavLink to="/traces" className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`}>
            <Activity size={18} /> Traces
          </NavLink>
          <NavLink to="/settings" className={({ isActive }) => `sidebar-link ${isActive ? 'active' : ''}`}>
            <Settings size={18} /> Settings
          </NavLink>
        </nav>
        <div className="sidebar-footer">
          <div className="sidebar-user">{user?.email}</div>
          <button className="sidebar-logout" onClick={handleLogout}>
            <LogOut size={16} /> Logout
          </button>
        </div>
      </aside>
      <main className="main-content">
        <Outlet />
      </main>
    </div>
  )
}
