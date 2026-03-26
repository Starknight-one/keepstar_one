import React from 'react';
import { User, GitBranch, TrendingUp, Database, Star } from 'lucide-react';
import Confetti from './Confetti';

export default function FeatureRows() {
  return (
    <div className="features-container" style={{ position: 'relative', overflow: 'hidden' }}>
      <Confetti seed={30} count={10} opacity={0.35} />
      {/* Feature 1 */}
      <section className="feature-row container">
        <div className="feature-text">
          <h2 className="feature-title">Hyper-Personalization</h2>
          <p className="feature-desc">Every visitor sees a page built just for them. The AI analyzes context, intent, and behavior to generate unique page layouts in real time. Product cards, comparisons, galleries — assembled dynamically from your catalog data.</p>
        </div>
        <div className="feature-visual" style={{background: 'linear-gradient(135deg, #C6DAFC 0%, #FEEFC3 100%)'}}>
          <div className="feature-card-mock">
            <div className="fc-icon"><User color="#1A73E8" /></div>
            <div>
              <div className="fc-title">Personalized for Sarah</div>
              <div className="fc-subtitle">Showing content based on preferences</div>
            </div>
          </div>
        </div>
      </section>

      {/* Feature 2 */}
      <section className="feature-row container reverse-row">
        <div className="feature-visual" style={{background: 'linear-gradient(135deg, #CEEAD6 0%, #D2E3FC 100%)'}}>
          <div className="feature-card-mock mb-3">
            <div className="fc-icon"><GitBranch color="#34A853" /></div>
            <div>
              <div className="fc-title">Discovery → Comparison → Decision</div>
              <div className="fc-subtitle">Automated funnel progression</div>
            </div>
          </div>
          <div className="feature-card-mock">
            <div className="fc-icon"><TrendingUp color="#34A853" /></div>
            <div>
              <div className="fc-title">189 visitors converted today</div>
              <div className="fc-subtitle">Real-time conversion tracking</div>
            </div>
          </div>
        </div>
        <div className="feature-text">
          <h2 className="feature-title">Auto-Funnel</h2>
          <p className="feature-desc">Every conversation is a conversion funnel. Visitors don't browse randomly anymore. Each interaction guides them closer to purchase — from discovery to comparison to checkout. Replace your entire website with an intelligent journey.</p>
        </div>
      </section>

      {/* Feature 3 */}
      <section className="feature-row container">
        <div className="feature-text">
          <h2 className="feature-title">Zero Info Loss</h2>
          <p className="feature-desc">The more data you have, the smarter it gets. Traditional sites force you to pick what to show. Keepstar uses all your product data — specs, reviews, comparisons — and surfaces exactly what each visitor needs to hear to buy.</p>
        </div>
        <div className="feature-visual" style={{background: 'linear-gradient(135deg, #FEEFC3 0%, #FCE8E6 100%)'}}>
          <div className="feature-card-mock mb-3">
            <div className="fc-icon"><Database color="#EA4335" /></div>
            <div>
              <div className="fc-title">2,847 specs indexed</div>
              <div className="fc-subtitle">All product data enriched</div>
            </div>
          </div>
          <div className="feature-card-mock">
            <div className="fc-icon"><Star color="#FBBC04" fill="#FBBC04" /></div>
            <div>
              <div className="fc-title">1,203 reviews surfaced</div>
              <div className="fc-subtitle">Matched to visitor intent</div>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
