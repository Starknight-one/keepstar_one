import React from 'react';
import { User, GitBranch, TrendingUp, Database, Star } from 'lucide-react';

export default function FeatureRows() {
  return (
    <div className="features-container">
      {/* Feature 1 */}
      <section className="feature-row container">
        <div className="feature-text">
          <h2 className="feature-title">Hyper-Personalization</h2>
          <p className="feature-desc">A visitor types &lsquo;moisturizer for dry skin&rsquo; — they see 4 matching products with reviews and prices. Someone else types &lsquo;gift set under $50&rsquo; — completely different page. No templates, no rules to configure.</p>
        </div>
        <div className="feature-visual" style={{background: 'linear-gradient(135deg, rgba(91,163,208,0.15) 0%, rgba(245,160,74,0.1) 100%)'}}>
          <div className="feature-card-mock">
            <div className="fc-icon"><User color="#185FA5" /></div>
            <div>
              <div className="fc-title">Personalized for Sarah</div>
              <div className="fc-subtitle">Showing content based on preferences</div>
            </div>
          </div>
        </div>
      </section>

      {/* Feature 2 */}
      <section className="feature-row container reverse-row">
        <div className="feature-visual" style={{background: 'linear-gradient(135deg, rgba(11,27,46,0.05) 0%, rgba(91,163,208,0.12) 100%)'}}>
          <div className="feature-card-mock mb-3">
            <div className="fc-icon"><GitBranch color="#185FA5" /></div>
            <div>
              <div className="fc-title">Discovery → Comparison → Decision</div>
              <div className="fc-subtitle">Automated funnel progression</div>
            </div>
          </div>
          <div className="feature-card-mock">
            <div className="fc-icon"><TrendingUp color="#185FA5" /></div>
            <div>
              <div className="fc-title">189 visitors converted today</div>
              <div className="fc-subtitle">Real-time conversion tracking</div>
            </div>
          </div>
        </div>
        <div className="feature-text">
          <h2 className="feature-title">Auto-Funnel</h2>
          <p className="feature-desc">Each conversation guides visitors from discovery to comparison to checkout. No random browsing — every interaction moves them closer to purchase. Your store gets a conversion path that adapts to each person.</p>
        </div>
      </section>

      {/* Feature 3 */}
      <section className="feature-row container">
        <div className="feature-text">
          <h2 className="feature-title">Zero Info Loss</h2>
          <p className="feature-desc">Your catalog has 2,000 products with specs, reviews, and comparisons. A static page shows 20 of them. Keepstar surfaces the specs, reviews, and comparisons relevant to each visitor's question — nothing hidden, nothing wasted.</p>
        </div>
        <div className="feature-visual" style={{background: 'linear-gradient(135deg, rgba(245,160,74,0.1) 0%, rgba(91,163,208,0.08) 100%)'}}>
          <div className="feature-card-mock mb-3">
            <div className="fc-icon"><Database color="#5BA3D0" /></div>
            <div>
              <div className="fc-title">2,847 specs indexed</div>
              <div className="fc-subtitle">All product data enriched</div>
            </div>
          </div>
          <div className="feature-card-mock">
            <div className="fc-icon"><Star color="#F5A04A" fill="#F5A04A" /></div>
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
