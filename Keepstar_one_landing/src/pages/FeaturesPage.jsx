import React, { useEffect, useState } from 'react';
import { Search, PanelsTopLeft, MousePointer, Upload, Layers, ShieldCheck, Timer, Mic, MessageCircle, Palette } from 'lucide-react';
import Header from '../components/Header';
import Footer from '../components/Footer';
import DemoModal from '../components/DemoModal';
import './StaticPage.css';

const confetti = [
  { type: 'ellipse', color: 'bg-blue', w: 6, top: '17%', left: '7%', delay: '' },
  { type: 'ellipse', color: 'bg-red', w: 4, top: '28%', left: '23%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-yellow', w: 8, h: 3, top: '10%', left: '38%', rot: '30deg', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-green', w: 5, top: '24%', left: '54%', delay: '' },
  { type: 'ellipse', color: 'bg-blue', w: 3, top: '34%', left: '67%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-red', w: 7, h: 3, top: '14%', left: '80%', rot: '-25deg', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-yellow', w: 5, top: '28%', left: '93%', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-green', w: 4, top: '83%', left: '14%', delay: '' },
  { type: 'rect', color: 'bg-blue', w: 6, h: 3, top: '90%', left: '48%', rot: '45deg', delay: 'delay-100' },
  { type: 'ellipse', color: 'bg-yellow', w: 5, top: '79%', left: '75%', delay: 'delay-200' },
];

const features = [
  [
    { icon: Search, color: '#4285F4', bg: '#E8F0FE', title: 'Hybrid Search', desc: "SQL + vector search + RRF merge in a single query. Finds products not just by exact matches, but by meaning — 'something warm for winter' returns down jackets and thermals, not a page about winter." },
    { icon: PanelsTopLeft, color: '#34A853', bg: '#CEEAD6', title: 'Dynamic UI Generation', desc: 'AI builds a unique page layout in direct response to the user — product cards, comparisons, galleries. Not templates, but real-time visual generation tailored to each specific query.' },
    { icon: MousePointer, color: '#F59E0B', bg: '#FEEFC3', title: 'One-Click Integration', desc: 'Add a widget with a single line of code — and your website transforms into a personalized experience. No complex setup, migrations, or dependencies required.' },
  ],
  [
    { icon: Upload, color: '#34A853', bg: '#CEEAD6', title: 'Quick Catalog Import', desc: 'Import your product catalog in minutes — CSV, API, Shopify, any format. The more data you feed it, the smarter the personalization and search become.' },
    { icon: Layers, color: '#8B5CF6', bg: '#F3F0FF', title: 'Content Saturation', desc: 'Content overload as a strategy. The more product information you provide — specs, reviews, comparisons — the better the AI addresses buyer fears and objections.' },
    { icon: ShieldCheck, color: '#4285F4', bg: '#E8F0FE', title: 'Fewer Hallucinations', desc: 'The LLM searches your actual database instead of making things up. Retrieval-Augmented Generation minimizes hallucinations — every answer is backed by real catalog data.' },
  ],
  [
    { icon: Timer, color: '#34A853', bg: '#CEEAD6', title: '5-Second Visual Response', desc: 'Full visual response in 5 seconds or less. As simple as clicking a button and navigating to a page — except the page is already built for your exact query. Often even faster.' },
    { icon: Mic, color: '#EA4335', bg: '#FCE8E6', title: 'Voice Mode', desc: 'Talk to the store with your voice — ask questions, compare products, choose without a single tap. Currently in development, coming to production soon.', badge: { text: 'Coming soon', color: '#92400E', bg: '#FEEFC3' } },
  ],
  [
    { icon: MessageCircle, color: '#F59E0B', bg: '#FEEFC3', title: 'Chat Analytics', desc: 'All user chats are available for review. Learn what actually concerns buyers, what questions they ask, where they hesitate — and build your product on real data.' },
    { icon: Palette, color: '#8B5CF6', bg: '#F3F0FF', title: 'Custom Design System', desc: 'Upload your own design system or generate a new one from scratch. No limitations — buttons, cards, typography, colors fully matched to your brand.' },
  ],
];

export default function FeaturesPage() {
  const [showDemo, setShowDemo] = useState(false);
  useEffect(() => { window.scrollTo(0, 0); }, []);

  return (
    <div className="app-container">
      <Header onDemo={() => setShowDemo(true)} />
      <section className="page-hero">
        {confetti.map((s, i) => (
          <div key={i} className={`floating-shape ${s.type === 'ellipse' ? 'shape-ellipse' : 'shape-rect'} ${s.color} ${s.delay}`}
            style={{ top: s.top, left: s.left, width: s.w, height: s.h || s.w, opacity: 0.6, transform: s.rot ? `rotate(${s.rot})` : undefined }} />
        ))}
        <h1>Features</h1>
        <p className="page-subtitle">Everything you need to turn static pages into<br />personalized conversion engines.</p>
      </section>

      <div className="feature-grid">
        {features.map((row, ri) => (
          <div key={ri} className="feature-row">
            {row.map((f, fi) => (
              <div key={fi} className="feature-card">
                <div className="feature-icon-wrap" style={{ background: f.bg }}>
                  <f.icon size={24} color={f.color} />
                </div>
                <h3>{f.title}</h3>
                <p>{f.desc}</p>
                {f.badge && (
                  <span className="feature-badge" style={{ background: f.badge.bg, color: f.badge.color }}>
                    {f.badge.text}
                  </span>
                )}
              </div>
            ))}
          </div>
        ))}
      </div>

      <div className="features-cta">
        <h2>Ready to see it in action?</h2>
        <p>Start your free 14-day trial. No credit card required.</p>
        <button className="btn-get-started" onClick={() => setShowDemo(true)}>Start free trial</button>
      </div>

      <Footer />
      {showDemo && <DemoModal onClose={() => setShowDemo(false)} />}
    </div>
  );
}
