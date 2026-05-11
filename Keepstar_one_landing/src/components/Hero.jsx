import React from 'react';
import './Hero.css';

export default function Hero({ onDemo }) {
  return (
    <section className="hero-section">
      <div className="hero-content">
        <div className="badge">Used by e-commerce teams worldwide</div>

        <h1 className="hero-headline">
          Every visitor gets<br/>their own website
        </h1>

        <p className="hero-subhead">
          A visitor types &lsquo;red sneakers under $100&rsquo; — and sees a curated page<br/>
          with cards, comparisons, and reviews. Built from your catalog<br/>
          in under a second.
        </p>

        <div className="hero-actions">
          <button className="btn-hero btn-hero-primary" onClick={onDemo}>Request demo</button>
        </div>
      </div>
    </section>
  );
}
