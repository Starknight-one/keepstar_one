import React, { useState, useEffect } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { api } from './api';
import Sidebar from './components/Sidebar';
import Login from './components/Login';
import PostsTable from './components/PostsTable';
import PostEditor from './components/PostEditor';
import Analytics from './components/Analytics';
import Settings from './components/Settings';
import './App.css';

export default function App() {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.me().then((data) => setUser(data.user)).catch(() => setUser(null)).finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="admin-loading">Loading...</div>;

  if (!user) {
    return (
      <Routes>
        <Route path="/login" element={<Login onLogin={setUser} />} />
        <Route path="*" element={<Navigate to="/login" />} />
      </Routes>
    );
  }

  return (
    <div className="admin-layout">
      <Sidebar user={user} onLogout={() => { api.logout(); setUser(null); }} />
      <main className="admin-main">
        <Routes>
          <Route path="/" element={<Navigate to="/posts" />} />
          <Route path="/posts" element={<PostsTable />} />
          <Route path="/posts/new" element={<PostEditor />} />
          <Route path="/posts/:id/edit" element={<PostEditor />} />
          <Route path="/analytics" element={<Analytics />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/posts" />} />
        </Routes>
      </main>
    </div>
  );
}
