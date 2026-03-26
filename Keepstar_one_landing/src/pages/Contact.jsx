import React, { useEffect } from 'react';
import { Send, Mail } from 'lucide-react';
import Header from '../components/Header';
import Footer from '../components/Footer';
import './StaticPage.css';

const confetti = [
  { type: 'ellipse', color: 'bg-blue', w: 6, top: '15%', left: '9%', delay: '' },
  { type: 'rect', color: 'bg-red', w: 7, h: 3, top: '30%', left: '23%', rot: '35deg', delay: 'delay-100' },
  { type: 'ellipse', color: 'bg-yellow', w: 5, top: '8%', left: '39%', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-green', w: 4, top: '34%', left: '53%', delay: '' },
  { type: 'rect', color: 'bg-blue', w: 8, h: 3, top: '15%', left: '67%', rot: '-25deg', delay: 'delay-100' },
  { type: 'ellipse', color: 'bg-yellow', w: 7, top: '26%', left: '80%', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-red', w: 3, top: '37%', left: '93%', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 5, top: '80%', left: '14%', delay: '' },
  { type: 'rect', color: 'bg-blue', w: 6, h: 3, top: '86%', left: '34%', rot: '50deg', delay: 'delay-100' },
  { type: 'ellipse', color: 'bg-yellow', w: 4, top: '83%', left: '59%', delay: 'delay-200' },
  { type: 'rect', color: 'bg-red', w: 9, h: 3, top: '72%', left: '75%', rot: '-40deg', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 5, top: '90%', left: '96%', delay: '' },
];

export default function Contact() {
  useEffect(() => { window.scrollTo(0, 0); }, []);

  return (
    <div className="app-container">
      <Header />
      <section className="page-hero">
        {confetti.map((s, i) => (
          <div key={i} className={`floating-shape ${s.type === 'ellipse' ? 'shape-ellipse' : 'shape-rect'} ${s.color} ${s.delay}`}
            style={{ top: s.top, left: s.left, width: s.w, height: s.h || s.w, opacity: 0.6, transform: s.rot ? `rotate(${s.rot})` : undefined }} />
        ))}
        <h1>Get in Touch</h1>
        <p className="page-subtitle">We'd love to hear from you. Reach out through any of the channels below.</p>
      </section>

      <div className="contact-cards">
        <div className="contact-card">
          <div className="contact-icon-wrap">
            <Send size={24} color="#fff" />
          </div>
          <h3>Telegram</h3>
          <p className="contact-desc">Scan the QR code below to chat with us on Telegram</p>
          <div className="contact-qr" />
        </div>

        <div className="contact-card">
          <div className="contact-icon-wrap">
            <Mail size={24} color="#fff" />
          </div>
          <h3>Email</h3>
          <p className="contact-desc">Drop us a line anytime and we'll get back to you within 24 hours</p>
          <a href="mailto:hello@keepstar.ai" className="contact-email-link">
            <Mail size={16} />
            hello@keepstar.ai
          </a>
        </div>
      </div>

      <Footer />
    </div>
  );
}
