import React, { useEffect } from 'react';
import Header from '../components/Header';
import Footer from '../components/Footer';
import './StaticPage.css';

const confetti = [
  { type: 'ellipse', color: 'bg-blue', w: 5, top: '14%', left: '8%', delay: '' },
  { type: 'ellipse', color: 'bg-red', w: 4, top: '31%', left: '25%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-yellow', w: 7, h: 3, top: '10%', left: '42%', rot: '35deg', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-green', w: 6, top: '28%', left: '60%', delay: '' },
  { type: 'ellipse', color: 'bg-blue', w: 3, top: '17%', left: '75%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-red', w: 8, h: 3, top: '34%', left: '89%', rot: '-30deg', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-yellow', w: 4, top: '83%', left: '14%', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 5, top: '90%', left: '53%', delay: '' },
  { type: 'rect', color: 'bg-blue', w: 6, h: 3, top: '76%', left: '78%', rot: '50deg', delay: 'delay-100' },
];

const entries = [
  {
    badge: { text: 'New', color: '#166534', bg: '#CEEAD6' },
    date: 'March 20, 2026',
    title: 'Personalization Engine v3.0',
    desc: 'Completely rebuilt personalization engine with 3x faster response times. New context-aware layout generation that adapts to visitor device, location, and browsing history. Added support for multi-language content generation.',
  },
  {
    badge: { text: 'Improvement', color: '#1A56DB', bg: '#E8F0FE' },
    date: 'March 10, 2026',
    title: 'Analytics Dashboard Redesign',
    desc: 'Redesigned analytics dashboard with real-time conversion tracking, visitor journey visualization, and AI performance metrics. New export capabilities for CSV and PDF reports.',
  },
  {
    badge: { text: 'Fix', color: '#991B1B', bg: '#FCE8E6' },
    date: 'February 28, 2026',
    title: 'Mobile Rendering Fixes',
    desc: "Fixed layout issues on iOS Safari where personalized cards would overlap. Resolved a bug where image carousels wouldn't load on slower connections. Improved touch interaction responsiveness across all generated layouts.",
  },
  {
    badge: { text: 'New', color: '#166534', bg: '#CEEAD6' },
    date: 'February 15, 2026',
    title: 'Shopify Integration',
    desc: 'One-click Shopify integration is now live. Automatically sync your product catalog, inventory, and customer data. Keepstar One generates personalized storefronts from your existing Shopify data with zero configuration.',
  },
];

export default function ChangelogPage() {
  useEffect(() => { window.scrollTo(0, 0); }, []);

  return (
    <div className="app-container">
      <Header />
      <section className="page-hero">
        {confetti.map((s, i) => (
          <div key={i} className={`floating-shape ${s.type === 'ellipse' ? 'shape-ellipse' : 'shape-rect'} ${s.color} ${s.delay}`}
            style={{ top: s.top, left: s.left, width: s.w, height: s.h || s.w, opacity: 0.6, transform: s.rot ? `rotate(${s.rot})` : undefined }} />
        ))}
        <h1>Changelog</h1>
        <p className="page-subtitle">Stay up to date with the latest improvements and new features.</p>
      </section>

      <div className="changelog-body">
        {entries.map((e, i) => (
          <React.Fragment key={i}>
            <div className="changelog-entry">
              <div className="changelog-header">
                <span className="changelog-badge" style={{ background: e.badge.bg, color: e.badge.color }}>
                  {e.badge.text}
                </span>
                <span className="changelog-date">{e.date}</span>
              </div>
              <h3>{e.title}</h3>
              <p>{e.desc}</p>
            </div>
            {i < entries.length - 1 && <div className="changelog-divider" />}
          </React.Fragment>
        ))}
      </div>

      <Footer />
    </div>
  );
}
