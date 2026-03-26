import React from 'react';
import { User, LayoutGrid, Code, Target, TrendingUp, Zap, Database, Globe } from 'lucide-react';
import Confetti from './Confetti';

export default function IconStrip() {
  const icons = [User, LayoutGrid, Code, Target, TrendingUp, Zap, Database, Globe];

  return (
    <section className="icon-strip-section container" style={{ position: 'relative', overflow: 'hidden' }}>
      <Confetti seed={20} count={6} opacity={0.3} />
      <div className="icon-row">
        {icons.map((Icon, idx) => (
          <div key={idx} className="icon-circle">
            <Icon size={24} color="#5F6368" />
          </div>
        ))}
      </div>
      
      <div className="big-statement-wrapper">
        <h2 className="big-statement">
          Keepstar is an AI-powered dynamic website engine, replacing static pages with personalized experiences that convert.
        </h2>
      </div>
    </section>
  );
}
