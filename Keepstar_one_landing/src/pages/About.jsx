import React, { useEffect } from 'react';
import { Zap, Users, Shield } from 'lucide-react';
import Header from '../components/Header';
import Footer from '../components/Footer';
import './StaticPage.css';

const confetti = [
  { type: 'ellipse', color: 'bg-blue', w: 5, top: '24%', left: '6%', delay: '' },
  { type: 'ellipse', color: 'bg-red', w: 4, top: '10%', left: '20%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-yellow', w: 8, h: 3, top: '31%', left: '33%', rot: '-20deg', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-green', w: 6, top: '17%', left: '48%', delay: '' },
  { type: 'ellipse', color: 'bg-blue', w: 3, top: '34%', left: '62%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-red', w: 7, h: 3, top: '10%', left: '75%', rot: '40deg', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-yellow', w: 5, top: '28%', left: '88%', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 4, top: '18%', left: '98%', delay: '' },
  { type: 'rect', color: 'bg-blue', w: 9, h: 3, top: '83%', left: '13%', rot: '30deg', delay: 'delay-100' },
  { type: 'ellipse', color: 'bg-yellow', w: 5, top: '90%', left: '40%', delay: 'delay-200' },
  { type: 'rect', color: 'bg-red', w: 6, h: 3, top: '79%', left: '64%', rot: '-45deg', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 4, top: '86%', left: '86%', delay: '' },
];

const values = [
  { icon: Zap, title: 'Innovation', desc: 'We push the boundaries of what\'s possible with AI and web technology.' },
  { icon: Users, title: 'User-First', desc: 'Every decision we make starts with the end user experience in mind.' },
  { icon: Shield, title: 'Trust', desc: 'Transparency and data privacy are at the core of everything we build.' },
];

export default function About() {
  useEffect(() => { window.scrollTo(0, 0); }, []);

  return (
    <div className="app-container">
      <Header />
      <section className="page-hero">
        {confetti.map((s, i) => (
          <div key={i} className={`floating-shape ${s.type === 'ellipse' ? 'shape-ellipse' : 'shape-rect'} ${s.color} ${s.delay}`}
            style={{ top: s.top, left: s.left, width: s.w, height: s.h || s.w, opacity: 0.6, transform: s.rot ? `rotate(${s.rot})` : undefined }} />
        ))}
        <h1>About Us</h1>
        <p className="page-subtitle">The story behind Keepstar One and the people who build it.</p>
      </section>

      <div className="story-section">
        <div className="story-photo">
          <div style={{ width: '100%', height: '100%', background: 'linear-gradient(135deg, #C084FC 0%, #818CF8 50%, #60A5FA 100%)' }} />
        </div>
        <div className="story-text">
          <span className="story-label">Our Story</span>
          <h2>Building the future of personalized web</h2>
          <p>Your story goes here. Tell visitors about your journey — how you started, what drives you, what problems you saw and how Keepstar One was born to solve them.</p>
          <p>Share your vision, your values, and what makes your approach unique. This is your space to connect with people on a personal level.</p>
          <p>Replace this placeholder text with your real story when you're ready.</p>
        </div>
      </div>

      <div className="mission-section">
        <span className="mission-label">OUR MISSION</span>
        <h2>Every visitor deserves a website<br />that speaks directly to them.</h2>
        <div className="values-row">
          {values.map((v, i) => (
            <div key={i} className="value-card">
              <div className="value-icon-wrap">
                <v.icon size={22} />
              </div>
              <h3>{v.title}</h3>
              <p>{v.desc}</p>
            </div>
          ))}
        </div>
      </div>

      <Footer />
    </div>
  );
}
