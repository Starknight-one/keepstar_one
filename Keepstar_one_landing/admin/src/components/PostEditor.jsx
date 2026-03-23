import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Save, ArrowLeft } from 'lucide-react';
import { api } from '../api';

const empty = { title: '', slug: '', excerpt: '', content: '', category: 'General', status: 'draft', cover_image: '', meta_description: '' };

export default function PostEditor() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [form, setForm] = useState(empty);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (id) {
      api.getPosts().then((posts) => {
        const post = posts.find((p) => p.id === Number(id));
        if (post) setForm(post);
      });
    }
  }, [id]);

  const handleChange = (field) => (e) => {
    const value = e.target.value;
    setForm((prev) => ({
      ...prev,
      [field]: value,
      ...(field === 'title' && !id ? { slug: value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '') } : {}),
    }));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setSaving(true);
    try {
      if (id) {
        await api.updatePost(id, form);
      } else {
        await api.createPost(form);
      }
      navigate('/posts');
    } catch (err) {
      alert(err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <div className="page-header">
        <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
          <button className="btn btn-secondary btn-sm" onClick={() => navigate('/posts')}>
            <ArrowLeft size={16} />
          </button>
          <h1 className="page-title">{id ? 'Edit Post' : 'New Post'}</h1>
        </div>
      </div>

      <form className="editor-container" onSubmit={handleSubmit}>
        <div className="editor-grid">
          <div className="field">
            <label>Title</label>
            <input value={form.title} onChange={handleChange('title')} placeholder="Post title" required />
          </div>

          <div className="editor-row">
            <div className="field">
              <label>Slug</label>
              <input value={form.slug} onChange={handleChange('slug')} placeholder="post-url-slug" required />
            </div>
            <div className="field">
              <label>Category</label>
              <select value={form.category} onChange={handleChange('category')}>
                <option>General</option>
                <option>Case Study</option>
                <option>Engineering</option>
                <option>Product</option>
                <option>Tutorial</option>
              </select>
            </div>
          </div>

          <div className="field">
            <label>Excerpt</label>
            <input value={form.excerpt} onChange={handleChange('excerpt')} placeholder="Short description for card preview" />
          </div>

          <div className="field">
            <label>Content (Markdown)</label>
            <textarea value={form.content} onChange={handleChange('content')} placeholder="Write your post content here..." style={{ minHeight: 300 }} />
          </div>

          <div className="editor-row">
            <div className="field">
              <label>Cover Image (CSS gradient or URL)</label>
              <input value={form.cover_image} onChange={handleChange('cover_image')} placeholder="linear-gradient(135deg, #4285F4, #1A73E8)" />
            </div>
            <div className="field">
              <label>Status</label>
              <select value={form.status} onChange={handleChange('status')}>
                <option value="draft">Draft</option>
                <option value="published">Published</option>
              </select>
            </div>
          </div>

          <div className="field">
            <label>Meta Description (SEO)</label>
            <input value={form.meta_description} onChange={handleChange('meta_description')} placeholder="Description for search engines" />
          </div>

          <div className="editor-actions">
            <button type="submit" className="btn btn-primary" disabled={saving}>
              <Save size={16} /> {saving ? 'Saving...' : 'Save Post'}
            </button>
            <button type="button" className="btn btn-secondary" onClick={() => navigate('/posts')}>Cancel</button>
          </div>
        </div>
      </form>
    </>
  );
}
