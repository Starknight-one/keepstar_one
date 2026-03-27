import React from 'react';
import Confetti from './Confetti';
import './Sections.css';

export default function Stats() {
  const stats = [
    { num: '+20%', label: 'Avg. conversion lift (pilot data)', color: '#1A73E8' },
    { num: '~3s', label: 'Visual response — faster than filters', color: '#EA4335' },
    { num: '1 line', label: 'Of code to integrate', color: '#34A853' },
    { num: '5 min', label: 'From signup to live', color: '#1A1A1A' }
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
