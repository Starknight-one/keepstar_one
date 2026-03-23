import React, { useState, useEffect } from 'react';
import { X, Send } from 'lucide-react';
import './DemoModal.css';

export default function DemoModal({ onClose }) {
  const [form, setForm] = useState({ name: '', email: '', company: '', message: '' });

  useEffect(() => {
    const handleEsc = (e) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handleEsc);
    return () => document.removeEventListener('keydown', handleEsc);
  }, [onClose]);

  const handleSubmit = (e) => {
    e.preventDefault();
    const apiUrl = import.meta.env.VITE_BLOG_API_URL || '';
    if (apiUrl) {
      fetch(`${apiUrl}/api/demo-request`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      }).catch(() => {});
    }
    onClose();
  };

  const handleChange = (field) => (e) => {
    setForm((prev) => ({ ...prev, [field]: e.target.value }));
  };

  return (
    <div className="modal-overlay" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card">
        <button className="modal-close" onClick={onClose}>
          <X size={20} />
        </button>

        <h2 className="modal-title">Book a Demo</h2>
        <p className="modal-subtitle">
          See how Keepstar One can transform your store's conversion rate.
        </p>

        <form className="modal-form" onSubmit={handleSubmit}>
          <div className="form-field">
            <label>Name</label>
            <input type="text" placeholder="John Smith" value={form.name} onChange={handleChange('name')} required />
          </div>
          <div className="form-field">
            <label>Work Email</label>
            <input type="email" placeholder="john@company.com" value={form.email} onChange={handleChange('email')} required />
          </div>
          <div className="form-field">
            <label>Company / Store URL</label>
            <input type="text" placeholder="https://yourstore.com" value={form.company} onChange={handleChange('company')} />
          </div>
          <div className="form-field">
            <label>Message</label>
            <textarea placeholder="Tell us about your store and goals..." value={form.message} onChange={handleChange('message')} />
          </div>
          <button type="submit" className="btn-submit-demo">
            <Send size={18} />
            Request Demo
          </button>
        </form>

        <p className="modal-privacy">We'll never share your info. Unsubscribe anytime.</p>
      </div>
    </div>
  );
}
