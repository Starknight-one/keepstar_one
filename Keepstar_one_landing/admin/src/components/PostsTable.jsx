import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Plus, Pencil, Trash2 } from 'lucide-react';
import { api } from '../api';

export default function PostsTable() {
  const [posts, setPosts] = useState([]);
  const [search, setSearch] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    api.getPosts().then(setPosts).catch(console.error);
  }, []);

  const handleDelete = async (id) => {
    if (!confirm('Delete this post?')) return;
    await api.deletePost(id);
    setPosts((prev) => prev.filter((p) => p.id !== id));
  };

  const filtered = posts.filter(
    (p) => p.title.toLowerCase().includes(search.toLowerCase()) || p.category.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <>
      <div className="page-header">
        <h1 className="page-title">Posts</h1>
        <button className="btn btn-primary" onClick={() => navigate('/posts/new')}>
          <Plus size={16} /> New Post
        </button>
      </div>

      <div className="search-bar">
        <input className="search-input" placeholder="Search posts..." value={search} onChange={(e) => setSearch(e.target.value)} />
      </div>

      {filtered.length === 0 ? (
        <div className="empty-state">
          <p>No posts yet. Create your first blog post.</p>
          <button className="btn btn-primary" onClick={() => navigate('/posts/new')}>
            <Plus size={16} /> New Post
          </button>
        </div>
      ) : (
        <div className="posts-table">
          <table>
            <thead>
              <tr>
                <th>Title</th>
                <th>Category</th>
                <th>Status</th>
                <th>Date</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((post) => (
                <tr key={post.id}>
                  <td className="post-title-cell">{post.title}</td>
                  <td>{post.category}</td>
                  <td>
                    <span className={`status-badge status-${post.status}`}>{post.status}</span>
                  </td>
                  <td>{new Date(post.created_at).toLocaleDateString()}</td>
                  <td>
                    <div className="table-actions">
                      <button className="btn btn-secondary btn-sm" onClick={() => navigate(`/posts/${post.id}/edit`)}>
                        <Pencil size={14} />
                      </button>
                      <button className="btn btn-danger btn-sm" onClick={() => handleDelete(post.id)}>
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
