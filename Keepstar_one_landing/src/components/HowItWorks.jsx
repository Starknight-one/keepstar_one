import React from 'react';
import './Sections.css';

export default function HowItWorks() {
  const steps = [
    { num: '1', title: 'Add the script', desc: "Paste one line of code to your website. That's it — Keepstar widget appears instantly.", code: '<script src="keepstar.js">' },
    { num: '2', title: 'Upload your catalog', desc: "Import your products via JSON, CSV, or connect your existing store. AI enriches and indexes everything automatically." },
    { num: '3', title: 'Watch conversions grow', desc: "Every visitor enters a personalized funnel. Track engagement, analyze conversations, and see revenue impact in real time." }
  ];

  return (
    <section className="how-section container">
      <h2 className="how-title">Live in 5 minutes.<br/>No code required.</h2>
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
