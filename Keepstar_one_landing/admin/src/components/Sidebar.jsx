import React from 'react';
import { NavLink } from 'react-router-dom';
import { FileText, BarChart3, Settings, LogOut } from 'lucide-react';

export default function Sidebar({ user, onLogout }) {
  return (
    <aside className="sidebar">
      <div className="sidebar-logo">Keepstar Admin</div>
      <nav className="sidebar-nav">
        <NavLink to="/posts" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`}>
          <FileText size={18} /> Posts
        </NavLink>
        <NavLink to="/analytics" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`}>
          <BarChart3 size={18} /> Analytics
        </NavLink>
        <NavLink to="/settings" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`}>
          <Settings size={18} /> Settings
        </NavLink>
      </nav>
      <div className="sidebar-footer">
        <div className="sidebar-user">{user.email}</div>
        <button className="sidebar-logout" onClick={onLogout}>
          <LogOut size={18} /> Log out
        </button>
      </div>
    </aside>
  );
}
