import React from 'react';
import Confetti from './Confetti';
import './Sections.css';

export default function HowItWorks() {
  const steps = [
    { num: '1', title: 'Add the script', desc: "Paste one line of code to your website. That's it — Keepstar One widget appears instantly.", code: '<script src="keepstar.js">' },
    { num: '2', title: 'Upload your catalog', desc: "JSON, CSV, or direct store sync. AI extracts attributes — skin type, sizes, ingredients — and builds a search index." },
    { num: '3', title: 'Track results in the dashboard', desc: "Every visitor enters a personalized path. See engagement, conversations, and revenue impact in real time." }
  ];

  return (
    <section className="how-section container" style={{ position: 'relative', overflow: 'hidden' }}>
      <Confetti seed={50} count={8} opacity={0.35} />
      <h2 className="how-title">Three steps from signup<br/>to your first personalized page</h2>
      <div className="how-steps">
        {steps.map((s, idx) => (
          <div key={idx} className="how-step">
            <div className="how-num">{s.num}</div>
            <h3 className="how-step-title">{s.title}</h3>
            <p className="how-step-desc">{s.desc}</p>
            {s.code && (
              <div className="code-block">
                <code>{s.code}</code>
              </div>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}
