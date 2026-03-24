import React from 'react';
import { Check } from 'lucide-react';
import './Pricing.css';

const plans = [
  {
    name: 'Starter',
    price: '$129',
    period: '/mo',
    desc: 'For small stores getting started with AI-powered personalization.',
    features: [
      'Up to 5,000 visitors/mo',
      'AI chat widget',
      'Product catalog sync',
      'Basic analytics',
      'Email support',
    ],
    featured: false,
  },
  {
    name: 'Growth',
    price: '$299',
    period: '/mo',
    desc: 'For growing brands that want to maximize every visitor.',
    features: [
      'Up to 25,000 visitors/mo',
      'Advanced AI personalization',
      'Visual Assembly Engine',
      'Full analytics dashboard',
      'Priority support',
      'Custom branding',
    ],
    featured: true,
  },
  {
    name: 'Scale',
    price: '$599',
    period: '/mo',
    desc: 'For high-traffic stores that need enterprise-grade performance.',
    features: [
      'Unlimited visitors',
      'Multi-store support',
      'API access',
      'Dedicated account manager',
      'Custom integrations',
      'SLA guarantee',
      'SSO & team management',
    ],
    featured: false,
  },
];

export default function Pricing({ onDemo }) {
  return (
    <section id="pricing" className="pricing-section container">
      <div className="pricing-header">
        <div className="pricing-badge">Pricing</div>
        <h2 className="pricing-title">Simple, transparent pricing</h2>
        <p className="pricing-subtitle">
          Start converting more visitors today. No hidden fees, cancel anytime.
        </p>
      </div>

      <div className="pricing-grid">
        {plans.map((plan) => (
          <div key={plan.name} className={`pricing-card${plan.featured ? ' featured' : ''}`}>
            {plan.featured && <span className="popular-badge">Most Popular</span>}
            <div className="plan-name">{plan.name}</div>
            <div className="plan-price-row">
              <span className="plan-price">{plan.price}</span>
              <span className="plan-period">{plan.period}</span>
            </div>
            <p className="plan-desc">{plan.desc}</p>
            <ul className="features-list">
              {plan.features.map((f) => (
                <li key={f} className="feature-item">
                  <Check size={18} className="feature-icon" />
                  {f}
                </li>
              ))}
            </ul>
            <button className={`btn-pricing ${plan.featured ? 'btn-pricing-primary' : 'btn-pricing-secondary'}`} onClick={onDemo}>
              Get started
            </button>
          </div>
        ))}
      </div>
    </section>
  );
}
