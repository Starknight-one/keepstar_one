import React from 'react';
import './Sections.css';

export default function Stats() {
  const stats = [
    { num: '+20%', label: 'Avg. conversion lift (pilot data)', color: '#185FA5' },
    { num: '~3s', label: 'Visual response — faster than filters', color: '#5BA3D0' },
    { num: '1 line', label: 'Of code to integrate', color: '#0B1B2E' },
    { num: '5 min', label: 'From signup to live', color: '#0B1B2E' }
  ];

  return (
    <section className="stats-section">
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
