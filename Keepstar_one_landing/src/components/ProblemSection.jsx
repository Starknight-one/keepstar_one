import React from 'react';
import Confetti from './Confetti';
import './Sections.css';

export default function ProblemSection() {
  const problems = [
    { num: '01', title: 'One size fits none', desc: "Every visitor has different needs, questions, and objections. Static pages can't adapt to individual contexts." },
    { num: '02', title: 'Information overload', desc: "You pack pages with content to address every doubt. But more info means more noise, and visitors leave overwhelmed." },
    { num: '03', title: 'No guided path', desc: "Visitors browse randomly, skip key pages, and leave without converting." }
  ];

  return (
    <section className="problem-section" style={{ position: 'relative', overflow: 'hidden' }}>
      <Confetti seed={40} count={8} opacity={0.3} />
      <div className="container">
        <div className="prob-header">
          <h2 className="prob-title">Your website shows one page<br/>to 10,000 different people</h2>
          <p className="prob-desc">A first-time visitor sees the same thing as a returning customer. The result: bounced sessions and lost revenue.</p>
        </div>
        <div className="prob-cards">
          {problems.map((p, idx) => (
            <div key={idx} className="prob-card">
              <div className="prob-num">{p.num}</div>
              <h3 className="prob-card-title">{p.title}</h3>
              <p className="prob-card-desc">{p.desc}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
