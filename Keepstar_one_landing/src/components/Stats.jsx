import React from 'react';
import Confetti from './Confetti';
import './Sections.css';

export default function Stats() {
  const stats = [
    { num: '6x', label: 'Higher conversion rate', color: '#1A73E8' },
    { num: '100%', label: 'Visitors enter a funnel', color: '#34A853' },
    { num: '<1s', label: 'Page generation time', color: '#EA4335' },
    { num: '1', label: 'Line of code to integrate', color: '#1A1A1A' }
  ];

  return (
    <section className="stats-section" style={{ position: 'relative', overflow: 'hidden' }}>
      <Confetti seed={60} count={8} opacity={0.4} />
      <div className="container stats-row">
        {stats.map((s, idx) => (
          <div key={idx} className="stat-card">
            <div className="stat-num" style={{ color: s.color }}>{s.num}</div>
            <div className="stat-label">{s.label}</div>
          </div>
        ))}
      </div>
    </section>
  );
}
