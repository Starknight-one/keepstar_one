import React from 'react';
import { Link } from 'react-router-dom';
import { ShoppingBag, Sparkles, Store } from 'lucide-react';
import Confetti from './Confetti';
import './UseCases.css';

const cases = [
  {
    icon: ShoppingBag,
    color: '#4285F4',
    bg: '#D2E3FC',
    title: 'E-Commerce Stores',
    desc: 'Turn product browsing into guided conversations. Every visitor gets personalized product recommendations, comparisons, and details — assembled dynamically from your catalog.',
  },
  {
    icon: Sparkles,
    color: '#EA4335',
    bg: '#FCE8E6',
    title: 'Beauty & Cosmetics',
    desc: 'Help shoppers find the right products by skin type, concern, or ingredient preference. AI surfaces relevant items from thousands of SKUs in seconds.',
  },
  {
    icon: Store,
    color: '#34A853',
    bg: '#CEEAD6',
    title: 'Marketplaces',
    desc: 'Replace overwhelming category pages with smart, conversational discovery. Guide buyers from intent to purchase across multi-vendor catalogs.',
  },
];

export default function UseCases() {
  return (
    <section id="use-cases" className="use-cases-section container" style={{ position: 'relative', overflow: 'hidden' }}>
      <Confetti seed={35} count={8} opacity={0.3} />
      <div className="use-cases-header">
        <div className="use-cases-badge">Use Cases</div>
        <h2 className="use-cases-title">Built for stores that want to sell more</h2>
        <p className="use-cases-subtitle">
          See how Keepstar One works across different industries and business models.
        </p>
      </div>

      <div className="use-cases-grid">
        {cases.map((c) => (
          <div key={c.title} className="use-case-card">
            <div className="use-case-icon" style={{ background: c.bg }}>
              <c.icon size={24} color={c.color} />
            </div>
            <h3 className="use-case-card-title">{c.title}</h3>
            <p className="use-case-card-desc">{c.desc}</p>
          </div>
        ))}
      </div>

      <div className="use-cases-cta">
        <Link to="/use-cases" className="btn-hero btn-hero-secondary">See all use cases</Link>
      </div>
    </section>
  );
}
