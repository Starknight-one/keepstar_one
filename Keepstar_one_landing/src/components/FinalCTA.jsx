import React from 'react';
import './Sections.css';

export default function FinalCTA({ onDemo }) {
  return (
    <section className="final-cta-section container">
      <h2 className="cta-title">Ready to turn every visitor<br/>into a conversion?</h2>
      <p className="cta-desc">Stop losing customers to static pages. Start your free trial and see<br/>how Keepstar transforms your website into a personalized experience engine.</p>
      <div className="cta-actions">
        <button className="btn-hero btn-hero-primary">Start free trial</button>
        <button className="btn-hero btn-hero-secondary" onClick={onDemo}>Book a demo</button>
      </div>
      <p className="cta-note">14-day free trial. No credit card required.</p>
    </section>
  );
}
