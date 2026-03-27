import React, { useEffect, useState } from 'react';
import { ShoppingBag, Sparkles, Store, Shirt, Coffee, Home } from 'lucide-react';
import Header from '../components/Header';
import Footer from '../components/Footer';
import DemoModal from '../components/DemoModal';
import './StaticPage.css';

const confetti = [
  { type: 'ellipse', color: 'bg-blue', w: 5, top: '24%', left: '6%', delay: '' },
  { type: 'ellipse', color: 'bg-red', w: 4, top: '10%', left: '20%', delay: 'delay-100' },
  { type: 'rect', color: 'bg-yellow', w: 8, h: 3, top: '31%', left: '33%', rot: '-20deg', delay: 'delay-200' },
  { type: 'ellipse', color: 'bg-green', w: 6, top: '14%', left: '50%', delay: '' },
  { type: 'rect', color: 'bg-blue', w: 7, h: 3, top: '28%', left: '68%', rot: '30deg', delay: 'delay-100' },
  { type: 'ellipse', color: 'bg-red', w: 4, top: '8%', left: '82%', delay: 'delay-300' },
  { type: 'ellipse', color: 'bg-yellow', w: 5, top: '22%', left: '92%', delay: 'delay-200' },
];

const useCases = [
  {
    icon: ShoppingBag,
    color: '#4285F4',
    bg: '#D2E3FC',
    title: 'E-Commerce Stores',
    desc: 'Turn product browsing into guided conversations. Every visitor gets personalized product recommendations, comparisons, and detailed views — assembled dynamically from your catalog. Reduce bounce rates, increase time on site, and convert more visitors into buyers.',
  },
  {
    icon: Sparkles,
    color: '#EA4335',
    bg: '#FCE8E6',
    title: 'Beauty & Cosmetics',
    desc: 'Help shoppers find the right products by skin type, concern, or ingredient preference. AI surfaces relevant items from thousands of SKUs in seconds. Perfect for stores with complex product attributes where traditional filters fall short.',
  },
  {
    icon: Store,
    color: '#34A853',
    bg: '#CEEAD6',
    title: 'Marketplaces',
    desc: 'Replace overwhelming category pages with smart, conversational discovery. Guide buyers from intent to purchase across multi-vendor catalogs. Aggregate products from multiple sellers and present the best matches for each visitor.',
  },
  {
    icon: Shirt,
    color: '#FBBC04',
    bg: '#FEEFC3',
    title: 'Fashion & Apparel',
    desc: 'Let customers describe what they are looking for in natural language — "casual summer outfit under $100" — and get curated selections. Handle size, color, and style preferences through conversation instead of rigid filters.',
  },
  {
    icon: Coffee,
    color: '#1A73E8',
    bg: '#D2E3FC',
    title: 'Food & Specialty',
    desc: 'Dietary preferences, allergens, flavor profiles — specialty products have unique discovery challenges. AI understands these nuances and recommends products that match complex, overlapping criteria.',
  },
  {
    icon: Home,
    color: '#34A853',
    bg: '#CEEAD6',
    title: 'Home & Lifestyle',
    desc: 'Large catalogs with technical specs need guided discovery. Help customers find the right furniture, appliances, or decor by understanding their space, style, and budget through natural conversation.',
  },
];

export default function UseCasesPage() {
  const [showDemo, setShowDemo] = useState(false);

  useEffect(() => {
    window.scrollTo(0, 0);
  }, []);

  return (
    <div className="app-container">
      <Header onDemo={() => setShowDemo(true)} />

      <section className="page-hero" style={{ position: 'relative', overflow: 'hidden' }}>
        {confetti.map((s, i) => (
          <div key={i} className={`floating-shape ${s.type === 'ellipse' ? 'shape-ellipse' : 'shape-rect'} ${s.color} ${s.delay}`}
            style={{ top: s.top, left: s.left, width: s.w, height: s.h || s.w, opacity: 0.6, transform: s.rot ? `rotate(${s.rot})` : undefined }} />
        ))}
        <h1>Use Cases</h1>
        <p className="page-subtitle">See how Keepstar One works across industries and business models.</p>
      </section>

      <div className="page-body container">
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(360px, 1fr))', gap: 32 }}>
          {useCases.map((c) => (
            <div key={c.title} style={{ padding: '40px 32px', border: '1px solid var(--border-light)', borderRadius: 20 }}>
              <div style={{ width: 48, height: 48, borderRadius: 12, background: c.bg, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 20 }}>
                <c.icon size={24} color={c.color} />
              </div>
              <h3 style={{ fontSize: 20, fontWeight: 600, marginBottom: 12 }}>{c.title}</h3>
              <p style={{ fontSize: 15, color: 'var(--text-secondary)', lineHeight: 1.6 }}>{c.desc}</p>
            </div>
          ))}
        </div>

        <div style={{ textAlign: 'center', padding: '64px 0' }}>
          <h2 style={{ fontSize: 36, marginBottom: 16 }}>Don't see your industry?</h2>
          <p style={{ fontSize: 18, color: 'var(--text-secondary)', marginBottom: 32 }}>
            Keepstar One works with any product catalog. Talk to us about your specific needs.
          </p>
          <button className="btn-hero btn-hero-primary" onClick={() => setShowDemo(true)}>Request demo</button>
        </div>
      </div>

      <Footer />
      {showDemo && <DemoModal onClose={() => setShowDemo(false)} />}
    </div>
  );
}
