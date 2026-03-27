import React, { useState } from 'react';
import { Check } from 'lucide-react';
import Confetti from './Confetti';
import './Pricing.css';

const plans = [
  {
    name: 'Base',
    monthlyPrice: 399,
    yearlyPrice: 339,
    desc: 'Everything you need to launch AI-powered personalization.',
    features: [
      '2,500 messages/mo',
      '2,500 products',
      'AI chat widget',
      'Visual Assembly Engine',
      'Basic analytics',
      'Support via Telegram & email',
      '+20% conversion rate lift',
    ],
    featured: false,
  },
  {
    name: 'Pro',
    monthlyPrice: 799,
    yearlyPrice: 679,
    desc: 'For growing brands that want to maximize every visitor.',
    features: [
      '5,000 messages/mo',
      'Up to 15,000 products',
      'Advanced AI personalization',
      'Visual Assembly Engine',
      'Full analytics dashboard',
      'Priority support via Telegram & email',
      'Custom branding',
      'SLA guarantee',
      '+20% conversion rate lift',
    ],
    featured: true,
  },
  {
    name: 'Enterprise',
    monthlyPrice: null,
    yearlyPrice: null,
    desc: 'For high-traffic stores that need enterprise-grade performance.',
    features: [
      'Unlimited messages',
      'Unlimited products',
      'Multi-store support',
      'API access',
      'Dedicated account manager',
      'Custom integrations',
      'SLA guarantee',
      'SSO & team management',
      'Visual Assembly Engine',
      '+20% conversion rate lift',
    ],
    featured: false,
  },
];

export default function Pricing({ onDemo }) {
  const [yearly, setYearly] = useState(false);

  return (
    <section id="pricing" className="pricing-section container" style={{ position: 'relative', overflow: 'hidden' }}>
      <Confetti seed={70} count={10} opacity={0.35} />
      <div className="pricing-header">
        <div className="pricing-badge">Pricing</div>
        <h2 className="pricing-title">Simple, transparent pricing</h2>
        <p className="pricing-subtitle">
          Start converting more visitors today. No hidden fees, cancel anytime.
        </p>

        <div className="billing-toggle">
          <span className={`toggle-label ${!yearly ? 'active' : ''}`}>Monthly</span>
          <button className="toggle-switch" onClick={() => setYearly(!yearly)} aria-label="Toggle billing period">
            <span className={`toggle-knob ${yearly ? 'on' : ''}`} />
          </button>
          <span className={`toggle-label ${yearly ? 'active' : ''}`}>
            Yearly <span className="toggle-save">Save 15%</span>
          </span>
        </div>
      </div>

      <div className="pricing-grid">
        {plans.map((plan) => {
          const isEnterprise = plan.monthlyPrice === null;
          const price = yearly ? plan.yearlyPrice : plan.monthlyPrice;

          return (
            <div key={plan.name} className={`pricing-card${plan.featured ? ' featured' : ''}`}>
              {plan.featured && <span className="popular-badge">Most Popular</span>}
              <div className="plan-name">{plan.name}</div>
              <div className="plan-price-row">
                {isEnterprise ? (
                  <span className="plan-price plan-price-custom">Custom</span>
                ) : (
                  <>
                    <span className="plan-price">${price}</span>
                    <span className="plan-period">/{yearly ? 'mo, billed yearly' : 'mo'}</span>
                  </>
                )}
              </div>
              {isEnterprise && yearly ? null : !isEnterprise && yearly && (
                <div className="plan-yearly-total">${price * 12}/year</div>
              )}
              <p className="plan-desc">{plan.desc}</p>
              <ul className="features-list">
                {plan.features.map((f) => (
                  <li key={f} className={`feature-item${f.startsWith('+20%') ? ' feature-highlight' : ''}`}>
                    <Check size={18} className="feature-icon" />
                    {f}
                  </li>
                ))}
              </ul>
              <button
                className={`btn-pricing ${plan.featured ? 'btn-pricing-primary' : 'btn-pricing-secondary'}`}
                onClick={onDemo}
              >
                Contact sales
              </button>
            </div>
          );
        })}
      </div>
    </section>
  );
}
