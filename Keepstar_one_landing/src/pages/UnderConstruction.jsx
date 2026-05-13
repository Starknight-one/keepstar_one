import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowRight, X, CheckCircle2 } from 'lucide-react';
import './UnderConstruction.css';

const shapes = [
  { type: 'ellipse', color: 'bg-blue',   w: 6,  top: '15%', left: '12%' },
  { type: 'ellipse', color: 'bg-red',    w: 4,  top: '10%', left: '24%' },
  { type: 'ellipse', color: 'bg-yellow', w: 5,  top: '20%', left: '36%' },
  { type: 'ellipse', color: 'bg-blue',   w: 3,  top: '8%',  left: '48%' },
  { type: 'ellipse', color: 'bg-green',  w: 7,  top: '18%', left: '62%' },
  { type: 'ellipse', color: 'bg-red',    w: 4,  top: '11%', left: '76%' },
  { type: 'ellipse', color: 'bg-blue',   w: 5,  top: '23%', left: '86%' },
  { type: 'rect',    color: 'bg-red',    w: 8,  h: 3, top: '25%', left: '19%', rot: 'rotate(25deg)' },
  { type: 'rect',    color: 'bg-blue',   w: 7,  h: 3, top: '13%', left: '41%', rot: 'rotate(-15deg)' },
  { type: 'rect',    color: 'bg-yellow', w: 9,  h: 3, top: '6%',  left: '57%', rot: 'rotate(40deg)' },
  { type: 'rect',    color: 'bg-green',  w: 6,  h: 3, top: '25%', left: '72%', rot: 'rotate(-30deg)' },
  { type: 'ellipse', color: 'bg-yellow', w: 4,  top: '70%', left: '10%' },
  { type: 'ellipse', color: 'bg-green',  w: 5,  top: '80%', left: '27%' },
  { type: 'rect',    color: 'bg-blue',   w: 8,  h: 3, top: '76%', left: '70%', rot: 'rotate(20deg)' },
  { type: 'ellipse', color: 'bg-red',    w: 6,  top: '68%', left: '86%' },
  { type: 'rect',    color: 'bg-yellow', w: 7,  h: 3, top: '86%', left: '14%', rot: 'rotate(-25deg)' },
  { type: 'ellipse', color: 'bg-green',  w: 3,  top: '52%', left: '90%' },
  { type: 'rect',    color: 'bg-red',    w: 10, h: 3, top: '48%', left: '6%',  rot: 'rotate(55deg)' },
];

function SuccessModal({ onClose }) {
  return (
    <div className="uc-modal-overlay" onClick={onClose}>
      <div className="uc-modal-card" onClick={(e) => e.stopPropagation()}>
        <button className="uc-modal-close" onClick={onClose} aria-label="Close">
          <X size={18} />
        </button>
        <div className="uc-modal-icon">
          <CheckCircle2 size={40} strokeWidth={1.5} />
        </div>
        <h2 className="uc-modal-title">You're on the list</h2>
        <p className="uc-modal-subtitle">
          Thanks! We'll reach out as soon as early access opens up.
        </p>
        <button className="uc-modal-btn" onClick={onClose}>
          Close
        </button>
      </div>
    </div>
  );
}

export default function UnderConstruction() {
  const [email, setEmail] = useState('');
  const [showSuccess, setShowSuccess] = useState(false);

  const handleSubmit = (e) => {
    e.preventDefault();
    setShowSuccess(true);
    setEmail('');
  };

  return (
    <div className="uc-page">
      <header className="uc-header">
        <Link to="/" className="uc-logo">
          <img src="/logo-mark-h.svg" alt="Keepstar" className="uc-logo-icon" />
          <span className="uc-logo-text">Keepstar One</span>
        </Link>
        <a href="mailto:startknight@keepstar.one" className="uc-contact">
          startknight@keepstar.one
        </a>
      </header>

      <section className="uc-hero">
        {shapes.map((s, i) => (
          <div
            key={i}
            className={`uc-shape ${s.type === 'ellipse' ? 'uc-shape-ellipse' : 'uc-shape-rect'} ${s.color}`}
            style={{
              top: s.top,
              left: s.left,
              width: s.w,
              height: s.h || s.w,
              transform: s.rot || 'none',
            }}
          />
        ))}

        <div className="uc-hero-content">
          <div className="uc-badge">
            <span className="uc-badge-dot" />
            In development · Coming soon
          </div>

          <h1 className="uc-headline">
            Something great<br />is on the way
          </h1>

          <p className="uc-subhead">
            Keepstar is an AI chat that generates fully personalized pages in seconds.<br />
            We're putting the finishing touches on a product that turns every visitor<br />
            into a conversion-focused conversation — instead of a static website.
          </p>

          <form className="uc-cta-row" onSubmit={handleSubmit}>
            <input
              type="email"
              required
              placeholder="your@email.com"
              className="uc-email-input"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <button type="submit" className="uc-notify-btn">
              Notify me
              <ArrowRight size={16} />
            </button>
          </form>

          <span className="uc-status">Expected launch — Q2 2026</span>
        </div>
      </section>

      <footer className="uc-footer">
        <span className="uc-copyright">
          &copy; {new Date().getFullYear()} Keepstar One. All rights reserved.
        </span>
      </footer>

      {showSuccess && <SuccessModal onClose={() => setShowSuccess(false)} />}
    </div>
  );
}
