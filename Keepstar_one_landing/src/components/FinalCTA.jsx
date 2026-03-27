import React from 'react';
import Confetti from './Confetti';
import './Sections.css';

export default function FinalCTA({ onDemo }) {
  return (
    <section className="final-cta-section container" style={{ position: 'relative', overflow: 'hidden' }}>
      <Confetti seed={80} count={10} opacity={0.4} />
      <h2 className="cta-title">See how it works with your catalog</h2>
      <p className="cta-desc">Request a demo — we'll set it up with your actual products<br/>so you can see real results.</p>
      <div className="cta-actions">
        <button className="btn-hero btn-hero-primary" onClick={onDemo}>Request demo</button>
      </div>
    </section>
  );
}
