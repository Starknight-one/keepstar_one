import React, { useState, useEffect } from 'react';
import { Key, Trash2, UserPlus } from 'lucide-react';
import { api } from '../api';

export default function Settings() {
  const [users, setUsers] = useState([]);
  const [newEmail, setNewEmail] = useState('');
  const [newPassword, setNewPassword] = useState('');

  useEffect(() => {
    api.getUsers().then(setUsers).catch(console.error);
  }, []);

  const handleAdd = async (e) => {
    e.preventDefault();
    if (!newEmail || !newPassword) return;
    try {
      const user = await api.createUser(newEmail, newPassword);
      setUsers((prev) => [{ ...user, created_at: new Date().toISOString() }, ...prev]);
      setNewEmail('');
      setNewPassword('');
    } catch (err) {
      alert(err.message);
    }
  };

  const handleChangePassword = async (id) => {
    const password = prompt('Enter new password:');
    if (!password) return;
    try {
      await api.changePassword(id, password);
      alert('Password updated');
    } catch (err) {
      alert(err.message);
    }
  };

  const handleDelete = async (id) => {
    if (!confirm('Delete this user?')) return;
    try {
      await api.deleteUser(id);
      setUsers((prev) => prev.filter((u) => u.id !== id));
    } catch (err) {
      alert(err.message);
    }
  };

  return (
    <>
      <div className="page-header">
        <h1 className="page-title">Settings</h1>
      </div>

      <h2 style={{ fontSize: 18, fontWeight: 600, marginBottom: 20 }}>Admin Users</h2>

      <div className="users-list">
        {users.map((user) => (
          <div key={user.id} className="user-row">
            <div className="user-info">
              <span className="user-email">{user.email}</span>
              <span className="user-date">Added {new Date(user.created_at).toLocaleDateString()}</span>
            </div>
            <div className="user-actions">
              <button className="btn btn-secondary btn-sm" onClick={() => handleChangePassword(user.id)} title="Change password">
                <Key size={14} />
              </button>
              <button className="btn btn-danger btn-sm" onClick={() => handleDelete(user.id)} title="Delete user">
                <Trash2 size={14} />
              </button>
            </div>
          </div>
        ))}
      </div>

      <form className="add-user-form" onSubmit={handleAdd}>
        <div className="field">
          <label>Email</label>
          <input type="email" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} placeholder="new-admin@keepstar.one" required />
        </div>
        <div className="field">
          <label>Password</label>
          <input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} placeholder="Min 6 characters" required />
        </div>
        <button type="submit" className="btn btn-primary" style={{ height: 40 }}>
          <UserPlus size={16} /> Add User
        </button>
      </form>
    </>
  );
}
